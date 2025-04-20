package cron

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"github.com/soluty/cron/internal/pubsub"
	"log"
	"os"
	"runtime/debug"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/soluty/cron/internal/mtx"
	isync "github.com/soluty/cron/internal/sync"
	"github.com/soluty/cron/internal/utils"
	"github.com/jonboulle/clockwork"
)

// Cron keeps track of any number of entries, invoking the associated func as
// specified by the schedule. It may be started, stopped, and the entries may
// be inspected while running.
type Cron struct {
	clock                clockwork.Clock                              // Clock interface (real or mock) used for timing
	runningJobsCount     atomic.Int32                                 // Count of currently running jobs
	runningJobsMap       isync.Map[EntryID, *mtx.RWMtx[jobRunsInner]] // Keep track of currently running jobs
	cond                 sync.Cond                                    // Signals when all jobs have completed
	entries              mtx.RWMtx[entries]                           // Thread-safe, sorted list of job entries
	ctx                  context.Context                              // Context to control the scheduler lifecycle
	cancel               context.CancelFunc                           // Cancels the scheduler context
	update               chan context.CancelFunc                      // Triggers update in the scheduler loop
	running              atomic.Bool                                  // Indicates if the scheduler is currently running
	location             mtx.RWMtx[*time.Location]                    // Thread-safe time zone location
	logger               *log.Logger                                  // Logger for scheduler events, errors, and diagnostics
	parser               ScheduleParser                               // Parses cron expressions into schedule objects
	idFactory            EntryIDFactory                               // Generates a new unique EntryID for each scheduled job
	ps                   *pubsub.PubSub[EntryID, JobEvent]            //
	jobRunCreatedCh      chan *jobRunStruct                           //
	jobRunCompletedCh    chan *jobRunStruct                           //
	keepCompletedRunsDur mtx.Mtx[time.Duration]                       //
	lastCleanupTS        mtx.Mtx[time.Time]                           //
}

type jobRunsInner struct {
	mapping   map[RunID]*jobRunStruct
	running   []*jobRunStruct
	completed []*jobRunStruct
}

type entries struct {
	entriesHeap EntryHeap
	entriesMap  map[EntryID]*Entry
}

// ErrEntryNotFound ...
var ErrEntryNotFound = errors.New("entry not found")

// ErrRunNotFound ...
var ErrRunNotFound = errors.New("run not found")

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
		return EntryID(utils.UuidV4())
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
	keepCompletedRunsDur := utils.Default(cfg.KeepCompletedRunsDur, time.Minute)
	ctx, cancel := context.WithCancel(parentCtx)
	c := &Cron{
		cond:                 sync.Cond{L: &sync.Mutex{}},
		clock:                clock,
		ctx:                  ctx,
		cancel:               cancel,
		runningJobsMap:       isync.Map[EntryID, *mtx.RWMtx[jobRunsInner]]{},
		update:               make(chan context.CancelFunc),
		location:             mtx.NewRWMtx(location),
		logger:               logger,
		parser:               parser,
		idFactory:            idFactory,
		ps:                   pubsub.NewPubSub[EntryID, JobEvent](),
		jobRunCreatedCh:      make(chan *jobRunStruct),
		jobRunCompletedCh:    make(chan *jobRunStruct),
		keepCompletedRunsDur: mtx.NewMtx(keepCompletedRunsDur),
		entries: mtx.NewRWMtx(entries{
			entriesHeap: make(EntryHeap, 0),
			entriesMap:  make(map[EntryID]*Entry),
		}),
	}
	if keepCompletedRunsDur > 0 {
		startCleanupThread(c)
	}
	return c
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
	return c.schedule(nil, schedule, job, opts...)
}

// AddEntry ...
func (c *Cron) AddEntry(entry Entry, opts ...EntryOption) (EntryID, error) {
	return c.addEntry(entry, opts...)
}

// UpdateSchedule ...
func (c *Cron) UpdateSchedule(id EntryID, schedule Schedule) error {
	return c.updateSchedule(id, nil, schedule)
}

// UpdateScheduleWithSpec ...
func (c *Cron) UpdateScheduleWithSpec(id EntryID, spec string) error {
	return c.updateScheduleWithSpec(id, spec)
}

// UpdateLabel ...
func (c *Cron) UpdateLabel(id EntryID, label string) { c.updateLabel(id, label) }

// Sub subscribes to Job events
func (c *Cron) Sub(id EntryID) *pubsub.Sub[EntryID, JobEvent] {
	return c.ps.Subscribe([]EntryID{id})
}

// JobRunCreatedCh ...
func (c *Cron) JobRunCreatedCh() <-chan *jobRunStruct { return c.jobRunCreatedCh }

