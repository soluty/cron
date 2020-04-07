package cron

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alaingilbert/clockwork"
	"github.com/stretchr/testify/assert"
)

func TestFuncPanicRecovery(t *testing.T) {
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	cron.Start()
	defer cron.Stop()
	nbCall := 0
	wg := new(sync.WaitGroup)
	wg.Add(1)
	_, _ = cron.AddFunc("* * * * *", func() {
		nbCall++
		wg.Done()
	})
	clock.Advance(61 * time.Second)
	wg.Wait()
	assert.Equal(t, 1, nbCall)
}

func TestFuncPanicRecovery1(t *testing.T) {
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	cron.Start()
	defer cron.Stop()
	nbCall := 0
	wg := new(sync.WaitGroup)
	wg.Add(1)
	_, _ = cron.AddFunc("* * * * * *", func() {
		nbCall++
		wg.Done()
	})
	clock.Advance(10 * time.Second)
	wg.Wait()
	assert.Equal(t, 1, nbCall)
}

type DummyJob struct{}

func (d DummyJob) Run() {
	panic("YOLO")
}

func TestJobPanicRecovery(t *testing.T) {
	var job DummyJob
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	cron.Start()
	defer cron.Stop()
	_, _ = cron.AddJob("* * * * * ?", job)
	cycle(cron)
	clock.Advance(time.Second)
	cycle(cron)
}

// Start and stop cron with no entries.
func TestNoEntries(t *testing.T) {
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	cron.Start()
	select {
	case <-time.After(time.Second):
		t.Fatal("expected cron will be stopped immediately")
	case <-cron.Stop().Done():
	}
}

//// Start, stop, then add an entry. Verify entry doesn't run.
//func TestStopCausesJobsToNotRun(t *testing.T) {
//	wg := &sync.WaitGroup{}
//	wg.Add(1)
//
//	clock := clockwork.NewRealClock()
//	cron := New(clock)
//	cron.Start()
//	cron.Stop()
//	_, _ = cron.AddFunc("* * * * * ?", func() { wg.Done() })
//
//	select {
//	case <-time.After(OneSecond):
//		// No job ran!
//	case <-wait(wg):
//		t.Fatal("expected stopped cron does not run any job")
//	}
//}

// Add a job, start cron, expect it runs.
func TestAddBeforeRunning(t *testing.T) {
	wg := &sync.WaitGroup{}
	wg.Add(1)

	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	_, _ = cron.AddFunc("* * * * * ?", func() { wg.Done() })
	cron.Start()
	defer cron.Stop()
	cycle(cron)
	clock.Advance(time.Second)
	cycle(cron)
	// Give cron 2 seconds to run our job (which is always activated).
	select {
	case <-time.After(time.Second):
		t.Fatal("expected job runs")
	case <-wait(wg):
	}
}

// Start cron, add a job, expect it runs.
func TestAddWhileRunning(t *testing.T) {
	wg := &sync.WaitGroup{}
	wg.Add(1)

	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	cron.Start()
	defer cron.Stop()
	_, _ = cron.AddFunc("* * * * * ?", func() { wg.Done() })
	clock.Advance(time.Second)
	cycle(cron)

	select {
	case <-time.After(3000 * time.Millisecond):
		t.Fatal("expected job runs")
	case <-wait(wg):
	}
}

// Test for #34. Adding a job after calling start results in multiple job invocations
func TestAddWhileRunningWithDelay(t *testing.T) {
	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	cron.Start()
	defer cron.Stop()
	clock.Advance(time.Second)
	cycle(cron)
	clock.Advance(time.Second)
	cycle(cron)
	clock.Advance(time.Second)
	cycle(cron)
	clock.Advance(time.Second)
	cycle(cron)
	clock.Advance(time.Second)
	cycle(cron)
	var calls = 0
	_, _ = cron.AddFunc("* * * * * *", func() { calls += 1 })
	clock.Advance(time.Second)
	cycle(cron)
	if calls != 1 {
		t.Errorf("called %d times, expected 1\n", calls)
	}
}

