package kairos

import (
	"context"
	"fmt"
	"math"
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

func TestNewTimer(t *testing.T) {
	repeat := func(n, mod, offset int, d time.Duration) []time.Duration {
		ds := make([]time.Duration, 0, n)
		for i := 0; i < n; i++ {
			ds = append(ds, time.Duration((i%mod)+offset)*d)
		}
		return ds
	}

	for _, tc := range []struct {
		desc   string
		ds     []time.Duration
		margin time.Duration
	}{
		{desc: "single, zero", ds: []time.Duration{0}},
		{desc: "single, negative small", ds: []time.Duration{-1 * time.Nanosecond}},
		{desc: "single, negative huge", ds: []time.Duration{math.MinInt64 * time.Nanosecond}},
		{desc: "single, positive", ds: []time.Duration{time.Second}},
		{desc: "many, same duration", ds: repeat(1000, 1, 1, time.Second)},     // 1s,1s,1s,1s,1s,1s,...
		{desc: "many, various durations", ds: repeat(1000, 4, 0, time.Second)}, // 0s,1s,2s,3s,0s,1s,...
		{desc: "many, even more variety", ds: repeat(1000, 11, 0, time.Second)},
		// The purpose of this test is to exercise concurrency, not load handling ability, so relax the
		// margin so that slow machines can handle all of the timers.
		{desc: "concurrency", ds: repeat(100000, 1, 1, time.Nanosecond), margin: 10 * time.Second},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			t.Cleanup(cancel)
			gr, ctx := errgroup.WithContext(ctx)
			if tc.margin == 0 {
				tc.margin = margin
			}
			start := time.Now()
			for _, d := range tc.ds {
				d := d
				want := d
				if want < 0 {
					want = 0
				}
				gr.Go(func() error {
					if err := ctx.Err(); err != nil {
						return err
					}
					// Created inside the goroutine to exercise thread safety.
					timer := NewTimer(d)
					select {
					case <-ctx.Done():
						return ctx.Err()
					case gotEndRx, ok := <-timer.C:
						if !ok {
							return fmt.Errorf("timer channel closed unexpectedly")
						}
						if got := time.Since(start); got < want || got >= want+tc.margin {
							return fmt.Errorf("timer fired at wrong time; got duration %v, want %v", got, want)
						}
						if got := gotEndRx.Sub(start); got < want || got >= want+tc.margin {
							return fmt.Errorf("reported time is wrong; got duration %v, want %v", got, want)
						}
						return nil
					}
				})
			}
			if err := gr.Wait(); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestStoppedTimer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	gr, ctx := errgroup.WithContext(ctx)
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

	gr.Go(func() error {
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
	if err := gr.Wait(); err != nil {
		t.Error(err)
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
	for _, tc := range []struct {
		desc     string
		initial  time.Duration // The timer's initial duration.
		prefire  time.Duration // The timer's reset duration immediately after timer creation.
		postfire time.Duration // The timer's reset duration after the prefire duration expires.
	}{
		{"zero", time.Second, 0, 0},
		{"positive", time.Second, 2 * time.Second, time.Second},
		{"negative", time.Second, -1 * time.Nanosecond, -100 * time.Second},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			t.Cleanup(cancel)
			timer := NewTimer(tc.initial)
			wantActive := true
			for _, d := range []time.Duration{tc.prefire, tc.postfire} {
				want := d
				if want < 0 {
					want = 0
				}
				start := time.Now()
				if gotActive := timer.Reset(d); gotActive != wantActive {
					t.Errorf("wrong timer.Reset return value; got %v, want %v", gotActive, wantActive)
				}
				wantActive = false
				var gr errgroup.Group
				gr.Go(func() error {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-timer.C:
						if got := time.Since(start); got < want || got >= want+margin {
							return fmt.Errorf("timer fired at wrong time; got duration %v, want %v", got, want)
						}
						return nil
					}
				})
				if err := gr.Wait(); err != nil {
					t.Error(err)
				}
			}
		})
	}
}

func TestMultipleResets(t *testing.T) {
	for _, d := range []time.Duration{2 * time.Second, 0, -1 * time.Second} {
		t.Run(fmt.Sprintf("%v", d), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			t.Cleanup(cancel)
			gr, ctx := errgroup.WithContext(ctx)
			want := d
			if want < 0 {
				want = 0
			}
			start := time.Now()
			for i := 0; i < 1000; i++ {
				gr.Go(func() error {
					timer := NewTimer(time.Second)
					timer.Reset(d)
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
		})
	}
}

func TestResetChannelClear(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	gr, ctx := errgroup.WithContext(ctx)
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

	gr.Go(func() error {
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
	if err := gr.Wait(); err != nil {
		t.Error(err)
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
