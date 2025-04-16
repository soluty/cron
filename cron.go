package cron

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alaingilbert/clockwork"
	"github.com/alaingilbert/cron/internal/mtx"
	"github.com/alaingilbert/cron/internal/utils"
	"github.com/google/uuid"
)

// Cron keeps track of any number of entries, invoking the associated func as
// specified by the schedule. It may be started, stopped, and the entries may
// be inspected while running.
type Cron struct {
	clock            clockwork.Clock           // Clock interface (real or mock) used for timing
	runningJobsCount atomic.Int32              // Count of currently running jobs
	cond             sync.Cond                 // Signals when all jobs have completed
	entries          mtx.RWMtxSlice[*Entry]    // Thread-safe, sorted list of job entries
	ctx              context.Context           // Context to control the scheduler lifecycle
	cancel           context.CancelFunc        // Cancels the scheduler context
	update           chan context.CancelFunc   // Triggers update in the scheduler loop
	running          atomic.Bool               // Indicates if the scheduler is currently running
	location         mtx.RWMtx[*time.Location] // Thread-safe time zone location
	logger           *log.Logger               // Logger
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

//-----------------------------------------------------------------------------

// New returns a new Cron job runner, in the Local time zone.
func New(opts ...Option) *Cron {
	cfg := utils.BuildConfig(opts)
	clock := utils.Or(cfg.Clock, clockwork.NewRealClock())
	location := utils.Or(cfg.Location, clock.Location())
	parentCtx := utils.Or(cfg.Ctx, context.Background())
	logger := utils.Or(cfg.Logger, log.New(os.Stderr, "cron", log.LstdFlags))
	ctx, cancel := context.WithCancel(parentCtx)
	return &Cron{
		cond:     sync.Cond{L: &sync.Mutex{}},
		clock:    clock,
		ctx:      ctx,
		cancel:   cancel,
		update:   make(chan context.CancelFunc),
		location: mtx.NewRWMtx(location),
		logger:   logger,
	}
}

// Run the cron scheduler, or no-op if already running.
func (c *Cron) Run() (started bool) {
	return c.startWith(func() { c.run() }) // sync
}

// Start the cron scheduler in its own go-routine, or no-op if already started.
func (c *Cron) Start() (started bool) {
	return c.startWith(func() { go c.run() }) // async
}

// Stop stops the cron scheduler if it is running; otherwise it does nothing.
// A context is returned so the caller can wait for running jobs to complete.
func (c *Cron) Stop() <-chan struct{} {
	return c.stop()
}

// AddJob adds a Job to the Cron to be run on the given schedule.
func (c *Cron) AddJob(spec string, cmd IntoJob, opts ...EntryOption) (EntryID, error) {
	return c.addJob(spec, cmd, opts...)
}

// Schedule adds a Job to the Cron to be run on the given schedule.
func (c *Cron) Schedule(schedule Schedule, cmd IntoJob, opts ...EntryOption) (EntryID, error) {
	return c.schedule(schedule, cmd, opts...)
}

// AddEntry ...
func (c *Cron) AddEntry(entry Entry, opts ...EntryOption) (EntryID, error) {
	return c.addEntry(entry, opts...)
}

// Enable ...
func (c *Cron) Enable(id EntryID) { c.setEntryActive(id, true) }

// Disable ...
func (c *Cron) Disable(id EntryID) { c.setEntryActive(id, false) }

// Entries returns a snapshot of the cron entries.
func (c *Cron) Entries() (out []Entry) {
	return c.getEntries()
}

// Entry returns a snapshot of the given entry, or nil if it couldn't be found.
func (c *Cron) Entry(id EntryID) (out Entry, err error) {
	return c.getEntry(id)
}

// Remove an entry from being run in the future.
func (c *Cron) Remove(id EntryID) {
	c.remove(id)
}

// RunNow allows the user to run a specific job now
func (c *Cron) RunNow(id EntryID) {
	c.runNow(id)
}

// Location gets the time zone location
func (c *Cron) Location() *time.Location {
	return c.getLocation()
}

// SetLocation sets a new location to use.
// Re-set the "Next" values for all entries.
// Re-sort entries and run due entries.
func (c *Cron) SetLocation(newLoc *time.Location) {
	c.setLocation(newLoc)
}

//-----------------------------------------------------------------------------

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
	if err := c.entries.WithE(func(entries *[]*Entry) error {
		entry := utils.Find(*entries, findByIDFn(id))
		if entry == nil {
			return errors.New("not found")
		}
		(*entry).Next = c.now()
		c.sortEntries(entries)
		return nil
	}); err != nil {
		return
	}
	c.entriesUpdated() // runNow
}

func (c *Cron) getLocation() *time.Location {
	return c.location.Get()
}

func (c *Cron) setLocation(newLoc *time.Location) {
	c.location.Set(newLoc)
	c.setEntriesNext()
	c.entriesUpdated() // setLocation
}

func (c *Cron) addJob(spec string, cmd IntoJob, opts ...EntryOption) (EntryID, error) {
	schedule, err := Parse(spec)
	if err != nil {
		return "", err
	}
	return c.schedule(schedule, cmd, opts...)
}