// Test timing with Entries.
func TestSnapshotEntries(t *testing.T) {
	wg := &sync.WaitGroup{}
	wg.Add(1)

	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	_, _ = cron.AddFunc("@every 2s", func() { wg.Done() })
	cron.Start()
	defer cron.Stop()
	clock.Advance(time.Second)
	cycle(cron)
	clock.Advance(time.Second)
	cycle(cron)
	// Cron should fire in 2 seconds. After 1 second, call Entries.
	cron.Entries()
	clock.Advance(time.Second)
	cycle(cron)
	// Even though Entries was called, the cron should fire at the 2 second mark.
	<-wait(wg)
}

// Test that the entries are correctly sorted.
// Add a bunch of long-in-the-future entries, and an immediate entry, and ensure
// that the immediate entry runs immediately.
// Also: Test that multiple jobs run in the same instant.
func TestMultipleEntries(t *testing.T) {
	wg := &sync.WaitGroup{}
	wg.Add(2)

	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	_, _ = cron.AddFunc("0 0 0 1 1 ?", func() {})
	_, _ = cron.AddFunc("* * * * * ?", func() { wg.Done() })
	_, _ = cron.AddFunc("0 0 0 31 12 ?", func() {})
	_, _ = cron.AddFunc("* * * * * ?", func() { wg.Done() })

	cron.Start()
	defer cron.Stop()
	cycle(cron)
	clock.Advance(time.Second)
	cycle(cron)
	<-wait(wg)
}

// Test running the same job twice.
func TestRunningJobTwice(t *testing.T) {
	wg := &sync.WaitGroup{}
	wg.Add(2)

	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	_, _ = cron.AddFunc("0 0 0 1 1 ?", func() {})
	_, _ = cron.AddFunc("0 0 0 31 12 ?", func() {})
	_, _ = cron.AddFunc("* * * * * ?", func() { wg.Done() })

	cron.Start()
	defer cron.Stop()
	cycle(cron)
	clock.Advance(time.Second)
	cycle(cron)
	clock.Advance(time.Second)
	cycle(cron)

	select {
	case <-time.After(time.Second):
		t.Error("expected job fires 2 times")
	case <-wait(wg):
	}
}

func TestRunningMultipleSchedules(t *testing.T) {
	wg := &sync.WaitGroup{}
	wg.Add(2)

	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	_, _ = cron.AddFunc("0 0 0 1 1 ?", func() {})
	_, _ = cron.AddFunc("0 0 0 31 12 ?", func() {})
	_, _ = cron.AddFunc("* * * * * ?", func() { wg.Done() })
	cron.Schedule(Every(time.Minute), FuncJob(func() {}))
	cron.Schedule(Every(time.Second), FuncJob(func() { wg.Done() }))
	cron.Schedule(Every(time.Hour), FuncJob(func() {}))

	cron.Start()
	defer cron.Stop()
	cycle(cron)
	clock.Advance(time.Second)
	cycle(cron)
	select {
	case <-time.After(time.Second):
		t.Error("expected job fires 2 times")
	case <-wait(wg):
	}
}

// Test that the cron is run in the local time zone (as opposed to UTC).
func TestLocalTimezone(t *testing.T) {
	wg := &sync.WaitGroup{}
	wg.Add(2)

	clock := clockwork.NewFakeClock()
	now := clock.Now().Local()
	spec := fmt.Sprintf("%d,%d %d %d %d %d ?",
		now.Second()+1, now.Second()+2, now.Minute(), now.Hour(), now.Day(), now.Month())

	cron := New(WithClock(clock))
	_, _ = cron.AddFunc(spec, func() { wg.Done() })
	cron.Start()
	defer cron.Stop()
	cycle(cron)
	clock.Advance(time.Second)
	cycle(cron)
	clock.Advance(time.Second)
	cycle(cron)

	select {
	case <-time.After(time.Second):
		t.Error("expected job fires 2 times")
	case <-wait(wg):
	}
}

