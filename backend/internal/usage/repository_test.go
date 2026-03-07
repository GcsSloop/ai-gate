package usage_test

import (
	"path/filepath"
	"testing"
	"time"

	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
	"github.com/gcssloop/codex-router/backend/internal/usage"
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
		AccountID:            7,
		Balance:              19.25,
		QuotaRemaining:       120000,
		RPMRemaining:         100,
		TPMRemaining:         80000,
		HealthScore:          0.82,
		RecentErrorRate:      0.02,
		AvgLatencyMS:         320,
		LastTotalTokens:      2048,
		LastInputTokens:      1800,
		LastOutputTokens:     248,
		ModelContextWindow:   258400,
		PrimaryUsedPercent:   18,
		SecondaryUsedPercent: 44,
		CheckedAt:            checkedAt,
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
	if got.HealthScore != 0.82 {
		t.Fatalf("HealthScore = %v, want %v", got.HealthScore, 0.82)
	}
	if got.RecentErrorRate != 0.02 {
		t.Fatalf("RecentErrorRate = %v, want %v", got.RecentErrorRate, 0.02)
	}
	if got.AvgLatencyMS != 320 {
		t.Fatalf("AvgLatencyMS = %v, want %v", got.AvgLatencyMS, 320)
	}
	if got.LastTotalTokens != 2048 {
		t.Fatalf("LastTotalTokens = %v, want 2048", got.LastTotalTokens)
	}
	if got.ModelContextWindow != 258400 {
		t.Fatalf("ModelContextWindow = %v, want 258400", got.ModelContextWindow)
	}
	if got.PrimaryUsedPercent != 18 {
		t.Fatalf("PrimaryUsedPercent = %v, want 18", got.PrimaryUsedPercent)
	}
	if !got.CheckedAt.Equal(checkedAt) {
		t.Fatalf("CheckedAt = %v, want %v", got.CheckedAt, checkedAt)
	}
}

func TestSQLiteRepositoryListLatest(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := usage.NewSQLiteRepository(store.DB())

	if err := repo.Save(usage.Snapshot{
		AccountID:      1,
		Balance:        10,
		QuotaRemaining: 500,
		CheckedAt:      time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("Save(account 1, old) returned error: %v", err)
	}
	if err := repo.Save(usage.Snapshot{
		AccountID:      1,
		Balance:        8,
		QuotaRemaining: 300,
		CheckedAt:      time.Date(2026, 3, 7, 11, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("Save(account 1, new) returned error: %v", err)
	}
	if err := repo.Save(usage.Snapshot{
		AccountID:      2,
		Balance:        5,
		QuotaRemaining: 200,
		CheckedAt:      time.Date(2026, 3, 7, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("Save(account 2) returned error: %v", err)
	}

	got, err := repo.ListLatest()
	if err != nil {
		t.Fatalf("ListLatest returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListLatest returned %d rows, want 2", len(got))
	}
	if got[0].AccountID != 1 || got[0].Balance != 8 {
		t.Fatalf("first latest snapshot = %+v, want latest account 1 snapshot", got[0])
	}
}
