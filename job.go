package cron

import (
	"context"
	"github.com/alaingilbert/cron/internal/utils"
	"sync/atomic"
	"time"
)

// Job is an interface for submitted cron jobs.
type Job interface {
	Run(context.Context, *Cron, Entry) error
}

type Job1 interface{ Run() }
type Job2 interface{ Run(context.Context) }
type Job3 interface{ Run(EntryID) }
type Job4 interface {
	Run(context.Context, EntryID)
}
type Job5 interface{ Run() error }
type Job6 interface{ Run(context.Context) error }
type Job7 interface{ Run(EntryID) error }
type Job8 interface {
	Run(context.Context, EntryID) error
}
type Job9 interface{ Run(*Cron) }
type Job10 interface{ Run(*Cron) error }
type Job11 interface {
	Run(context.Context, *Cron)
}
type Job12 interface {
	Run(context.Context, *Cron) error
}
type Job13 interface {
	Run(*Cron, EntryID)
}
type Job14 interface {
	Run(*Cron, EntryID) error
}
type Job15 interface {
	Run(context.Context, *Cron, EntryID)
}
type Job16 interface {
	Run(context.Context, *Cron, EntryID) error
}

// FuncJob is a wrapper that turns a func() into a cron.Job
type FuncJob func(context.Context, *Cron, Entry) error

func (f FuncJob) Run(ctx context.Context, c *Cron, e Entry) error { return f(ctx, c, e) }

type Job1Wrapper struct{ Job1 }

func (j *Job1Wrapper) Run(context.Context, *Cron, Entry) error {
	j.Job1.Run()
	return nil
}

type Job2Wrapper struct{ Job2 }

func (j *Job2Wrapper) Run(ctx context.Context, _ *Cron, _ Entry) error {
	j.Job2.Run(ctx)
	return nil
}

type Job3Wrapper struct{ Job3 }

func (j *Job3Wrapper) Run(_ context.Context, _ *Cron, e Entry) error {
	j.Job3.Run(e.ID)
	return nil
}

type Job4Wrapper struct{ Job4 }

func (j *Job4Wrapper) Run(ctx context.Context, _ *Cron, e Entry) error {
	j.Job4.Run(ctx, e.ID)
	return nil
}

type Job5Wrapper struct{ Job5 }

func (j *Job5Wrapper) Run(context.Context, *Cron, Entry) error { return j.Job5.Run() }

type Job6Wrapper struct{ Job6 }

func (j *Job6Wrapper) Run(ctx context.Context, _ *Cron, _ Entry) error { return j.Job6.Run(ctx) }

type Job7Wrapper struct{ Job7 }

func (j *Job7Wrapper) Run(_ context.Context, _ *Cron, e Entry) error { return j.Job7.Run(e.ID) }

type Job8Wrapper struct{ Job8 }

func (j *Job8Wrapper) Run(ctx context.Context, _ *Cron, e Entry) error { return j.Job8.Run(ctx, e.ID) }

type Job9Wrapper struct{ Job9 }

func (j *Job9Wrapper) Run(_ context.Context, cron *Cron, _ Entry) error {
	j.Job9.Run(cron)
	return nil
}

type Job10Wrapper struct{ Job10 }

func (j *Job10Wrapper) Run(_ context.Context, cron *Cron, _ Entry) error {
	return j.Job10.Run(cron)
}

type Job11Wrapper struct{ Job11 }

func (j *Job11Wrapper) Run(ctx context.Context, cron *Cron, _ Entry) error {
	j.Job11.Run(ctx, cron)
	return nil
}

type Job12Wrapper struct{ Job12 }

func (j *Job12Wrapper) Run(ctx context.Context, cron *Cron, _ Entry) error {
	return j.Job12.Run(ctx, cron)
}

type Job13Wrapper struct{ Job13 }

func (j *Job13Wrapper) Run(_ context.Context, cron *Cron, e Entry) error {
	j.Job13.Run(cron, e.ID)
	return nil
}

type Job14Wrapper struct{ Job14 }

func (j *Job14Wrapper) Run(_ context.Context, cron *Cron, e Entry) error {
	return j.Job14.Run(cron, e.ID)
}

type Job15Wrapper struct{ Job15 }

