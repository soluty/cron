package cron

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alaingilbert/clockwork"
)

// Cron keeps track of any number of entries, invoking the associated func as
// specified by the schedule. It may be started, stopped, and the entries may
// be inspected while running.
type Cron struct {
	clockMu   sync.Mutex
	clock     clockwork.Clock
	nextID    int32 // atomic value
	entries   []*Entry
	entriesMu sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	update    chan context.CancelFunc
	running   int32 // atomic value
	locMu     sync.Mutex
	location  *time.Location
	jobWaiter sync.WaitGroup
}

type EntryID int32

// Job is an interface for submitted cron jobs.
type Job interface {
	Run()
}

// The Schedule describes a job's duty cycle.
type Schedule interface {
	// Return the next activation time, later than the given time.
	// Next is invoked initially, and then each time the job is run.
	Next(time.Time) time.Time
}

// Entry consists of a schedule and the func to execute on that schedule.
type Entry struct {
	ID EntryID

	// The schedule on which this job should be run.
	Schedule Schedule

	// The next time the job will run. This is the zero time if Cron has not been
	// started or this entry's schedule is unsatisfiable
	Next time.Time

	// The last time this job was run. This is the zero time if the job has never
	// been run.
	Prev time.Time

	// The Job to run.
	Job Job

	Label string
}

