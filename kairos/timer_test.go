package kairos

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"
)

// The observed passage of time can be greater than a timer's configured duration for many reasons:
//   - A timer can fire a bit late.
//   - The test code might be a bit slow when creating a new timer.
//   - The test code might be a bit slow when reacting to a fired timer.
//   - The resolution of the system clock might be poor.
//
// To accommodate these potential delays, this margin is added to the timer duration d to allow
// times in the half-open range [d, d+margin).  (Timers should never fire early, and elapsed time
// should never be negative, so the margin is not subtracted from the early side of the range, only
// added to the late side.)
const margin = 100 * time.Millisecond

func TestNullTimout(t *testing.T) {
	start := time.Now()
	timer := NewTimer(0)
	<-timer.C
	got := time.Since(start)
	if got >= margin {
		t.Errorf("timer fired too late; got duration %v, want 0s", got)
	}
}

func TestNegativeTimout(t *testing.T) {
	start := time.Now()
	timer := NewTimer(-1)
	<-timer.C
	got := time.Since(start)
	if got >= margin {
		t.Errorf("timer fired too late; got duration %v, want 0s", got)
	}

	start = time.Now()
	timer = NewTimer(-100 * time.Second)
	<-timer.C
	got = time.Since(start)
	if got >= margin {
		t.Errorf("timer fired too late; got duration %v, want 0s", got)
	}
}

func TestTimeValue(t *testing.T) {
	const want = time.Second
	start := time.Now()
	timer := NewTimer(want)
	end := <-timer.C
	got := end.Sub(start)
	if got < want || got >= want+margin {
		t.Errorf("reported time is wrong; got duration %v, want %v", got, want)
	}
}

func TestSingleTimout(t *testing.T) {
	const want = time.Second
	start := time.Now()
	timer := NewTimer(want)
	<-timer.C
	got := time.Since(start)
	if got < want || got >= want+margin {
		t.Errorf("timer fired at wrong time; got duration %v, want %v", got, want)
	}
}

func TestMultipleTimouts(t *testing.T) {
	const want = time.Second
	start := time.Now()
	var timers []*Timer

	for i := 0; i < 1000; i++ {
		timers = append(timers, NewTimer(want))
	}

	for _, timer := range timers {
		<-timer.C
	}
	got := time.Since(start)
	if got < want || got >= want+margin {
		t.Errorf("timer(s) fired at wrong time; got duration %v, want %v", got, want)
	}
}

func TestMultipleDifferentTimouts(t *testing.T) {
	start := time.Now()
	var timers []*Timer

	for i := 0; i < 1000; i++ {
		timers = append(timers, NewTimer(time.Duration(i%4)*time.Second))
	}

	for _, timer := range timers {
		<-timer.C
	}
	got := time.Since(start)
	const want = 3 * time.Second
	if got < want || got >= want+margin {
		t.Errorf("timer(s) fired at wrong time; got duration %v, want %v", got, want)
	}
}

func TestStoppedTimer(t *testing.T) {
	timer := NewStoppedTimer()
	if !timer.when.IsZero() {
		t.Errorf("invalid stopped timer when value")
	}

	const want = time.Second
	start := time.Now()
	wasActive := timer.Reset(want)
	if wasActive {
		t.Errorf("stopped timer: was active is true")
	}

	<-timer.C
	got := time.Since(start)
	if got < want || got >= want+margin {
		t.Errorf("timer fired at wrong time; got duration %v, want %v", got, want)
	}
}

func TestStop(t *testing.T) {
	timer := NewTimer(time.Second)
	wasActive := timer.Stop()
	if !wasActive {
		t.Errorf("stop timer: was active is false")
	}

	select {
	case <-timer.C:
		t.Errorf("failed to stop timer")
	case <-time.After(2 * time.Second):
	}

	wasActive = timer.Stop()
	if wasActive {
		t.Errorf("stop timer: was active is true")
	}
}

func TestStopPanic(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil || r.(string) != "timer: Stop called on uninitialized Timer" {
			t.Errorf("stop timer: invalid stop panic")
		}
	}()

	timer := &Timer{}
	timer.Stop()
}

func TestMultipleStop(t *testing.T) {
	var timers []*Timer

	for i := 0; i < 1000; i++ {
		timer := NewTimer(time.Second)
		wasActive := timer.Stop()
		if !wasActive {
			t.Errorf("stop timer: was active is false")
		}

		timers = append(timers, timer)
	}

	time.Sleep(2 * time.Second)

	// All channels must block.
	for _, timer := range timers {
		select {
		case <-timer.C:
			t.Errorf("failed to stop timer")
		default:
		}
	}

	for _, timer := range timers {
		wasActive := timer.Stop()
		if wasActive {
			t.Errorf("stop timer: was active is true")
		}
	}
}

