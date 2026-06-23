package api

import (
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/usage"
)

type slowUsageStore struct {
	saveEventDelay time.Duration
	events         []usage.Event
}

func (s *slowUsageStore) GetLatest(accountID int64) (usage.Snapshot, error) {
	return usage.Snapshot{AccountID: accountID, HealthScore: 1}, nil
}

func (s *slowUsageStore) Save(snapshot usage.Snapshot) error {
	return nil
}

func (s *slowUsageStore) SaveEvent(event usage.Event) error {
	time.Sleep(s.saveEventDelay)
	s.events = append(s.events, event)
	return nil
}

func TestAsyncUsageStoreSaveEventDoesNotBlockCaller(t *testing.T) {
	base := &slowUsageStore{saveEventDelay: 100 * time.Millisecond}
	store := NewAsyncUsageStore(base, AsyncUsageStoreOptions{QueueSize: 4})
	defer store.Close()

	startedAt := time.Now()
	if err := store.SaveEvent(usage.Event{AccountID: 1, RequestKind: "responses", Status: "completed"}); err != nil {
		t.Fatalf("SaveEvent returned error: %v", err)
	}
	if elapsed := time.Since(startedAt); elapsed >= 50*time.Millisecond {
		t.Fatalf("SaveEvent blocked for %s, want non-blocking enqueue", elapsed)
	}

	store.Drain()
	if len(base.events) != 1 {
		t.Fatalf("persisted events = %d, want 1", len(base.events))
	}
}
