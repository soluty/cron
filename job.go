package cron

import "context"

// Job is an interface for submitted cron jobs.
type Job interface {
	Run(context.Context, EntryID) error
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

type Job1Wrapper struct{ Job1 }

func (j *Job1Wrapper) Run(context.Context, EntryID) error {
	j.Job1.Run()
	return nil
}

type Job2Wrapper struct{ Job2 }

func (j *Job2Wrapper) Run(ctx context.Context, _ EntryID) error {
	j.Job2.Run(ctx)
	return nil
}

type Job3Wrapper struct{ Job3 }

func (j *Job3Wrapper) Run(_ context.Context, id EntryID) error {
	j.Job3.Run(id)
	return nil
}

type Job4Wrapper struct{ Job4 }

func (j *Job4Wrapper) Run(ctx context.Context, id EntryID) error {
	j.Job4.Run(ctx, id)
	return nil
}

type Job5Wrapper struct{ Job5 }

func (j *Job5Wrapper) Run(context.Context, EntryID) error { return j.Job5.Run() }

type Job6Wrapper struct{ Job6 }

func (j *Job6Wrapper) Run(ctx context.Context, _ EntryID) error { return j.Job6.Run(ctx) }

type Job7Wrapper struct{ Job7 }

func (j *Job7Wrapper) Run(_ context.Context, id EntryID) error { return j.Job7.Run(id) }
