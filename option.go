package cron

import (
	"context"
	"log"
	"time"

	"github.com/alaingilbert/clockwork"
)

// Config ...
type Config struct {
	Ctx      context.Context
	Location *time.Location
	Clock    clockwork.Clock
	Logger   *log.Logger
	Parser   ScheduleParser
}

// Option represents a modification to the default behavior of a Cron.
type Option func(*Config)

// WithLocation overrides the timezone of the cron instance.
func WithLocation(loc *time.Location) Option {
	return func(c *Config) {
		c.Location = loc
	}
}

// WithClock ...
func WithClock(clock clockwork.Clock) Option {
	return func(c *Config) {
		c.Clock = clock
	}
}

// WithContext ...
func WithContext(ctx context.Context) Option {
	return func(c *Config) {
		c.Ctx = ctx
	}
}

// WithLogger ...
func WithLogger(logger *log.Logger) Option {
	return func(c *Config) {
		c.Logger = logger
	}
}

// WithParser overrides the parser used for interpreting job schedules.
func WithParser(p ScheduleParser) Option {
	return func(c *Config) {
		c.Parser = p
	}
}