func (c *Cron) schedule(schedule Schedule, cmd IntoJob, opts ...EntryOption) (EntryID, error) {
	entry := Entry{
		ID:       EntryID(uuid.New().String()),
		Schedule: schedule,
		job:      castIntoJob(cmd),
		Active:   true,
		Next:     schedule.Next(c.now()),
	}
	return c.addEntry(entry, opts...)
}

func (c *Cron) addEntry(entry Entry, opts ...EntryOption) (EntryID, error) {
	utils.ApplyOptions(&entry, opts)
	if c.entryExists(entry.ID) {
		return "", ErrIDAlreadyUsed
	}
	c.entries.With(func(entries *[]*Entry) { insertSorted(entries, &entry) })
	c.entriesUpdated() // addEntry
	return entry.ID, nil
}

func (c *Cron) setEntryActive(id EntryID, active bool) {
	if err := c.entries.WithE(func(entries *[]*Entry) error {
		entry := utils.Find(*entries, findByIDFn(id))
		if entry == nil {
			return errors.New("not found")
		}
		if (*entry).Active == active {
			return errors.New("unchanged")
		}
		(*entry).Active = active
		c.sortEntries(entries)
		return nil
	}); err != nil {
		return
	}
	c.entriesUpdated() // setEntryActive
}

func (c *Cron) sortEntries(entries *[]*Entry) {
	sort.Slice(*entries, func(i, j int) bool { return less((*entries)[i], (*entries)[j]) })
}

func insertSorted(entries *[]*Entry, entry *Entry) {
	i := sort.Search(len(*entries), func(i int) bool { return !less((*entries)[i], entry) })
	*entries = append(*entries, nil)
	copy((*entries)[i+1:], (*entries)[i:])
	(*entries)[i] = entry
}

func (c *Cron) remove(id EntryID) {
	c.removeEntry(id)
	c.entriesUpdated() // remove
}

func (c *Cron) getEntries() (out []Entry) {
	c.entries.RWith(func(entries []*Entry) {
		out = make([]Entry, len(entries))
		for i, e := range entries {
			out[i] = *e
		}
	})
	return
}

func (c *Cron) run() {
	for {
		// Determine the next entry to run.
		delay := c.getNextDelay()
		var updated context.CancelFunc
		select {
		case <-c.clock.After(delay):
		case updated = <-c.update:
		case <-c.ctx.Done():
			return
		}
		c.runDueEntries()
		if updated != nil {
			updated()
		}
		if c.ctx.Err() != nil {
			return
		}
	}
}

func (c *Cron) getNextDelay() (out time.Duration) {
	c.entries.RWith(func(entries []*Entry) {
		if len(entries) == 0 || entries[0].Next.IsZero() {
			out = 100_000 * time.Hour // If there are no entries yet, just sleep - it still handles new entries and stop requests.
		} else {
			out = entries[0].Next.Sub(c.now())
		}
	})
	return
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
		c.entries.With(func(entries *[]*Entry) {
			var toSortCount int
			for _, entry := range *entries {
				if entry.Next.After(now) || entry.Next.IsZero() || !entry.Active {
					break
				}
				entry.Prev = entry.Next
				entry.Next = entry.Schedule.Next(now) // Compute new Next property for the Entry
				c.startJob(*entry)
				toSortCount++
			}
			utils.InsertionSortPartial(*entries, toSortCount, less)
		})
	}
}

func (c *Cron) setEntriesNext() {
	now := c.now()
	c.entries.With(func(entries *[]*Entry) {
		for _, entry := range *entries {
			entry.Next = entry.Schedule.Next(now)
		}
		c.sortEntries(entries)
	})
}

func (c *Cron) removeEntry(id EntryID) {
	c.entries.With(func(entries *[]*Entry) {
		removeEntry(entries, id)
	})
}

func removeEntry(entries *[]*Entry, id EntryID) {
	if _, i := utils.FindIdx(*entries, findByIDFn(id)); i != -1 {
		*entries = slices.Delete(*entries, i, i+1)
	}
}

func (c *Cron) getEntry(id EntryID) (Entry, error) {
	entries := c.getEntries()
	if entry := utils.Find(entries, func(e Entry) bool { return e.ID == id }); entry != nil {
		return *entry, nil
	}
	return Entry{}, ErrEntryNotFound
}

func (c *Cron) entryExists(id EntryID) bool {
	return utils.Second(c.getEntry(id)) == nil
}

// startJob runs the given job in a new goroutine.
func (c *Cron) startJob(entry Entry) {
	c.runningJobsCount.Add(1)
	go func() {
		defer func() {
			c.runningJobsCount.Add(-1)
			c.signalJobCompleted()
		}()
		c.runWithRecovery(entry)
	}()
}

func (c *Cron) runWithRecovery(entry Entry) {
	defer func() {
		if r := recover(); r != nil {
			c.logger.Printf("%s\n", string(debug.Stack()))
		}
	}()
	if err := entry.job.Run(c.ctx, c, entry); err != nil {
		msg := fmt.Sprintf("error running job %s", entry.ID)
		msg += utils.TernaryOrZero(entry.Label != "", " "+entry.Label)
		msg += " : " + err.Error()
		c.logger.Print(msg)
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
