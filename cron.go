package cron

import (
	"container/heap"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alaingilbert/cron/internal/mtx"
	isync "github.com/alaingilbert/cron/internal/sync"
	"github.com/alaingilbert/cron/internal/utils"
	"github.com/jonboulle/clockwork"
)

// Cron keeps track of any number of entries, invoking the associated func as
// specified by the schedule. It may be started, stopped, and the entries may
// be inspected while running.
type Cron struct {
	clock            clockwork.Clock                   // Clock interface (real or mock) used for timing
	runningJobsCount atomic.Int32                      // Count of currently running jobs
	runningJobsMap   isync.Map[EntryID, *atomic.Int32] // Keep track of currently running jobs
	cond             sync.Cond                         // Signals when all jobs have completed
	entries          mtx.RWMtx[Entries]                // Thread-safe, sorted list of job entries
	ctx              context.Context                   // Context to control the scheduler lifecycle
	cancel           context.CancelFunc                // Cancels the scheduler context
	update           chan context.CancelFunc           // Triggers update in the scheduler loop
	running          atomic.Bool                       // Indicates if the scheduler is currently running
	location         mtx.RWMtx[*time.Location]         // Thread-safe time zone location
	logger           *log.Logger                       // Logger for scheduler events, errors, and diagnostics
	parser           ScheduleParser                    // Parses cron expressions into schedule objects
	idFactory        EntryIDFactory                    // Generates a new unique EntryID for each scheduled job
}

type Entries struct {
	entriesHeap EntryHeap
	entriesMap  map[EntryID]*Entry
}

// ErrEntryNotFound ...
var ErrEntryNotFound = errors.New("entry not found")

// ErrUnsupportedJobType ...
var ErrUnsupportedJobType = errors.New("unsupported job type")

// ErrJobAlreadyRunning ...
var ErrJobAlreadyRunning = errors.New("job already running")

// ErrIDAlreadyUsed ...
var ErrIDAlreadyUsed = errors.New("id already used")

// The Schedule describes a job's duty cycle.
type Schedule interface {
	// Next return the next activation time, later than the given time.
	// Next is invoked initially, and then each time the job is run.
	Next(time.Time) time.Time
}

// ScheduleParser is an interface for schedule spec parsers that return a Schedule
type ScheduleParser interface {
	Parse(spec string) (Schedule, error)
}

type FuncEntryIDFactory func() EntryID

func (f FuncEntryIDFactory) Next() EntryID { return f() }

type EntryIDFactory interface {
	Next() EntryID
}

// UUIDEntryIDFactory generate and format UUID V4
func UUIDEntryIDFactory() EntryIDFactory {
	return FuncEntryIDFactory(func() EntryID {
		var uuid [16]byte
		_, _ = io.ReadFull(rand.Reader, uuid[:])
		uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
		uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant is 10
		var buf [32]byte
		hex.Encode(buf[:], uuid[:4])
		hex.Encode(buf[8:12], uuid[4:6])
		hex.Encode(buf[12:16], uuid[6:8])
		hex.Encode(buf[16:20], uuid[8:10])
		hex.Encode(buf[20:], uuid[10:])
		return EntryID(buf[:])
	})
}

//-----------------------------------------------------------------------------

// New returns a new Cron job runner
func New(opts ...Option) *Cron {
	cfg := utils.BuildConfig(opts)
	clock := utils.Or(cfg.Clock, clockwork.NewRealClock())
	location := utils.Or(cfg.Location, clock.Now().Location())
	parentCtx := utils.Or(cfg.Ctx, context.Background())
	logger := utils.Or(cfg.Logger, log.New(os.Stderr, "cron", log.LstdFlags))
	parser := utils.Or(cfg.Parser, ScheduleParser(standardParser))
	idFactory := utils.Or(cfg.IDFactory, UUIDEntryIDFactory())
	ctx, cancel := context.WithCancel(parentCtx)
	return &Cron{
		cond:           sync.Cond{L: &sync.Mutex{}},
		clock:          clock,
		ctx:            ctx,
		cancel:         cancel,
		runningJobsMap: isync.Map[EntryID, *atomic.Int32]{},
		update:         make(chan context.CancelFunc),
		location:       mtx.NewRWMtx(location),
		logger:         logger,
		parser:         parser,
		idFactory:      idFactory,
		entries: mtx.NewRWMtx(Entries{
			entriesHeap: make(EntryHeap, 0),
			entriesMap:  make(map[EntryID]*Entry),
		}),
	}
}

