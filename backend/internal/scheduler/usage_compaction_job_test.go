package scheduler_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/scheduler"
)

func TestUsageCompactionJobRunsCompaction(t *testing.T) {
	t.Parallel()

	var called atomic.Int32
	var got time.Time
	job := scheduler.NewUsageCompactionJob(func(_ context.Context, now time.Time) error {
		called.Add(1)
		got = now
		return nil
	})

	now := time.Date(2026, 3, 17, 10, 30, 0, 0, time.UTC)
	if err := job.Run(context.Background(), now); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if called.Load() != 1 {
		t.Fatalf("calls = %d, want 1", called.Load())
	}
	if !got.Equal(now) {
		t.Fatalf("run time = %v, want %v", got, now)
	}
}

func TestUsageCompactionJobSkipsWhenAlreadyRunning(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})
	var called atomic.Int32
	job := scheduler.NewUsageCompactionJob(func(_ context.Context, _ time.Time) error {
		called.Add(1)
		close(started)
		<-release
		return nil
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- job.Run(context.Background(), time.Date(2026, 3, 17, 10, 30, 0, 0, time.UTC))
	}()

	<-started
	if err := job.Run(context.Background(), time.Date(2026, 3, 17, 10, 31, 0, 0, time.UTC)); err != nil {
		t.Fatalf("second Run returned error: %v", err)
	}
	close(release)

	if err := <-errCh; err != nil {
		t.Fatalf("first Run returned error: %v", err)
	}
	if called.Load() != 1 {
		t.Fatalf("calls = %d, want 1", called.Load())
	}
}