func (j *Job15Wrapper) Run(ctx context.Context, cron *Cron, e Entry) error {
	j.Job15.Run(ctx, cron, e.ID)
	return nil
}

type Job16Wrapper struct{ Job16 }

func (j *Job16Wrapper) Run(ctx context.Context, cron *Cron, e Entry) error {
	return j.Job16.Run(ctx, cron, e.ID)
}

type IntoJob any

func castIntoJob(v IntoJob) Job {
	switch j := v.(type) {
	case func():
		return FuncJob(func(context.Context, *Cron, Entry) error {
			j()
			return nil
		})
	case func() error:
		return FuncJob(func(context.Context, *Cron, Entry) error {
			return j()
		})
	case func(context.Context):
		return FuncJob(func(ctx context.Context, _ *Cron, _ Entry) error {
			j(ctx)
			return nil
		})
	case func(context.Context) error:
		return FuncJob(func(ctx context.Context, _ *Cron, _ Entry) error { return j(ctx) })
	case func(EntryID):
		return FuncJob(func(_ context.Context, _ *Cron, e Entry) error {
			j(e.ID)
			return nil
		})
	case func(EntryID) error:
		return FuncJob(func(_ context.Context, _ *Cron, e Entry) error {
			return j(e.ID)
		})
	case func(Entry):
		return FuncJob(func(_ context.Context, _ *Cron, e Entry) error {
			j(e)
			return nil
		})
	case func(Entry) error:
		return FuncJob(func(_ context.Context, _ *Cron, e Entry) error {
			return j(e)
		})
	case func(*Cron):
		return FuncJob(func(_ context.Context, c *Cron, _ Entry) error {
			j(c)
			return nil
		})
	case func(*Cron) error:
		return FuncJob(func(_ context.Context, c *Cron, _ Entry) error {
			return j(c)
		})
	case func(context.Context, EntryID):
		return FuncJob(func(ctx context.Context, _ *Cron, e Entry) error {
			j(ctx, e.ID)
			return nil
		})
	case func(context.Context, EntryID) error:
		return FuncJob(func(ctx context.Context, _ *Cron, e Entry) error {
			return j(ctx, e.ID)
		})
	case func(context.Context, Entry):
		return FuncJob(func(ctx context.Context, _ *Cron, e Entry) error {
			j(ctx, e)
			return nil
		})
	case func(context.Context, Entry) error:
		return FuncJob(func(ctx context.Context, _ *Cron, e Entry) error {
			return j(ctx, e)
		})
	case func(context.Context, *Cron):
		return FuncJob(func(ctx context.Context, c *Cron, e Entry) error {
			j(ctx, c)
			return nil
		})
	case func(context.Context, *Cron) error:
		return FuncJob(func(ctx context.Context, c *Cron, e Entry) error {
			return j(ctx, c)
		})
	case func(*Cron, EntryID):
		return FuncJob(func(_ context.Context, c *Cron, e Entry) error {
			j(c, e.ID)
			return nil
		})
	case func(*Cron, EntryID) error:
		return FuncJob(func(_ context.Context, c *Cron, e Entry) error {
			return j(c, e.ID)
		})
	case func(*Cron, Entry):
		return FuncJob(func(_ context.Context, c *Cron, e Entry) error {
			j(c, e)
			return nil
		})
	case func(*Cron, Entry) error:
		return FuncJob(func(_ context.Context, c *Cron, e Entry) error {
			return j(c, e)
		})
	case func(context.Context, *Cron, Entry):
		return FuncJob(func(ctx context.Context, c *Cron, e Entry) error {
			j(ctx, c, e)
			return nil
		})
	case func(context.Context, *Cron, Entry) error:
		return FuncJob(func(ctx context.Context, c *Cron, e Entry) error {
			return j(ctx, c, e)
		})
	case func(context.Context, *Cron, EntryID):
		return FuncJob(func(ctx context.Context, c *Cron, e Entry) error {
			j(ctx, c, e.ID)
			return nil
		})
	case func(context.Context, *Cron, EntryID) error:
		return FuncJob(func(ctx context.Context, c *Cron, e Entry) error {
			return j(ctx, c, e.ID)
		})
	case Job:
		return j
	case Job1:
		return &Job1Wrapper{j}
	case Job2:
		return &Job2Wrapper{j}
	case Job3:
		return &Job3Wrapper{j}
	case Job4:
		return &Job4Wrapper{j}
	case Job5:
		return &Job5Wrapper{j}
	case Job6:
		return &Job6Wrapper{j}
	case Job7:
		return &Job7Wrapper{j}
	case Job8:
		return &Job8Wrapper{j}
	case Job9:
		return &Job9Wrapper{j}
	case Job10:
		return &Job10Wrapper{j}
	case Job11:
		return &Job11Wrapper{j}
	case Job12:
		return &Job12Wrapper{j}
	case Job13:
		return &Job13Wrapper{j}
	case Job14:
		return &Job14Wrapper{j}
	case Job15:
		return &Job15Wrapper{j}
	case Job16:
		return &Job16Wrapper{j}
	default:
		panic(ErrUnsupportedJobType)
	}
}