func TestReset(t *testing.T) {
	want := 2 * time.Second
	start := time.Now()
	timer := NewTimer(want / 2)
	wasActive := timer.Reset(want)
	if !wasActive {
		t.Errorf("reset timer: was active is false")
	}

	<-timer.C
	got := time.Since(start)
	if got < want || got >= want+margin {
		t.Errorf("timer fired at wrong time; got duration %v, want %v", got, want)
	}

	want = time.Second
	start = time.Now()
	wasActive = timer.Reset(want)
	if wasActive {
		t.Errorf("reset timer: was active is true")
	}

	<-timer.C
	got = time.Since(start)
	if got < want || got >= want+margin {
		t.Errorf("timer fired at wrong time; got duration %v, want %v", got, want)
	}
}

func TestNegativeReset(t *testing.T) {
	start := time.Now()
	timer := NewTimer(time.Second)
	timer.Reset(-1)
	<-timer.C
	got := time.Since(start)
	if got >= margin {
		t.Errorf("timer fired too late; got duration %v, want 0s", got)
	}

	start = time.Now()
	timer = NewTimer(time.Second)
	timer.Reset(-100 * time.Second)
	<-timer.C
	got = time.Since(start)
	if got >= margin {
		t.Errorf("timer fired too late; got duration %v, want 0s", got)
	}
}

func TestMultipleResets(t *testing.T) {
	const want = 2 * time.Second
	start := time.Now()
	var timers []*Timer

	for i := 0; i < 1000; i++ {
		timer := NewTimer(want / 2)
		timers = append(timers, timer)
		timer.Reset(want)
	}

	for _, timer := range timers {
		<-timer.C
	}
	got := time.Since(start)
	if got < want || got >= want+margin {
		t.Errorf("timer fired at wrong time; got duration %v, want %v", got, want)
	}
}

func TestMultipleZeroResets(t *testing.T) {
	start := time.Now()
	var timers []*Timer

	for i := 0; i < 1000; i++ {
		timer := NewTimer(time.Second)
		timers = append(timers, timer)
		timer.Reset(0)
	}

	for _, timer := range timers {
		<-timer.C
	}
	got := time.Since(start)
	if got >= margin {
		t.Errorf("timer(s) fired too late; got duration %v, want 0s", got)
	}
}

func TestResetChannelClear(t *testing.T) {
	timer := NewTimer(0)
	time.Sleep(time.Second)

	if len(timer.C) != 1 {
		t.Errorf("reset timer: channel should be filled")
	}

	const want = 2 * time.Second
	start := time.Now()
	wasActive := timer.Reset(want)
	if wasActive {
		t.Errorf("reset timer: was active is true")
	}

	if len(timer.C) != 0 {
		t.Errorf("reset timer: channel should be empty")
	}

	<-timer.C
	got := time.Since(start)
	if got < want || got >= want+margin {
		t.Errorf("timer fired at wrong time; got duration %v, want %v", got, want)
	}
}

func TestResetPanic(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil || r.(string) != "timer: Reset called on uninitialized Timer" {
			t.Errorf("reset timer: invalid reset panic")
		}
	}()

	timer := &Timer{}
	timer.Reset(0)
}

func TestResetBehavior(t *testing.T) {
	const want = 3 * time.Second
	start := time.Now()

	timer := NewTimer(want / 3)

	// Let the timer fill the channel.
	time.Sleep(2 * want / 3)

	// Reset the timer without draining the channel manually.  The channel should be automatically
	// drained -- this should behave the same as creating a new timer except the same channel is
	// reused.
	timer.Reset(want / 3)

	// If timer was a *time.Timer, this receive would not block because the channel would not be
	// drained from the previous fire.  See <https://github.com/golang/go/issues/11513>.
	<-timer.C
	got := time.Since(start)
	if got < want || got >= want+margin {
		t.Errorf("timer fired at wrong time; got duration %v, want %v", got, want)
	}
}

func TestMultipleTimersForValidTimeouts(t *testing.T) {
	var gr errgroup.Group

	for i := 0; i < 1000; i++ {
		want := time.Duration(i%11) * time.Second
		start := time.Now()
		timer := NewTimer(want)
		gr.Go(func() error {
			<-timer.C
			got := time.Since(start)
			if got < want || got >= want+margin {
				return fmt.Errorf("timer fired at wrong time; got duration %v, want %v", got, want)
			}
			return nil
		})
	}

	if err := gr.Wait(); err != nil {
		t.Error(err)
	}
}

func TestMultipleTimersConcurrentAddRemove(t *testing.T) {
	var wg sync.WaitGroup

	for i := 0; i < 100000; i++ {
		timer := NewTimer(time.Nanosecond)
		wg.Add(1)
		go func() {
			<-timer.C
			wg.Done()
		}()
	}

	wg.Wait()
}