// Run the cron scheduler, or no-op if already running.
func (c *Cron) Run() (started bool) { return c.start() }

// Start the cron scheduler in its own go-routine, or no-op if already started.
func (c *Cron) Start() (started bool) { return c.startAsync() }

// Stop stops the cron scheduler if it is running; otherwise it does nothing.
// A context is returned so the caller can wait for running jobs to complete.
func (c *Cron) Stop() <-chan struct{} { return c.stop() }

// AddJob adds a Job to the Cron to be run on the given schedule.
func (c *Cron) AddJob(spec string, job IntoJob, opts ...EntryOption) (EntryID, error) {
	return c.addJob(spec, job, opts...)
}

// AddJob1 adds a Job to the Cron to be run on the given schedule.
func (c *Cron) AddJob1(spec string, job Job, opts ...EntryOption) (EntryID, error) {
	return c.addJob(spec, job, opts...)
}

// Schedule adds a Job to the Cron to be run on the given schedule.
func (c *Cron) Schedule(schedule Schedule, job Job, opts ...EntryOption) (EntryID, error) {
	return c.schedule(schedule, job, opts...)
}

// AddEntry ...
func (c *Cron) AddEntry(entry Entry, opts ...EntryOption) (EntryID, error) {
	return c.addEntry(entry, opts...)
}

// Enable ...
func (c *Cron) Enable(id EntryID) { c.setEntryActive(id, true) }

// Disable ...
func (c *Cron) Disable(id EntryID) { c.setEntryActive(id, false) }

// UpdateSchedule ...
func (c *Cron) UpdateSchedule(id EntryID, schedule Schedule) error {
	return c.updateSchedule(id, schedule)
}

// UpdateScheduleWithSpec ...
func (c *Cron) UpdateScheduleWithSpec(id EntryID, spec string) error {
	return c.updateScheduleWithSpec(id, spec)
}

// Entries returns a snapshot of the cron entries.
func (c *Cron) Entries() (out []Entry) { return c.getEntries() }

// Entry returns a snapshot of the given entry, or nil if it couldn't be found.
func (c *Cron) Entry(id EntryID) (out Entry, err error) { return c.getEntry(id) }

// Remove an entry from being run in the future.
func (c *Cron) Remove(id EntryID) { c.remove(id) }

// RunNow allows the user to run a specific job now
func (c *Cron) RunNow(id EntryID) { c.runNow(id) }

// IsRunning either or not a specific job is currently running
func (c *Cron) IsRunning(id EntryID) bool { return c.entryIsRunning(id) }

// Location gets the time zone location
func (c *Cron) Location() *time.Location { return c.getLocation() }

// SetLocation sets a new location to use.
// Re-set the "Next" values for all entries.
// Re-sort entries and run due entries.
func (c *Cron) SetLocation(newLoc *time.Location) { c.setLocation(newLoc) }

// GetNextTime returns the next time a job is scheduled to be executed
// If no job is scheduled to be executed, the Zero time is returned
func (c *Cron) GetNextTime() time.Time { return c.getNextTime() }

//-----------------------------------------------------------------------------

func (c *Cron) start() (started bool) {
	return c.startWith(func() { c.run() }) // sync
}

func (c *Cron) startAsync() (started bool) {
	return c.startWith(func() { go c.run() }) // async
}

func (c *Cron) startWith(runFunc func()) (started bool) {
	if started = c.startRunning(); started {
		runFunc()
	}
	return
}

func (c *Cron) stop() <-chan struct{} {
	if !c.stopRunning() {
		return nil
	}
	c.cancel()
	ch := make(chan struct{})
	go func() {
		c.waitAllJobsCompleted()
		close(ch)
	}()
	return ch
}

func (c *Cron) startRunning() bool {
	return c.running.CompareAndSwap(false, true)
}
func (c *Cron) stopRunning() bool {
	return c.running.CompareAndSwap(true, false)
}
func (c *Cron) isRunning() bool {
	return c.running.Load()
}

func findByIDFn(id EntryID) func(e *Entry) bool {
	return func(e *Entry) bool { return e.ID == id }
}

func (c *Cron) runNow(id EntryID) {
	if err := c.entries.WithE(func(entries *Entries) error {
		if entry, idx := utils.FindIdx((*entries).entriesHeap, findByIDFn(id)); entry != nil {
			(*entry).Next = c.now()
			reInsertEntry(&entries.entriesHeap, idx) // runNow
			return nil
		}
		return errors.New("not found")
	}); err != nil {
		return
	}
	c.entriesUpdated() // runNow
}

