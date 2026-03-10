package main

import (
	"context"
	"io"
	"testing"
	"time"
)

func waitForDone(ctx context.Context, timeout time.Duration) bool {
	select {
	case <-ctx.Done():
		return true
	case <-time.After(timeout):
		return false
	}
}

func TestMonitorParentHeartbeatCancelsWhenHeartbeatTimesOut(t *testing.T) {
	r, w := io.Pipe()
	defer r.Close()
	defer w.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go monitorParentHeartbeat(ctx, cancel, r, 40*time.Millisecond, 10*time.Millisecond)

	if _, err := w.Write([]byte("hb\n")); err != nil {
		t.Fatalf("write heartbeat: %v", err)
	}

	if !waitForDone(ctx, 300*time.Millisecond) {
		t.Fatal("expected context cancellation when heartbeat times out")
	}
}

func TestMonitorParentHeartbeatStaysAliveWithHeartbeatsThenCancelsOnEOF(t *testing.T) {
	r, w := io.Pipe()
	defer r.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go monitorParentHeartbeat(ctx, cancel, r, 80*time.Millisecond, 10*time.Millisecond)

	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(15 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				_, _ = w.Write([]byte("hb\n"))
			}
		}
	}()

	time.Sleep(120 * time.Millisecond)
	select {
	case <-ctx.Done():
		t.Fatal("context canceled unexpectedly while heartbeats are active")
	default:
	}

	close(stop)
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	if !waitForDone(ctx, 300*time.Millisecond) {
		t.Fatal("expected context cancellation after heartbeat pipe EOF")
	}
}