// Test that the cron is run in the given time zone (as opposed to local).
func TestNonLocalTimezone(t *testing.T) {
	wg := &sync.WaitGroup{}
	wg.Add(2)

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
	_, _ = cron.AddFunc(spec, func() { wg.Done() })
	cron.Start()
	defer cron.Stop()
	cycle(cron)
	clock.Advance(time.Second)
	cycle(cron)
	clock.Advance(time.Second)
	cycle(cron)

	select {
	case <-time.After(time.Second):
		t.Error("expected job fires 2 times")
	case <-wait(wg):
	}
}

// Test that calling stop before start silently returns without
// blocking the stop channel.
func TestStopWithoutStart(t *testing.T) {
	clock := clockwork.NewRealClock()
	cron := New(WithClock(clock))
	cron.Stop()
}

type testJob struct {
	wg   *sync.WaitGroup
	name string
}

func (t testJob) Run() {
	t.wg.Done()
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
	wg := &sync.WaitGroup{}
	wg.Add(1)

	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	_, _ = cron.AddFunc("* * * * * ?", func() { wg.Done() })

	var unblockChan = make(chan struct{})

	go func() {
		cron.Run()
		close(unblockChan)
	}()
	defer cron.Stop()
	cycle(cron)
	clock.Advance(time.Second)
	cycle(cron)
	select {
	case <-time.After(time.Second):
		t.Error("expected job fires")
	case <-unblockChan:
		t.Error("expected that Run() blocks")
	case <-wait(wg):
	}
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

// Simple test using Runnables.
func TestJob(t *testing.T) {
	wg := &sync.WaitGroup{}
	wg.Add(1)

	clock := clockwork.NewFakeClock()
	cron := New(WithClock(clock))
	_, _ = cron.AddJob("0 0 0 30 Feb ?", testJob{wg, "job0"})
	_, _ = cron.AddJob("0 0 0 1 1 ?", testJob{wg, "job1"})
	_, _ = cron.AddJob("* * * * * ?", testJob{wg, "job2"})
	_, _ = cron.AddJob("1 0 0 1 1 ?", testJob{wg, "job3"})
	cron.Schedule(Every(5*time.Second+5*time.Nanosecond), testJob{wg, "job4"})
	cron.Schedule(Every(5*time.Minute), testJob{wg, "job5"})

	cron.Start()
	defer cron.Stop()
	cycle(cron)
	clock.Advance(time.Second)
	cycle(cron)
	select {
	case <-time.After(time.Second):
		t.FailNow()
	case <-wait(wg):
	}

	// Ensure the entries are in the right order.
	expecteds := []string{"job2", "job4", "job5", "job1", "job3", "job0"}

	actuals := make([]string, 0)
	for _, entry := range cron.Entries() {
		actuals = append(actuals, entry.Job.(testJob).name)
	}

	for i, expected := range expecteds {
		if actuals[i] != expected {
			t.Fatalf("Jobs not in the right order.  (expected) %s != %s (actual)", expecteds, actuals)
		}
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
	calls := 0
	_, _ = cron.AddFunc("* * * * * *", func() { calls += 1 })
	cron.Schedule(new(ZeroSchedule), FuncJob(func() { t.Error("expected zero task will not run") }))
	cron.Start()
	defer cron.Stop()
	cycle(cron)
	clock.Advance(time.Second)
	cycle(cron)
	if calls != 1 {
		t.Errorf("called %d times, expected 1\n", calls)
	}
}

func wait(wg *sync.WaitGroup) chan bool {
	ch := make(chan bool)
	go func() {
		wg.Wait()
		ch <- true
	}()
	return ch
}

// run one logic cycle
func cycle(cron *Cron) {
	cron.update <- struct{}{}
	time.Sleep(time.Millisecond)
}
