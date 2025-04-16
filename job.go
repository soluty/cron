package cron

import "context"

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
	case func(context.Context) error:
		return FuncJob(func(ctx context.Context, _ *Cron, _ Entry) error { return j(ctx) })
	case func(context.Context, EntryID) error:
		return FuncJob(func(ctx context.Context, _ *Cron, e Entry) error {
			return j(ctx, e.ID)
		})
	case func(context.Context, EntryID):
		return FuncJob(func(ctx context.Context, _ *Cron, e Entry) error {
			j(ctx, e.ID)
			return nil
		})
	case func() error:
		return FuncJob(func(context.Context, *Cron, Entry) error { return j() })
	case func(context.Context):
		return FuncJob(func(ctx context.Context, _ *Cron, _ Entry) error {
			j(ctx)
			return nil
		})
	case func(id EntryID):
		return FuncJob(func(_ context.Context, _ *Cron, e Entry) error {
			j(e.ID)
			return nil
		})
	case func(id EntryID) error:
		return FuncJob(func(_ context.Context, _ *Cron, e Entry) error { return j(e.ID) })
	case func():
		return FuncJob(func(context.Context, *Cron, Entry) error {
			j()
			return nil
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