func (c *Cron) addJob(spec string, job IntoJob, opts ...EntryOption) (EntryID, error) {
	return c.addJob1(spec, castIntoJob(job), opts...)
}

func (c *Cron) addJob1(spec string, job Job, opts ...EntryOption) (EntryID, error) {
	schedule, err := c.parser.Parse(spec)
	if err != nil {
		var zeroID EntryID
		return zeroID, err
	}
	return c.schedule(schedule, job, opts...)
}

func (c *Cron) schedule(schedule Schedule, job Job, opts ...EntryOption) (EntryID, error) {
	entry := Entry{
		ID:       c.idFactory.Next(),
		job:      job,
		Schedule: schedule,
		Next:     schedule.Next(c.now()),
		Active:   true,
	}
	return c.addEntry(entry, opts...)
}

func (c *Cron) addEntry(entry Entry, opts ...EntryOption) (EntryID, error) {
	for _, opt := range opts {
		opt(c, &entry)
	}
	if c.entryExists(entry.ID) {
		var zeroID EntryID
		return zeroID, ErrIDAlreadyUsed
	}
	c.entries.With(func(entries *Entries) {
		entries.entriesMap[entry.ID] = &entry
		heap.Push(&entries.entriesHeap, &entry)
	})
	c.entriesUpdated() // addEntry
	return entry.ID, nil
}

func (c *Cron) setEntryActive(id EntryID, active bool) {
	if err := c.entries.WithE(func(entries *Entries) error {
		entry, idx := utils.FindIdx((*entries).entriesHeap, findByIDFn(id))
		if entry == nil || (*entry).Active == active {
			return errors.New("not found or unchanged")
		}
		(*entry).Active = active
		(*entry).Next = (*entry).Schedule.Next(c.now())
		reInsertEntry(&entries.entriesHeap, idx) // setEntryActive
		return nil
	}); err != nil {
		return
	}
	c.entriesUpdated() // setEntryActive
}

func (c *Cron) updateScheduleWithSpec(id EntryID, spec string) error {
	schedule, err := c.parser.Parse(spec)
	if err != nil {
		return err
	}
	return c.updateSchedule(id, schedule)
}

func (c *Cron) updateSchedule(id EntryID, schedule Schedule) error {
	if err := c.entries.WithE(func(entries *Entries) error {
		entry, idx := utils.FindIdx((*entries).entriesHeap, findByIDFn(id))
		if entry == nil {
			return ErrEntryNotFound
		}
		(*entry).Schedule = schedule
		(*entry).Next = schedule.Next(c.now())
		reInsertEntry(&entries.entriesHeap, idx) // updateSchedule
		return nil
	}); err != nil {
		return err
	}
	c.entriesUpdated() // updateSchedule
	return nil
}

func (c *Cron) getNextTime() (out time.Time) {
	c.entries.RWith(func(entries Entries) {
		if e := entries.entriesHeap.Peek(); e != nil && e.Active {
			out = e.Next
		}
	})
	return
}

func (c *Cron) getNextDelay() (out time.Duration) {
	nextTime := c.getNextTime()
	if nextTime.IsZero() {
		return 100_000 * time.Hour // If there are no entries yet, just sleep - it still handles new entries and stop requests.
	} else {
		return nextTime.Sub(c.now())
	}
}

func (c *Cron) run() {
	for {
		var updated context.CancelFunc
		select {
		case <-c.clock.After(c.getNextDelay()):
		case updated = <-c.update:
		case <-c.ctx.Done():
			return
		}
		c.runDueEntries()
		if updated != nil {
			updated()
		}
	}
}

// trigger an update of the entries in the run loop
func (c *Cron) entriesUpdated() {
	if c.isRunning() { // If the cron is not running, no need to notify the main loop about updating entries
		ctx, cancel := context.WithCancel(c.ctx)
		select { // Wait until the main loop pickup our update task (or cron exiting)
		case c.update <- cancel:
		case <-ctx.Done():
		}
		<-ctx.Done() // Wait until "runDueEntries" is done running (or cron exiting)
	}
}

// Run every entry whose next time was less than now
func (c *Cron) runDueEntries() {
	if c.isRunning() {
		now := c.now()
		c.entries.With(func(entries *Entries) {
			for {
				entry := entries.entriesHeap.Peek()
				if entry == nil || entry.Next.After(now) || entry.Next.IsZero() || !entry.Active {
					break
				}
				entry = heap.Pop(&entries.entriesHeap).(*Entry)
				entry.Prev = entry.Next
				entry.Next = entry.Schedule.Next(now)
				heap.Push(&entries.entriesHeap, entry)
				c.startJob(*entry)
			}
		})
	}
}

