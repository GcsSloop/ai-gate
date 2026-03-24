package settings_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/routing"
	"github.com/gcssloop/codex-router/backend/internal/settings"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

func TestOrderCandidatesUsesExplicitQueueWhenEnabled(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := settings.NewSQLiteRepository(store.DB())
	current := settings.DefaultAppSettings()
	current.AutoFailoverEnabled = true
	if err := repo.SaveAppSettings(current); err != nil {
		t.Fatalf("SaveAppSettings returned error: %v", err)
	}
	if err := repo.SaveFailoverQueue([]int64{3, 2, 1, 4}); err != nil {
		t.Fatalf("SaveFailoverQueue returned error: %v", err)
	}

	ordered, err := settings.OrderCandidates(repo, []routing.Candidate{
		{Account: accounts.Account{ID: 1, Status: accounts.StatusActive, Priority: 100}, Snapshot: usage.Snapshot{HealthScore: 0.9}},
		{Account: accounts.Account{ID: 2, Status: accounts.StatusActive, Priority: 99, CooldownUntil: timePtr()}, Snapshot: usage.Snapshot{HealthScore: 0.8}},
		{Account: accounts.Account{ID: 3, Status: accounts.StatusDegraded, Priority: 98}, Snapshot: usage.Snapshot{HealthScore: 0.7}},
		{Account: accounts.Account{ID: 4, Status: accounts.StatusDisabled, Priority: 97}, Snapshot: usage.Snapshot{HealthScore: 0.6}},
	})
	if err != nil {
		t.Fatalf("OrderCandidates returned error: %v", err)
	}

	if len(ordered) != 2 {
		t.Fatalf("len(ordered) = %d, want 2", len(ordered))
	}
	if ordered[0].Account.ID != 3 || ordered[1].Account.ID != 1 {
		t.Fatalf("ordered ids = [%d %d], want [3 1]", ordered[0].Account.ID, ordered[1].Account.ID)
	}
}

func timePtr() *time.Time {
	value := time.Now().UTC().Add(5 * time.Minute)
	return &value
}

func TestOrderCandidatesFallsBackToScoredOrderWhenDisabled(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := settings.NewSQLiteRepository(store.DB())
	ordered, err := settings.OrderCandidates(repo, []routing.Candidate{
		{Account: accounts.Account{ID: 1, Status: accounts.StatusActive, Priority: 1}, Snapshot: usage.Snapshot{HealthScore: 0.1}},
		{Account: accounts.Account{ID: 2, Status: accounts.StatusActive, Priority: 10}, Snapshot: usage.Snapshot{HealthScore: 0.9}},
	})
	if err != nil {
		t.Fatalf("OrderCandidates returned error: %v", err)
	}

	if len(ordered) != 2 {
		t.Fatalf("len(ordered) = %d, want 2", len(ordered))
	}
	if ordered[0].Account.ID != 2 || ordered[1].Account.ID != 1 {
		t.Fatalf("ordered ids = [%d %d], want [2 1]", ordered[0].Account.ID, ordered[1].Account.ID)
	}
}
