package api

import (
	"path/filepath"
	"testing"
	"time"

	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

func TestAsyncUsageStoreObservesRefreshedQuota(t *testing.T) {
	db, err := sqlitestore.Open(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	base := usage.NewSQLiteRepository(db.DB())
	store := NewAsyncUsageStore(base, AsyncUsageStoreOptions{})
	defer store.Close()
	now := time.Now().UTC()
	for i, used := range []float64{95, 97, 0} {
		if err := base.Save(usage.Snapshot{AccountID: 1, PrimaryUsedPercent: used, CheckedAt: now.Add(time.Duration(i) * time.Second)}); err != nil {
			t.Fatal(err)
		}
		got, err := store.GetLatest(1)
		if err != nil || got.PrimaryUsedPercent != used {
			t.Fatalf("fresh usage = %+v, %v; want %v%% used", got, err, used)
		}
	}
}

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
