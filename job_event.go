package cron

import "time"

type JobEventType int

const (
	Start JobEventType = iota + 1
	Completed
	CompletedNoErr
	CompletedErr
	CompletedPanic
)

func (e JobEventType) String() string {
	switch e {
	case Start:
		return "Start"
	case Completed:
		return "Completed"
	case CompletedNoErr:
		return "CompletedNoErr"
	case CompletedErr:
		return "CompletedErr"
	case CompletedPanic:
		return "CompletedPanic"
	default:
		return "Unknown"
	}
}

type JobEvent struct {
	Typ       JobEventType
	JobRun    JobRun
	CreatedAt time.Time
}

func NewJobEvent(typ JobEventType, jobRun *jobRunStruct) JobEvent {
	return JobEvent{
		Typ:       typ,
		JobRun:    jobRun.Export(),
		CreatedAt: jobRun.clock.Now(),
	}
}
