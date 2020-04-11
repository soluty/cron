package cron

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alaingilbert/clockwork"
	"github.com/stretchr/testify/assert"
)

// run one logic cycle
func cycle(cron *Cron) {
	cron.entriesUpdated()
	time.Sleep(5 * time.Millisecond)
}

func advanceAndCycle(cron *Cron, d time.Duration) {
	cron.clock.(clockwork.FakeClock).Advance(d)
	cycle(cron)
}

func recvWithTimeout(t *testing.T, ch <-chan struct{}, msg ...string) {
	select {
	case <-time.After(time.Second):
		t.Fatal(msg)
	case <-ch:
	}
}

type DummyJob struct{}

func (d DummyJob) Run() {
	panic("YOLO")
}

func TestFuncPanicRecovery(t *testing.T) {
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	cron.Start()
	defer cron.Stop()
	var nbCall int32
	_, _ = cron.AddFunc("* * * * *", func() { atomic.AddInt32(&nbCall, 1) })
	cycle(cron)
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(1), atomic.LoadInt32(&nbCall))
}

func TestFuncPanicRecovery1(t *testing.T) {
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	cron.Start()
	defer cron.Stop()
	var nbCall int32
	_, _ = cron.AddFunc("* * * * * *", func() { atomic.AddInt32(&nbCall, 1) })
	cycle(cron)
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(1), atomic.LoadInt32(&nbCall))
}

func TestJobPanicRecovery(t *testing.T) {
	var job DummyJob
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	cron.Start()
	defer cron.Stop()
	_, _ = cron.AddJob("* * * * * ?", job)
	advanceAndCycle(cron, time.Second)
}

// Start and stop cron with no entries.
func TestNoEntries(t *testing.T) {
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	cron.Start()
	recvWithTimeout(t, cron.Stop().Done(), "expected cron will be stopped immediately")
}

// Start, stop, then add an entry. Verify entry doesn't run.
func TestStopCausesJobsToNotRun(t *testing.T) {
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	cron.Start()
	cron.Stop()
	var call int32
	_, _ = cron.AddFunc("* * * * * ?", func() { atomic.AddInt32(&call, 1) })
	cycle(cron)
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(0), atomic.LoadInt32(&call))
}

// Add a job, start cron, expect it runs.
func TestAddBeforeRunning(t *testing.T) {
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	var call int32
	_, _ = cron.AddFunc("* * * * * ?", func() { atomic.AddInt32(&call, 1) })
	cron.Start()
	defer cron.Stop()
	cycle(cron)
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(1), atomic.LoadInt32(&call))
}

// Start cron, add a job, expect it runs.
func TestAddWhileRunning(t *testing.T) {
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	cron.Start()
	defer cron.Stop()
	var call int32
	_, _ = cron.AddFunc("* * * * * ?", func() { atomic.AddInt32(&call, 1) })
	cycle(cron)
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(1), atomic.LoadInt32(&call))
}

