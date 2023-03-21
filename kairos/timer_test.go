package kairos

import (
	"context"
	"fmt"
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	start := time.Now()
	timer := NewTimer(0)
	select {
	case <-ctx.Done():
		t.Error(ctx.Err())
	case <-timer.C:
		got := time.Since(start)
		if got >= margin {
			t.Errorf("timer fired too late; got duration %v, want 0s", got)
		}
	}
}

func TestNegativeTimout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	start := time.Now()
	timer := NewTimer(-1)
	select {
	case <-ctx.Done():
		t.Error(ctx.Err())
	case <-timer.C:
		got := time.Since(start)
		if got >= margin {
			t.Errorf("timer fired too late; got duration %v, want 0s", got)
		}
	}

	start = time.Now()
	timer = NewTimer(-100 * time.Second)
	select {
	case <-ctx.Done():
		t.Error(ctx.Err())
	case <-timer.C:
		got := time.Since(start)
		if got >= margin {
			t.Errorf("timer fired too late; got duration %v, want 0s", got)
		}
	}
}

func TestTimeValue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	const want = time.Second
	start := time.Now()
	timer := NewTimer(want)
	select {
	case <-ctx.Done():
		t.Error(ctx.Err())
	case end := <-timer.C:
		got := end.Sub(start)
		if got < want || got >= want+margin {
			t.Errorf("reported time is wrong; got duration %v, want %v", got, want)
		}
	}
}

func TestSingleTimout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	const want = time.Second
	start := time.Now()
	timer := NewTimer(want)
	select {
	case <-ctx.Done():
		t.Error(ctx.Err())
	case <-timer.C:
		got := time.Since(start)
		if got < want || got >= want+margin {
			t.Errorf("timer fired at wrong time; got duration %v, want %v", got, want)
		}
	}
}

func TestMultipleTimouts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	gr, ctx := errgroup.WithContext(ctx)
	const want = time.Second
	start := time.Now()
	for i := 0; i < 1000; i++ {
		gr.Go(func() error {
			timer := NewTimer(want)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				got := time.Since(start)
				if got < want || got >= want+margin {
					return fmt.Errorf("timer(s) fired at wrong time; got duration %v, want %v", got, want)
				}
				return nil
			}
		})
	}
	if err := gr.Wait(); err != nil {
		t.Error(err)
	}
}

func TestMultipleDifferentTimouts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	gr, ctx := errgroup.WithContext(ctx)
	start := time.Now()
	for i := 0; i < 1000; i++ {
		want := time.Duration(i%4) * time.Second
		gr.Go(func() error {
			timer := NewTimer(want)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				got := time.Since(start)
				if got < want || got >= want+margin {
					return fmt.Errorf("timer fired at wrong time; got duration %v, want %v", got, want)
				}
				return nil
			}
		})
	}
	if err := gr.Wait(); err != nil {
		t.Error(err)
	}
}

func TestStoppedTimer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
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

	select {
	case <-ctx.Done():
		t.Error(ctx.Err())
	case <-timer.C:
		got := time.Since(start)
		if got < want || got >= want+margin {
			t.Errorf("timer fired at wrong time; got duration %v, want %v", got, want)
		}
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	want := 2 * time.Second
	start := time.Now()
	timer := NewTimer(want / 2)
	wasActive := timer.Reset(want)
	if !wasActive {
		t.Errorf("reset timer: was active is false")
	}

	select {
	case <-ctx.Done():
		t.Error(ctx.Err())
	case <-timer.C:
		got := time.Since(start)
		if got < want || got >= want+margin {
			t.Errorf("timer fired at wrong time; got duration %v, want %v", got, want)
		}
	}

	want = time.Second
	start = time.Now()
	wasActive = timer.Reset(want)
	if wasActive {
		t.Errorf("reset timer: was active is true")
	}

	select {
	case <-ctx.Done():
		t.Error(ctx.Err())
	case <-timer.C:
		got := time.Since(start)
		if got < want || got >= want+margin {
			t.Errorf("timer fired at wrong time; got duration %v, want %v", got, want)
		}
	}
}