// JobRunCompletedCh ...
func (c *Cron) JobRunCompletedCh() <-chan *jobRunStruct { return c.jobRunCompletedCh }

// Enable ...
func (c *Cron) Enable(id EntryID) { c.setEntryActive(id, true) }

// Disable ...
func (c *Cron) Disable(id EntryID) { c.setEntryActive(id, false) }

// Entries returns a snapshot of the cron entries.
func (c *Cron) Entries() []Entry { return c.getEntries() }

// Entry returns a snapshot of the given entry, or nil if it couldn't be found.
func (c *Cron) Entry(id EntryID) (Entry, error) { return c.getEntry(id) }

// Remove an entry from being run in the future.
func (c *Cron) Remove(id EntryID) { c.remove(id) }

// RunNow allows the user to run a specific job now
func (c *Cron) RunNow(id EntryID) error { return c.runNow(id) }

// IsRunning either or not a specific job is currently running
func (c *Cron) IsRunning(id EntryID) bool { return c.entryIsRunning(id) }

// RunningJobs ...
func (c *Cron) RunningJobs() []JobRun { return c.runningJobs() }

// RunningJobsFor ...
func (c *Cron) RunningJobsFor(entryID EntryID) ([]JobRun, error) {
	return c.runningJobsFor(entryID)
}

// CompletedJobRunsFor ...
func (c *Cron) CompletedJobRunsFor(entryID EntryID) ([]JobRun, error) {
	return c.completedJobRunsFor(entryID)
}

// GetRun ...
func (c *Cron) GetRun(entryID EntryID, runID RunID) (JobRun, error) {
	return c.getRun(entryID, runID)
}

// CancelRun ...
func (c *Cron) CancelRun(entryID EntryID, runID RunID) error { return c.cancelRun(entryID, runID) }

// Location gets the time zone location
func (c *Cron) Location() *time.Location { return c.getLocation() }

// SetLocation sets a new location to use.
// Re-set the "Next" values for all entries.
// Re-sort entries and run due entries.
func (c *Cron) SetLocation(newLoc *time.Location) { c.setLocation(newLoc) }

// GetNextTime returns the next time a job is scheduled to be executed
// If no job is scheduled to be executed, the Zero time is returned
func (c *Cron) GetNextTime() time.Time { return c.getNextTime() }

// GetCleanupTS ...
func (c *Cron) GetCleanupTS() time.Time { return c.lastCleanupTS.Get() }

//-----------------------------------------------------------------------------

func startCleanupThread(c *Cron) {
	go func() {
		for {
			keepCompletedRunsDur := c.keepCompletedRunsDur.Get()
			select {
			case <-c.clock.After(keepCompletedRunsDur):
			case <-c.ctx.Done():
				return
			}
			for v := range c.runningJobsMap.IterValues() {
				v.With(func(inner *jobRunsInner) {
					for i := len(inner.completed) - 1; i >= 0; i-- {
						now := c.clock.Now()
						el := inner.completed[i]
						if el.inner.Get().completedAt.Before(now.Add(-keepCompletedRunsDur)) {
							delete(inner.mapping, el.runID)
							inner.completed = slices.Delete(inner.completed, i, i+1)
						}
					}
				})
			}
			c.lastCleanupTS.Set(c.clock.Now())
		}
	}()
}

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

func (c *Cron) runNow(id EntryID) error {
	if err := c.entries.WithE(func(entries *entries) error {
		if entry, idx := utils.FindIdx((*entries).entriesHeap, findByIDFn(id)); entry != nil {
			(*entry).Next = c.now()
			reInsertEntry(&entries.entriesHeap, idx) // runNow
			return nil
		}
		return ErrEntryNotFound
	}); err != nil {
		return err
	}
	c.entriesUpdated() // runNow
	return nil
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
	return c.schedule(&spec, schedule, job, opts...)
}

