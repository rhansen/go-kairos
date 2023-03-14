// Package kairos is a Go timer implementation with a fixed Reset behavior.
package kairos

import (
	"time"
)

// The Timer type represents a single event. When the Timer expires,
// the current time will be sent on C, unless the Timer was created by AfterFunc.
// A Timer must be created with NewTimer. NewStoppedTimer or AfterFunc.
type Timer struct {
	C <-chan time.Time
	c chan<- time.Time // Same channel as C.

	i    int       // heap index.
	when time.Time // Timer wakes up at when.
}

// NewTimer creates a new Timer that will send the current time on its
// channel after at least duration d.
func NewTimer(d time.Duration) *Timer {
	return realClock.NewTimer(d)
}

// NewStoppedTimer creates a new stopped Timer.
func NewStoppedTimer() *Timer {
	return realClock.NewStoppedTimer()
}

// Stop prevents the Timer from firing.
// It returns true if the call stops the timer,
// false if the timer has already expired or been stopped.
// Stop does not close the channel, to prevent a read from
// the channel succeeding incorrectly.
func (t *Timer) Stop() (wasActive bool) {
	if t.c == nil {
		panic("timer: Stop called on uninitialized Timer")
	}
	return realClock.delTimer(t)
}

// Reset changes the timer to expire after duration d.
// It returns true if the timer had been active,
// false if the timer had expired or been stopped.
// The channel t.C is cleared and calling t.Reset() behaves as creating a
// new Timer.
func (t *Timer) Reset(d time.Duration) bool {
	if t.c == nil {
		panic("timer: Reset called on uninitialized Timer")
	}
	return realClock.resetTimer(t, d)
}