func (c *Cron) getLocation() *time.Location {
	return c.location.Get()
}

func (c *Cron) setLocation(newLoc *time.Location) {
	c.location.Set(newLoc)
	c.setEntriesNext()
	c.entriesUpdated() // setLocation
}

func (c *Cron) sortEntries(entries *EntryHeap) {
	sortedEntries := new(EntryHeap)
	for len(*entries) > 0 {
		heap.Push(sortedEntries, heap.Pop(entries).(*Entry))
	}
	*entries = *sortedEntries
}

// Reset all entries "Next" property.
// This is only done when the cron timezone is changed at runtime.
func (c *Cron) setEntriesNext() {
	now := c.now()
	c.entries.With(func(entries *Entries) {
		for _, entry := range (*entries).entriesHeap {
			entry.Next = entry.Schedule.Next(now)
		}
		c.sortEntries(&entries.entriesHeap) // setEntriesNext
	})
}

func (c *Cron) remove(id EntryID) {
	c.removeEntry(id)
	c.entriesUpdated() // remove
}

func (c *Cron) removeEntry(id EntryID) {
	c.entries.With(func(entries *Entries) {
		delete(entries.entriesMap, id)
		removeEntry(&entries.entriesHeap, id)
	})
}

func removeEntry(entries *EntryHeap, id EntryID) {
	if _, i := utils.FindIdx(*entries, findByIDFn(id)); i != -1 {
		heap.Remove(entries, i)
	}
}

func reInsertEntry(entries *EntryHeap, idx int) {
	entry := heap.Remove(entries, idx).(*Entry)
	heap.Push(entries, entry)
}

func (c *Cron) getEntries() (out []Entry) {
	c.entries.With(func(entries *Entries) {
		clone := new(EntryHeap)
		l := len(entries.entriesHeap)
		out = make([]Entry, l)
		for i := 0; i < l; i++ {
			entry := heap.Pop(&entries.entriesHeap).(*Entry)
			out[i] = *entry
			heap.Push(clone, entry)
		}
		(*entries).entriesHeap = *clone
	})
	return
}

func (c *Cron) getEntry(id EntryID) (out Entry, err error) {
	err = c.entries.RWithE(func(entries Entries) error {
		if entry, ok := entries.entriesMap[id]; ok {
			out = *entry
			return nil
		}
		return ErrEntryNotFound
	})
	return out, err
}

func (c *Cron) entryIsRunning(id EntryID) bool {
	val, ok := c.runningJobsMap.Load(id)
	return ok && val.Load() > 0
}

func (c *Cron) entryExists(id EntryID) bool {
	return utils.Second(c.getEntry(id)) == nil
}

func (c *Cron) updateJobsCounter(key EntryID, delta int32) {
	val, _ := c.runningJobsMap.LoadOrStore(key, &atomic.Int32{})
	val.Add(delta)
}

// startJob runs the given job in a new goroutine.
func (c *Cron) startJob(entry Entry) {
	c.runningJobsCount.Add(1)
	go func() {
		defer func() {
			c.updateJobsCounter(entry.ID, -1)
			c.runningJobsCount.Add(-1)
			c.signalJobCompleted()
		}()
		c.updateJobsCounter(entry.ID, 1)
		c.runWithRecovery(entry)
	}()
}

func (c *Cron) runWithRecovery(entry Entry) {
	defer func() {
		if r := recover(); r != nil {
			c.logger.Printf("%v\n%s\n", r, string(debug.Stack()))
		}
	}()
	if err := entry.job.Run(c.ctx, c, entry); err != nil {
		msg := fmt.Sprintf("error running job %s", entry.ID)
		msg += utils.TernaryOrZero(entry.Label != "", " "+entry.Label)
		msg += " : " + err.Error()
		c.logger.Println(msg)
	}
}

func (c *Cron) signalJobCompleted() {
	c.cond.L.Lock()
	c.cond.Broadcast()
	c.cond.L.Unlock()
}

func (c *Cron) waitAllJobsCompleted() {
	c.cond.L.Lock()
	for c.runningJobsCount.Load() != 0 {
		c.cond.Wait()
	}
	c.cond.L.Unlock()
}

// now returns current time in c location
func (c *Cron) now() time.Time {
	return c.clock.Now().In(c.Location())
}
