package cron

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWithLocation(t *testing.T) {
	c := New(WithLocation(time.UTC))
	assert.Equal(t, time.UTC, c.location)
}
