package cron

import (
	"time"

	"github.com/alaingilbert/clockwork"
)

// Option represents a modification to the default behavior of a Cron.
type Option func(*Cron)

// WithLocation overrides the timezone of the cron instance.
func WithLocation(loc *time.Location) Option {
	return func(c *Cron) {
		c.location = loc
	}
}

// WithClock ...
func WithClock(clock clockwork.Clock) Option {
	return func(c *Cron) {
		c.clock = clock
	}
}