// Test for #34. Adding a job after calling start results in multiple job invocations
func TestAddWhileRunningWithDelay(t *testing.T) {
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	cron.Start()
	defer cron.Stop()
	advanceAndCycle(cron, time.Second)
	advanceAndCycle(cron, time.Second)
	advanceAndCycle(cron, time.Second)
	var calls int32
	_, _ = cron.AddFunc("* * * * * *", func() { atomic.AddInt32(&calls, 1) })
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

// Test timing with Entries.
func TestSnapshotEntries(t *testing.T) {
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	var calls int32
	_, _ = cron.AddFunc("@every 2s", func() { atomic.AddInt32(&calls, 1) })
	cron.Start()
	defer cron.Stop()
	advanceAndCycle(cron, time.Second)
	advanceAndCycle(cron, time.Second)
	// Cron should fire in 2 seconds. After 1 second, call Entries.
	cron.Entries()
	advanceAndCycle(cron, time.Second)
	// Even though Entries was called, the cron should fire at the 2 second mark.
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestEntry(t *testing.T) {
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	id, _ := cron.AddFunc("@every 2s", func() {})
	cron.Start()
	defer cron.Stop()
	entry := cron.Entry(id)
	assert.Equal(t, id, entry.ID)
}

func TestUnexistingEntry(t *testing.T) {
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	_, _ = cron.AddFunc("@every 2s", func() {})
	cron.Start()
	defer cron.Stop()
	entry := cron.Entry(EntryID(123))
	assert.Equal(t, EntryID(0), entry.ID)
}

// Test that the entries are correctly sorted.
// Add a bunch of long-in-the-future entries, and an immediate entry, and ensure
// that the immediate entry runs immediately.
// Also: Test that multiple jobs run in the same instant.
func TestMultipleEntries(t *testing.T) {
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	var calls int32
	_, _ = cron.AddFunc("0 0 0 1 1 ?", func() {})
	_, _ = cron.AddFunc("* * * * * ?", func() { atomic.AddInt32(&calls, 1) })
	_, _ = cron.AddFunc("0 0 0 31 12 ?", func() {})
	_, _ = cron.AddFunc("* * * * * ?", func() { atomic.AddInt32(&calls, 1) })
	cron.Start()
	defer cron.Stop()
	cycle(cron)
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

// Test running the same job twice.
func TestRunningJobTwice(t *testing.T) {
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	var calls int32
	_, _ = cron.AddFunc("0 0 0 1 1 ?", func() {})
	_, _ = cron.AddFunc("0 0 0 31 12 ?", func() {})
	_, _ = cron.AddFunc("* * * * * ?", func() { atomic.AddInt32(&calls, 1) })
	cron.Start()
	defer cron.Stop()
	cycle(cron)
	advanceAndCycle(cron, time.Second)
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

func TestRunningMultipleSchedules(t *testing.T) {
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	var calls int32
	_, _ = cron.AddFunc("0 0 0 1 1 ?", func() {})
	_, _ = cron.AddFunc("0 0 0 31 12 ?", func() {})
	_, _ = cron.AddFunc("* * * * * ?", func() { atomic.AddInt32(&calls, 1) })
	cron.Schedule(Every(time.Minute), FuncJob(func() {}))
	cron.Schedule(Every(time.Second), FuncJob(func() { atomic.AddInt32(&calls, 1) }))
	cron.Schedule(Every(time.Hour), FuncJob(func() {}))
	cron.Start()
	defer cron.Stop()
	cycle(cron)
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

// Test that the cron is run in the local time zone (as opposed to UTC).
func TestLocalTimezone(t *testing.T) {
	clock := clockwork.NewFakeClock()
	now := clock.Now().Local()
	spec := fmt.Sprintf("%d,%d %d %d %d %d ?",
		now.Second()+1, now.Second()+2, now.Minute(), now.Hour(), now.Day(), now.Month())
	cron := New(WithClock(clock))
	var calls int32
	_, _ = cron.AddFunc(spec, func() { atomic.AddInt32(&calls, 1) })
	cron.Start()
	defer cron.Stop()
	cycle(cron)
	advanceAndCycle(cron, time.Second)
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

// Test that the cron is run in the given time zone (as opposed to local).
func TestNonLocalTimezone(t *testing.T) {
	clock := clockwork.NewFakeClock()
	loc, err := time.LoadLocation("Atlantic/Cape_Verde")
	if err != nil {
		fmt.Printf("Failed to load time zone Atlantic/Cape_Verde: %+v", err)
		t.Fail()
	}
	now := clock.Now().In(loc)
	spec := fmt.Sprintf("%d,%d %d %d %d %d ?",
		now.Second()+1, now.Second()+2, now.Minute(), now.Hour(), now.Day(), now.Month())
	cron := New(WithClock(clock), WithLocation(loc))
	var calls int32
	_, _ = cron.AddFunc(spec, func() { atomic.AddInt32(&calls, 1) })
	cron.Start()
	defer cron.Stop()
	cycle(cron)
	advanceAndCycle(cron, time.Second)
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

// Test that calling stop before start silently returns without
// blocking the stop channel.
func TestStopWithoutStart(t *testing.T) {
	clock := clockwork.NewRealClock()
	cron := New(WithClock(clock))
	cron.Stop()
}

type testJob struct {
	calls *int32
	name  string
}

func (t testJob) Run() {
	atomic.AddInt32(t.calls, 1)
}

// Test that adding an invalid job spec returns an error
func TestInvalidJobSpec(t *testing.T) {
	clock := clockwork.NewRealClock()
	cron := New(WithClock(clock))
	_, err := cron.AddJob("this will not parse", nil)
	if err == nil {
		t.Errorf("expected an error with invalid spec, got nil")
	}
}

// Test blocking run method behaves as Start()
func TestBlockingRun(t *testing.T) {
	var calls int32
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	_, _ = cron.AddFunc("* * * * * ?", func() { atomic.AddInt32(&calls, 1) })
	go func() {
		cron.Run()
		atomic.AddInt32(&calls, 1)
	}()
	defer cron.Stop()
	cycle(cron)
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

// Test that double-running Run is a no-op
func TestBlockingRunNoop(t *testing.T) {
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	go cron.Run()
	defer cron.Stop()
	time.Sleep(time.Millisecond)
	assert.False(t, cron.Run())
}

// Test that double-running is a no-op
func TestStartNoop(t *testing.T) {
	clock := clockwork.NewRealClock()
	cron := New(WithClock(clock))
	started := cron.Start()
	defer cron.Stop()
	assert.True(t, started)
	started = cron.Start()
	assert.False(t, started)
}

// TestChangeLocationWhileRunning ...
func TestChangeLocationWhileRunning(t *testing.T) {
	newLoc, _ := time.LoadLocation("Atlantic/Cape_Verde")
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock), WithLocation(time.UTC))
	cron.Start()
	defer cron.Stop()
	_, _ = cron.AddFunc("* * * * * ?", func() {})
	_, _ = cron.AddFunc("0 0 1 * * ?", func() {})
	assert.Equal(t, clock.Now().Add(time.Second).In(time.UTC), cron.entries[0].Next)
	assert.Equal(t, time.Date(1984, time.April, 4, 1, 0, 0, 0, time.UTC), cron.entries[1].Next)
	cron.SetLocation(newLoc)
	assert.Equal(t, clock.Now().Add(time.Second).In(newLoc), cron.entries[0].Next)
	assert.Equal(t, time.Date(1984, time.April, 4, 2, 0, 0, 0, time.UTC).In(newLoc), cron.entries[1].Next)
	assert.Equal(t, time.Date(1984, time.April, 4, 1, 0, 0, 0, newLoc), cron.entries[1].Next)
}

// Simple test using Runnables.
func TestJob(t *testing.T) {
	var calls int32
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	_, _ = cron.AddJob("0 0 0 30 Feb ?", testJob{&calls, "job0"})
	_, _ = cron.AddJob("0 0 0 1 1 ?", testJob{&calls, "job1"})
	_, _ = cron.AddJob("* * * * * ?", testJob{&calls, "job2"})
	_, _ = cron.AddJob("1 0 0 1 1 ?", testJob{&calls, "job3"})
	cron.Schedule(Every(5*time.Second+5*time.Nanosecond), testJob{&calls, "job4"})
	cron.Schedule(Every(5*time.Minute), testJob{&calls, "job5"})
	cron.Start()
	defer cron.Stop()
	cycle(cron)
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
	// Ensure the entries are in the right order.
	expecteds := []string{"job2", "job4", "job5", "job1", "job3", "job0"}
	for i, entry := range cron.Entries() {
		assert.Equal(t, expecteds[i], entry.Job.(testJob).name)
	}
}

type ZeroSchedule struct{}

func (*ZeroSchedule) Next(time.Time) time.Time {
	return time.Time{}
}

// Tests that job without time does not run
func TestJobWithZeroTimeDoesNotRun(t *testing.T) {
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	var calls int32
	_, _ = cron.AddFunc("* * * * * *", func() { atomic.AddInt32(&calls, 1) })
	cron.Schedule(new(ZeroSchedule), FuncJob(func() { t.Error("expected zero task will not run") }))
	cron.Start()
	defer cron.Stop()
	cycle(cron)
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestRemove(t *testing.T) {
	var calls int32
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	id, _ := cron.AddFunc("* * * * * *", func() { atomic.AddInt32(&calls, 1) })
	assert.Equal(t, int32(0), atomic.LoadInt32(&calls))
	cron.Start()
	defer cron.Stop()
	cycle(cron)
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
	advanceAndCycle(cron, 500*time.Millisecond)
	cron.Remove(id)
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
	advanceAndCycle(cron, 500*time.Millisecond)
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

// Issue #206
// Ensure that the next run of a job after removing an entry is accurate.
func TestScheduleAfterRemoval(t *testing.T) {
	// The first time this job is run, set a timer and remove the other job
	// 750ms later. Correct behavior would be to still run the job again in
	// 250ms, but the bug would cause it to run instead 1s later.
	var calls int32
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	hourJob := cron.Schedule(Every(time.Hour), FuncJob(func() {}))
	cron.Schedule(Every(time.Second), FuncJob(func() {
		switch atomic.LoadInt32(&calls) {
		case 0:
			atomic.AddInt32(&calls, 1)
		case 1:
			atomic.AddInt32(&calls, 1)
			advanceAndCycle(cron, 750*time.Millisecond)
			cron.Remove(hourJob)
			cycle(cron)
		case 2:
			atomic.AddInt32(&calls, 1)
		case 3:
			panic("unexpected 3rd call")
		}
	}))
	cron.Start()
	defer cron.Stop()
	cycle(cron)
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
	advanceAndCycle(cron, 250*time.Millisecond)
	assert.Equal(t, int32(3), atomic.LoadInt32(&calls))
}

func TestTwoCrons(t *testing.T) {
	var calls int32
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	_, _ = cron.AddFunc("1 * * * * *", func() { atomic.AddInt32(&calls, 1) })
	_, _ = cron.AddFunc("3 * * * * *", func() { atomic.AddInt32(&calls, 1) })
	assert.Equal(t, int32(0), atomic.LoadInt32(&calls))
	cron.Start()
	defer cron.Stop()
	cycle(cron)
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

func TestMultipleCrons(t *testing.T) {
	var calls int32
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	_, _ = cron.AddFunc("1 * * * * *", func() { atomic.AddInt32(&calls, 1) })
	_, _ = cron.AddFunc("* * * * * *", func() { atomic.AddInt32(&calls, 1) })
	_, _ = cron.AddFunc("3 * * * * *", func() { atomic.AddInt32(&calls, 1) })
	assert.Equal(t, int32(0), atomic.LoadInt32(&calls))
	cron.Start()
	defer cron.Stop()
	cycle(cron)
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(3), atomic.LoadInt32(&calls))
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(5), atomic.LoadInt32(&calls))
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(6), atomic.LoadInt32(&calls))
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(7), atomic.LoadInt32(&calls))
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(8), atomic.LoadInt32(&calls))
}

func TestSetEntriesNext(t *testing.T) {
	var calls int32
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	_, _ = cron.AddFunc("*/5 * * * * *", func() { atomic.AddInt32(&calls, 1) })
	assert.Equal(t, int32(0), atomic.LoadInt32(&calls))
	advanceAndCycle(cron, 58*time.Second)
	cron.Start()
	defer cron.Stop()
	assert.Equal(t, int32(0), atomic.LoadInt32(&calls))
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(0), atomic.LoadInt32(&calls))
	advanceAndCycle(cron, time.Second)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestNextIDIsThreadSafe(t *testing.T) {
	var calls int32
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	wg := sync.WaitGroup{}
	wg.Add(1000)
	for i := 0; i < 1000; i++ {
		go func() {
			defer wg.Done()
			_, _ = cron.AddFunc("*/5 * * * * *", func() { atomic.AddInt32(&calls, 1) })
		}()
	}
	wg.Wait()
	m := make(map[EntryID]bool)
	for _, e := range cron.entries {
		if _, ok := m[e.ID]; ok {
			t.Fail()
			return
		}
		m[e.ID] = true
	}
}
