package scheduler

import (
	"context"
	"sync/atomic"
	"time"
)

type UsageCompactionFunc func(ctx context.Context, now time.Time) error

type UsageCompactionJob struct {
	run     UsageCompactionFunc
	running atomic.Bool
}

func NewUsageCompactionJob(run UsageCompactionFunc) *UsageCompactionJob {
	return &UsageCompactionJob{run: run}
}

func (j *UsageCompactionJob) Run(ctx context.Context, now time.Time) error {
	if j == nil || j.run == nil {
		return nil
	}
	if !j.running.CompareAndSwap(false, true) {
		return nil
	}
	defer j.running.Store(false)
	return j.run(ctx, now.UTC())
}