// New returns a new Cron job runner, in the Local time zone.
func New(opts ...Option) *Cron {
	ctx, cancel := context.WithCancel(context.Background())
	clock := clockwork.NewRealClock()
	c := &Cron{
		clock:    clock,
		entries:  nil,
		ctx:      ctx,
		cancel:   cancel,
		update:   make(chan context.CancelFunc),
		location: clock.Location(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Cron) startRunning() bool {
	return atomic.CompareAndSwapInt32(&c.running, 0, 1)
}
func (c *Cron) stopRunning() bool {
	return atomic.CompareAndSwapInt32(&c.running, 1, 0)
}
func (c *Cron) isRunning() bool {
	return atomic.LoadInt32(&c.running) == 1
}

// A wrapper that turns a func() into a cron.Job
type FuncJob func()

func (f FuncJob) Run() { f() }

// AddFunc adds a func to the Cron to be run on the given schedule.
func (c *Cron) AddFunc(spec string, cmd func(), label string) (EntryID, error) {
	return c.AddJob(spec, FuncJob(cmd), label)
}

// AddJob adds a Job to the Cron to be run on the given schedule.
func (c *Cron) AddJob(spec string, cmd Job, label string) (EntryID, error) {
	schedule, err := Parse(spec)
	if err != nil {
		return 0, err
	}
	return c.Schedule(schedule, cmd, label), nil
}

// Schedule adds a Job to the Cron to be run on the given schedule.
func (c *Cron) Schedule(schedule Schedule, cmd Job, label string) EntryID {
	newID := atomic.AddInt32(&c.nextID, 1)
	entry := &Entry{
		ID:       EntryID(newID),
		Schedule: schedule,
		Job:      cmd,
		Label:    label,
	}
	entry.Next = entry.Schedule.Next(c.now())
	c.appendEntry(entry)
	if c.isRunning() {
		c.entriesUpdated()
	}
	return entry.ID
}

// Entries returns a snapshot of the cron entries.
func (c *Cron) Entries() []Entry {
	return c.entrySnapshot()
}

// Entry returns a snapshot of the given entry, or nil if it couldn't be found.
func (c *Cron) Entry(id EntryID) (out Entry) {
	for _, entry := range c.Entries() {
		if id == entry.ID {
			return entry
		}
	}
	return
}

// Remove an entry from being run in the future.
func (c *Cron) Remove(id EntryID) {
	c.removeEntry(id)
	c.entriesUpdated()
}

// Run the cron scheduler, or no-op if already running.
func (c *Cron) Run() (started bool) {
	if started = c.startRunning(); started {
		c.run()
	}
	return
}

// Start the cron scheduler in its own go-routine, or no-op if already started.
func (c *Cron) Start() (started bool) {
	if started = c.startRunning(); started {
		go c.run()
	}
	return
}

// Stop stops the cron scheduler if it is running; otherwise it does nothing.
// A context is returned so the caller can wait for running jobs to complete.
func (c *Cron) Stop() context.Context {
	if c.stopRunning() {
		c.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		c.jobWaiter.Wait()
		cancel()
	}()
	return ctx
}

func (c *Cron) runWithRecovery(j Job) {
	defer func() { recover() }()
	j.Run()
}

func (c *Cron) afterCh(delay time.Duration) (out <-chan time.Time) {
	c.clockMu.Lock()
	out = c.clock.After(delay)
	c.clockMu.Unlock()
	return
}

func (c *Cron) advanceFakeClock(d time.Duration) {
	c.clockMu.Lock()
	defer c.clockMu.Unlock()
	if fc, ok := c.clock.(clockwork.FakeClock); ok {
		fc.Advance(d)
	}
}

// Run the scheduler. this is private just due to the need to synchronize
// access to the 'running' state variable.
func (c *Cron) run() {
	// Figure out the next activation times for each entry.
	c.setEntriesNext()
	for {
		// Determine the next entry to run.
		delay := c.getNextDelay()
		select {
		case <-c.afterCh(delay):
			c.runDueEntries()
		case updated := <-c.update:
			c.getNextDelay()
			c.runDueEntries()
			updated()
		case <-c.ctx.Done():
			return
		}
	}
}

// trigger an update of the entries in the run loop
func (c *Cron) entriesUpdated() {
	// If the cron is not running, no need to notify the main loop about updating entries
	if !c.isRunning() {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	// Wait until the main loop pickup our update task (or cron exiting)
	select {
	case c.update <- cancel:
	case <-c.ctx.Done():
	}
	// Wait until "runDueEntries" is done running (or cron exiting)
	select {
	case <-ctx.Done():
	case <-c.ctx.Done():
	}
}

// entrySnapshot returns a copy of the current cron entry list.
func (c *Cron) entrySnapshot() []Entry {
	c.entriesMu.RLock()
	defer c.entriesMu.RUnlock()
	var entries = make([]Entry, len(c.entries))
	for i, e := range c.entries {
		entries[i] = *e
	}
	return entries
}

// Run every entry whose next time was less than now
func (c *Cron) runDueEntries() {
	c.entriesMu.Lock()
	defer c.entriesMu.Unlock()
	now := c.now()
	for _, e := range c.entries {
		if e.Next.After(now) || e.Next.IsZero() {
			break
		}
		c.startJob(e.Job)
		e.Prev = e.Next
		e.Next = e.Schedule.Next(now)
	}
}

func (c *Cron) getNextDelay() time.Duration {
	c.entriesMu.RLock()
	defer c.entriesMu.RUnlock()
	c.sortEntries()
	if len(c.entries) == 0 || c.entries[0].Next.IsZero() {
		return 100000 * time.Hour // If there are no entries yet, just sleep - it still handles new entries and stop requests.
	}
	return c.entries[0].Next.Sub(c.now())
}

func (c *Cron) setEntriesNext() {
	c.entriesMu.Lock()
	defer c.entriesMu.Unlock()
	now := c.now()
	for _, entry := range c.entries {
		entry.Next = entry.Schedule.Next(now)
	}
}

func (c *Cron) sortEntries() {
	sort.Slice(c.entries, func(i, j int) bool {
		if c.entries[i].Next.IsZero() {
			return false
		}
		if c.entries[j].Next.IsZero() {
			return true
		}
		return c.entries[i].Next.Before(c.entries[j].Next)
	})
}

func (c *Cron) appendEntry(entry *Entry) {
	c.entriesMu.Lock()
	defer c.entriesMu.Unlock()
	c.entries = append(c.entries, entry)
}

func (c *Cron) removeEntry(id EntryID) {
	c.entriesMu.Lock()
	defer c.entriesMu.Unlock()
	for i := len(c.entries) - 1; i >= 0; i-- {
		if c.entries[i].ID == id {
			c.entries = append(c.entries[:i], c.entries[i+1:]...) // remove entry
			break
		}
	}
}

// startJob runs the given job in a new goroutine.
func (c *Cron) startJob(j Job) {
	c.jobWaiter.Add(1)
	go func() {
		defer c.jobWaiter.Done()
		c.runWithRecovery(j)
	}()
}

// Waits until all processing jobs are done running (for tests)
func (c *Cron) waitJobs() {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		c.jobWaiter.Wait()
		cancel()
	}()
	select {
	case <-ctx.Done():
	case <-c.ctx.Done():
	}
}

// Location gets the time zone location
func (c *Cron) Location() *time.Location {
	c.locMu.Lock()
	defer c.locMu.Unlock()
	return c.location
}

// SetLocation ...
func (c *Cron) SetLocation(newLoc *time.Location) {
	c.locMu.Lock()
	c.location = newLoc
	c.locMu.Unlock()
	c.setEntriesNext()
	c.entriesUpdated()
}

// now returns current time in c location
func (c *Cron) now() time.Time {
	return c.clock.Now().In(c.Location())
}