func (c *Cron) schedule(spec *string, schedule Schedule, job Job, opts ...EntryOption) (EntryID, error) {
	entry := Entry{
		ID:       c.idFactory.Next(),
		job:      job,
		Spec:     spec,
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
	var zeroID EntryID
	if entry.ID == zeroID {
		return zeroID, errors.New("id cannot be empty")
	}
	if !entry.Active {
		entry.Next = time.Time{}
	}
	if c.entryExists(entry.ID) {
		var zeroID EntryID
		return zeroID, ErrIDAlreadyUsed
	}
	c.entries.With(func(entries *entries) {
		entries.entriesMap[entry.ID] = &entry
		heap.Push(&entries.entriesHeap, &entry)
	})
	c.entriesUpdated() // addEntry
	return entry.ID, nil
}

func (c *Cron) updateLabel(id EntryID, label string) {
	c.entries.With(func(entries *entries) {
		if entry, ok := entries.entriesMap[id]; ok {
			(*entry).Label = label
		}
	})
}

func (c *Cron) setEntryActive(id EntryID, active bool) {
	if err := c.entries.WithE(func(entries *entries) error {
		entry, idx := utils.FindIdx((*entries).entriesHeap, findByIDFn(id))
		if entry == nil || (*entry).Active == active {
			return errors.New("not found or unchanged")
		}
		(*entry).Active = active
		(*entry).Next = utils.TernaryOrZero(active, (*entry).Schedule.Next(c.now()))
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
	return c.updateSchedule(id, &spec, schedule)
}

func (c *Cron) updateSchedule(id EntryID, spec *string, schedule Schedule) error {
	if err := c.entries.WithE(func(entries *entries) error {
		entry, idx := utils.FindIdx((*entries).entriesHeap, findByIDFn(id))
		if entry == nil {
			return ErrEntryNotFound
		}
		(*entry).Spec = spec
		(*entry).Schedule = schedule
		(*entry).Next = utils.TernaryOrZero((*entry).Active, schedule.Next(c.now()))
		reInsertEntry(&entries.entriesHeap, idx) // updateSchedule
		return nil
	}); err != nil {
		return err
	}
	c.entriesUpdated() // updateSchedule
	return nil
}

func (c *Cron) getNextTime() (out time.Time) {
	c.entries.RWith(func(entries entries) {
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
		c.entries.With(func(entries *entries) {
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
	c.entries.With(func(entries *entries) {
		for _, entry := range (*entries).entriesHeap {
			entry.Next = utils.TernaryOrZero(entry.Active, entry.Schedule.Next(now))
		}
		c.sortEntries(&entries.entriesHeap) // setEntriesNext
	})
}

func (c *Cron) remove(id EntryID) {
	c.removeEntry(id)
	c.entriesUpdated() // remove
}

func (c *Cron) removeEntry(id EntryID) {
	if jobRuns, ok := c.runningJobsMap.Load(id); ok {
		jobRuns.With(func(v *jobRunsInner) {
			for _, j := range v.running {
				j.cancel()
			}
		})
	}
	c.runningJobsMap.Delete(id)
	c.entries.With(func(entries *entries) {
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
	c.entries.With(func(entries *entries) {
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
	err = c.entries.RWithE(func(entries entries) error {
		if entry, ok := entries.entriesMap[id]; ok {
			out = *entry
			return nil
		}
		return ErrEntryNotFound
	})
	return out, err
}

func (c *Cron) entryIsRunning(id EntryID) bool {
	jobRuns, ok := c.runningJobsMap.Load(id)
	if !ok {
		return false
	}
	var count int
	jobRuns.RWith(func(v jobRunsInner) { count = len(v.running) })
	return count > 0
}

func (c *Cron) getRunClb(entryID EntryID, runID RunID, clb func(*jobRunStruct)) error {
	if jobRunsInnerMtx, ok := c.runningJobsMap.Load(entryID); ok {
		if err := jobRunsInnerMtx.RWithE(func(jobRunsIn jobRunsInner) error {
			if run, ok := jobRunsIn.mapping[runID]; ok {
				clb(run)
				return nil
			}
			return ErrRunNotFound
		}); err != nil {
			return err
		}
		return nil
	}
	return ErrRunNotFound
}

func (c *Cron) getRun(entryID EntryID, runID RunID) (JobRun, error) {
	var jobRunPub JobRun
	err := c.getRunClb(entryID, runID, func(run *jobRunStruct) {
		jobRunPub = run.Export()
	})
	return jobRunPub, err
}

func (c *Cron) cancelRun(entryID EntryID, runID RunID) error {
	return c.getRunClb(entryID, runID, func(run *jobRunStruct) {
		(*run).cancel()
	})
}

func (c *Cron) runningJobs() (out []JobRun) {
	for jobRuns := range c.runningJobsMap.IterValues() {
		jobRuns.RWith(func(v jobRunsInner) {
			out = append(out, exportJobRuns(v.running)...)
		})
	}
	sortJobRunsPublic(out)
	return
}

func (c *Cron) jobRunsForWith(entryID EntryID, clb func(jobRunsInner) []*jobRunStruct) (out []JobRun, err error) {
	if jobRuns, ok := c.runningJobsMap.Load(entryID); ok {
		jobRuns.RWith(func(v jobRunsInner) {
			out = exportJobRuns(clb(v))
		})
		sortJobRunsPublic(out)
		return
	}
	return nil, ErrEntryNotFound
}

func (c *Cron) runningJobsFor(entryID EntryID) (out []JobRun, err error) {
	return c.jobRunsForWith(entryID, func(v jobRunsInner) []*jobRunStruct { return v.running })
}

func (c *Cron) completedJobRunsFor(entryID EntryID) (out []JobRun, err error) {
	return c.jobRunsForWith(entryID, func(v jobRunsInner) []*jobRunStruct { return v.completed })
}

func exportJobRuns(runs []*jobRunStruct) (out []JobRun) {
	for _, j := range runs {
		out = append(out, j.Export())
	}
	return
}

func sortJobRunsPublic(runs []JobRun) {
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].CreatedAt.Before(runs[j].CreatedAt)
	})
}

func (c *Cron) entryExists(id EntryID) bool {
	return utils.Second(c.getEntry(id)) == nil
}

func (c *Cron) updateJobsCounter(key EntryID, jobRun *jobRunStruct, delta int32) {
	jobRuns, _ := c.runningJobsMap.LoadOrStore(key, utils.Ptr(mtx.NewRWMtx(jobRunsInner{
		mapping: make(map[RunID]*jobRunStruct),
	})))
	if delta == 1 {
		utils.NonBlockingSend(c.jobRunCreatedCh, jobRun)
		jobRuns.With(func(v *jobRunsInner) {
			(*v).mapping[jobRun.runID] = jobRun
			(*v).running = append((*v).running, jobRun)
		})
	} else {
		utils.NonBlockingSend(c.jobRunCompletedCh, jobRun)
		jobRuns.With(func(v *jobRunsInner) {
			if idx := slices.Index((*v).running, jobRun); idx != -1 {
				(*v).running = slices.Delete((*v).running, idx, idx+1)
				if c.keepCompletedRunsDur.Get() > 0 {
					(*v).completed = append((*v).completed, jobRun)
				} else {
					delete((*v).mapping, jobRun.runID)
				}
			}
		})
	}
}

// startJob runs the given job in a new goroutine.
func (c *Cron) startJob(entry Entry) {
	jobRun := newJobRun(c.ctx, c.clock, entry)
	c.runningJobsCount.Add(1)
	go func() {
		defer func() {
			c.updateJobsCounter(entry.ID, jobRun, -1)
			c.runningJobsCount.Add(-1)
			c.signalJobCompleted()
		}()
		c.updateJobsCounter(entry.ID, jobRun, 1)
		c.runWithRecovery(jobRun)
	}()
}

func (c *Cron) runWithRecovery(jobRun *jobRunStruct) {
	entry := jobRun.entry
	logger := c.logger
	defer func() {
		if r := recover(); r != nil {
			logger.Printf("%v\n%s\n", r, string(debug.Stack()))
			makeEvent(c, entry, jobRun, CompletedPanic)
		}
		makeEvent(c, entry, jobRun, Completed)
	}()
	makeEvent(c, entry, jobRun, Start)
	if err := entry.job.Run(jobRun.ctx, c, entry); err != nil {
		msg := fmt.Sprintf("error running job %s", entry.ID)
		msg += utils.TernaryOrZero(entry.Label != "", " "+entry.Label)
		msg += " : " + err.Error()
		logger.Println(msg)
		makeEventErr(c, entry, jobRun, CompletedErr, err)
	} else {
		makeEvent(c, entry, jobRun, CompletedNoErr)
	}
}

func makeEvent(c *Cron, entry Entry, jobRun *jobRunStruct, typ JobEventType) {
	makeEventErr(c, entry, jobRun, typ, nil)
}

func makeEventErr(c *Cron, entry Entry, jobRun *jobRunStruct, typ JobEventType, err error) {
	now := c.clock.Now()
	var opt func(*jobRunInner)
	switch typ {
	case Start:
		opt = func(inner *jobRunInner) { (*inner).startedAt = utils.Ptr(now) }
	case Completed:
		opt = func(inner *jobRunInner) { (*inner).completedAt = utils.Ptr(now) }
	case CompletedPanic:
		opt = func(inner *jobRunInner) { (*inner).panic = true }
	case CompletedErr:
		opt = func(inner *jobRunInner) { (*inner).error = err }
	case CompletedNoErr:
		opt = func(inner *jobRunInner) {}
	}
	evt := NewJobEvent(typ, jobRun)
	jobRun.inner.With(func(inner *jobRunInner) {
		utils.ApplyOptions(inner, opt)
		inner.addEvent(evt)
	})
	c.ps.Pub(entry.ID, evt)
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