type JobWrapper func(IntoJob) Job

func Once(job IntoJob) Job {
	return FuncJob(func(ctx context.Context, c *Cron, e Entry) error {
		c.Remove(e.ID)
		return castIntoJob(job).Run(ctx, c, e)
	})
}

func NWrapper(n int) JobWrapper {
	return func(job IntoJob) Job {
		var count atomic.Int32
		return FuncJob(func(ctx context.Context, c *Cron, e Entry) error {
			if newCount := count.Add(1); int(newCount) == n {
				c.Remove(e.ID)
			}
			return castIntoJob(job).Run(ctx, c, e)
		})
	}
}

func ScheduledTime(job func(t time.Time)) Job {
	return FuncJob(func(ctx context.Context, c *Cron, e Entry) error {
		entry, err := c.Entry(e.ID)
		if err != nil {
			return err
		}
		job(entry.Prev)
		return nil
	})
}

// N runs a job "n" times
func N(n int, j IntoJob) Job {
	return NWrapper(n)(j)
}

// SkipIfStillRunning skips an invocation of the Job if a previous invocation is still running.
func SkipIfStillRunning(j IntoJob) Job {
	var running atomic.Bool
	return FuncJob(func(ctx context.Context, c *Cron, e Entry) (err error) {
		if running.CompareAndSwap(false, true) {
			defer running.Store(false)
			err = castIntoJob(j).Run(ctx, c, e)
		} else {
			return ErrJobAlreadyRunning
		}
		return
	})
}

// JitterWrapper add some random delay before running the job
func JitterWrapper(duration time.Duration) JobWrapper {
	return func(j IntoJob) Job {
		return FuncJob(func(ctx context.Context, c *Cron, e Entry) error {
			delay := utils.RandDuration(0, max(duration, 0))
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
			return castIntoJob(j).Run(ctx, c, e)
		})
	}
}

// WithJitter add some random delay before running the job
func WithJitter(duration time.Duration, job IntoJob) Job {
	return JitterWrapper(duration)(job)
}

// TimeoutWrapper automatically cancel the job context after a given duration
func TimeoutWrapper(duration time.Duration) JobWrapper {
	return func(j IntoJob) Job {
		return FuncJob(func(ctx context.Context, c *Cron, e Entry) error {
			timeoutCtx, cancel := context.WithTimeout(ctx, duration)
			defer cancel()
			return castIntoJob(j).Run(timeoutCtx, c, e)
		})
	}
}

// WithTimeout ...
// `_, _ = cron.AddJob("* * * * * *", cron.WithTimeout(time.Second, func(ctx context.Context) { ... }))`
func WithTimeout(d time.Duration, job IntoJob) Job {
	return TimeoutWrapper(d)(job)
}

func DeadlineWrapper(deadline time.Time) JobWrapper {
	return func(j IntoJob) Job {
		return FuncJob(func(ctx context.Context, c *Cron, e Entry) error {
			deadlineCtx, cancel := context.WithDeadline(ctx, deadline)
			defer cancel()
			return castIntoJob(j).Run(deadlineCtx, c, e)
		})
	}
}

func WithDeadline(deadline time.Time, job IntoJob) Job {
	return DeadlineWrapper(deadline)(job)
}

// Chain `Chain(j, w1, w2, w3)` -> `w3(w2(w1(j)))`
func Chain(j IntoJob, wrappers ...JobWrapper) Job {
	job := castIntoJob(j)
	for _, w := range wrappers {
		job = w(job)
	}
	return job
}
