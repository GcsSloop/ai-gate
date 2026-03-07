package usage_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/usage"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
)

func TestSQLiteRepositorySaveAndGetLatest(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := usage.NewSQLiteRepository(store.DB())
	checkedAt := time.Now().UTC().Truncate(time.Second)

	err = repo.Save(usage.Snapshot{
		AccountID:       7,
		Balance:         19.25,
		QuotaRemaining:  120000,
		RPMRemaining:    100,
		TPMRemaining:    80000,
		RecentErrorRate: 0.02,
		AvgLatencyMS:    320,
		CheckedAt:       checkedAt,
	})
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	got, err := repo.GetLatest(7)
	if err != nil {
		t.Fatalf("GetLatest returned error: %v", err)
	}

	if got.Balance != 19.25 {
		t.Fatalf("Balance = %v, want %v", got.Balance, 19.25)
	}
	if got.QuotaRemaining != 120000 {
		t.Fatalf("QuotaRemaining = %v, want %v", got.QuotaRemaining, 120000)
	}
	if got.RPMRemaining != 100 {
		t.Fatalf("RPMRemaining = %v, want %v", got.RPMRemaining, 100)
	}
	if got.TPMRemaining != 80000 {
		t.Fatalf("TPMRemaining = %v, want %v", got.TPMRemaining, 80000)
	}
	if got.RecentErrorRate != 0.02 {
		t.Fatalf("RecentErrorRate = %v, want %v", got.RecentErrorRate, 0.02)
	}
	if got.AvgLatencyMS != 320 {
		t.Fatalf("AvgLatencyMS = %v, want %v", got.AvgLatencyMS, 320)
	}
	if !got.CheckedAt.Equal(checkedAt) {
		t.Fatalf("CheckedAt = %v, want %v", got.CheckedAt, checkedAt)
	}
}
