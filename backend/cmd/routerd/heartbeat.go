package main

import (
	"bufio"
	"context"
	"io"
	"strings"
	"sync"
	"time"
)

const heartbeatToken = "hb"

type heartbeatState struct {
	mu   sync.RWMutex
	last time.Time
}

func newHeartbeatState(now time.Time) *heartbeatState {
	return &heartbeatState{last: now}
}

func (s *heartbeatState) mark(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last = now
}

func (s *heartbeatState) expired(now time.Time, timeout time.Duration) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return now.Sub(s.last) > timeout
}

func monitorParentHeartbeat(
	ctx context.Context,
	cancel context.CancelFunc,
	r io.Reader,
	heartbeatTimeout time.Duration,
	checkInterval time.Duration,
) {
	if r == nil || heartbeatTimeout <= 0 {
		return
	}
	if checkInterval <= 0 {
		checkInterval = heartbeatTimeout / 2
		if checkInterval <= 0 {
			checkInterval = 100 * time.Millisecond
		}
	}

	state := newHeartbeatState(time.Now())
	heartbeatCh := make(chan struct{}, 1)
	readerDone := make(chan struct{})

	go func() {
		defer close(readerDone)
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			if strings.TrimSpace(scanner.Text()) == heartbeatToken {
				select {
				case heartbeatCh <- struct{}{}:
				default:
				}
			}
		}
	}()

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeatCh:
			state.mark(time.Now())
		case <-readerDone:
			cancel()
			return
		case <-ticker.C:
			if state.expired(time.Now(), heartbeatTimeout) {
				cancel()
				return
			}
		}
	}
}