func TestNegativeReset(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	start := time.Now()
	timer := NewTimer(time.Second)
	timer.Reset(-1)
	select {
	case <-ctx.Done():
		t.Error(ctx.Err())
	case <-timer.C:
		got := time.Since(start)
		if got >= margin {
			t.Errorf("timer fired too late; got duration %v, want 0s", got)
		}
	}

	start = time.Now()
	timer = NewTimer(time.Second)
	timer.Reset(-100 * time.Second)
	select {
	case <-ctx.Done():
		t.Error(ctx.Err())
	case <-timer.C:
		got := time.Since(start)
		if got >= margin {
			t.Errorf("timer fired too late; got duration %v, want 0s", got)
		}
	}
}

func TestMultipleResets(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	gr, ctx := errgroup.WithContext(ctx)
	const want = 2 * time.Second
	start := time.Now()
	for i := 0; i < 1000; i++ {
		gr.Go(func() error {
			timer := NewTimer(want / 2)
			timer.Reset(want)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				got := time.Since(start)
				if got < want || got >= want+margin {
					return fmt.Errorf("timer fired at wrong time; got duration %v, want %v", got, want)
				}
				return nil
			}
		})
	}
	if err := gr.Wait(); err != nil {
		t.Error(err)
	}
}

func TestMultipleZeroResets(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	gr, ctx := errgroup.WithContext(ctx)
	start := time.Now()
	for i := 0; i < 1000; i++ {
		gr.Go(func() error {
			timer := NewTimer(time.Second)
			timer.Reset(0)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				got := time.Since(start)
				if got >= margin {
					return fmt.Errorf("timer fired too late; got duration %v, want 0s", got)
				}
				return nil
			}
		})
	}
	if err := gr.Wait(); err != nil {
		t.Error(err)
	}
}

func TestResetChannelClear(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
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

	select {
	case <-ctx.Done():
		t.Error(ctx.Err())
	case <-timer.C:
		got := time.Since(start)
		if got < want || got >= want+margin {
			t.Errorf("timer fired at wrong time; got duration %v, want %v", got, want)
		}
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	const want = 3 * time.Second
	start := time.Now()

	timer := NewTimer(want / 3)

	// Let the timer fill the channel.
	time.Sleep(2 * want / 3)

	// Reset the timer without draining the channel manually.  The channel should be automatically
	// drained -- this should behave the same as creating a new timer except the same channel is
	// reused.
	timer.Reset(want / 3)

	select {
	case <-ctx.Done():
		t.Error(ctx.Err())
	// If timer was a *time.Timer, this receive would not block because the channel would not be
	// drained from the previous fire.  See <https://github.com/golang/go/issues/11513>.
	case <-timer.C:
		got := time.Since(start)
		if got < want || got >= want+margin {
			t.Errorf("timer fired at wrong time; got duration %v, want %v", got, want)
		}
	}
}

func TestMultipleTimersForValidTimeouts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	gr, ctx := errgroup.WithContext(ctx)
	start := time.Now()
	for i := 0; i < 1000; i++ {
		want := time.Duration(i%11) * time.Second
		gr.Go(func() error {
			timer := NewTimer(want)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				got := time.Since(start)
				if got < want || got >= want+margin {
					return fmt.Errorf("timer fired at wrong time; got duration %v, want %v", got, want)
				}
				return nil
			}
		})
	}
	if err := gr.Wait(); err != nil {
		t.Error(err)
	}
}

func TestMultipleTimersConcurrentAddRemove(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	gr, ctx := errgroup.WithContext(ctx)

	for i := 0; i < 100000; i++ {
		gr.Go(func() error {
			timer := NewTimer(time.Nanosecond)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		})
	}

	if err := gr.Wait(); err != nil {
		t.Error(err)
	}
}
