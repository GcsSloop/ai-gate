package bootstrap

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestBackgroundLoopRecoversFromPanics(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tickCh := make(chan time.Time, 2)
	var runs atomic.Int32
	panicOnce := true

	done := make(chan struct{})
	go func() {
		defer close(done)
		runBackgroundLoop(ctx, tickCh, func(context.Context, time.Time) {
			runs.Add(1)
			if panicOnce {
				panicOnce = false
				panic("boom")
			}
		})
	}()

	tickCh <- time.Now()
	tickCh <- time.Now().Add(time.Second)

	deadline := time.After(200 * time.Millisecond)
	for runs.Load() < 2 {
		select {
		case <-deadline:
			t.Fatalf("background loop stopped after panic; runs=%d", runs.Load())
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("background loop did not stop after cancel")
	}
}
