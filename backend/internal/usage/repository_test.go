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

func TestSQLiteRepositorySaveEventListRecentAndSummarize(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := usage.NewSQLiteRepository(store.DB())
	from := time.Date(2026, 3, 15, 8, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

	balanceBefore := 100.0
	balanceAfter := 98.5
	quotaBefore := 3000000.0
	quotaAfter := 2997000.0

	if err := repo.SaveEvent(usage.Event{
		AccountID:     9,
		ProviderType:  "openai",
		RequestKind:   "responses",
		Model:         "gpt-5.2",
		Status:        "completed",
		InputTokens:   1200,
		OutputTokens:  300,
		TotalTokens:   1500,
		EstimatedCost: 0.42,
		BalanceBefore: &balanceBefore,
		BalanceAfter:  &balanceAfter,
		QuotaBefore:   &quotaBefore,
		QuotaAfter:    &quotaAfter,
		LatencyMS:     321,
		CreatedAt:     time.Date(2026, 3, 15, 10, 5, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SaveEvent(first) returned error: %v", err)
	}

	if err := repo.SaveEvent(usage.Event{
		AccountID:     9,
		ProviderType:  "openai",
		RequestKind:   "responses",
		Model:         "gpt-5.2",
		Status:        "rate_limited",
		InputTokens:   200,
		OutputTokens:  0,
		TotalTokens:   200,
		EstimatedCost: 0.01,
		LatencyMS:     99,
		CreatedAt:     time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SaveEvent(second) returned error: %v", err)
	}

	accountID := int64(9)
	events, err := repo.ListRecentEvents(usage.EventFilter{
		From:      &from,
		To:        &to,
		AccountID: &accountID,
		Model:     "gpt-5.2",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("ListRecentEvents returned error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}
	if events[0].Status != "rate_limited" {
		t.Fatalf("events[0].Status = %q, want newest event first", events[0].Status)
	}

	summary, err := repo.SummarizeEvents(usage.EventFilter{
		From:      &from,
		To:        &to,
		AccountID: &accountID,
		Model:     "gpt-5.2",
	})
	if err != nil {
		t.Fatalf("SummarizeEvents returned error: %v", err)
	}
	if summary.RequestCount != 2 {
		t.Fatalf("RequestCount = %d, want 2", summary.RequestCount)
	}
	if summary.SuccessCount != 1 || summary.FailureCount != 1 {
		t.Fatalf("success/failure = %d/%d, want 1/1", summary.SuccessCount, summary.FailureCount)
	}
	if summary.InputTokens != 1400 || summary.OutputTokens != 300 || summary.TotalTokens != 1700 {
		t.Fatalf("token summary = %+v, want input=1400 output=300 total=1700", summary)
	}
	if summary.EstimatedCost != 0.43 {
		t.Fatalf("EstimatedCost = %v, want 0.43", summary.EstimatedCost)
	}
	if summary.BalanceDelta != -1.5 {
		t.Fatalf("BalanceDelta = %v, want -1.5", summary.BalanceDelta)
	}
	if summary.QuotaDelta != -3000 {
		t.Fatalf("QuotaDelta = %v, want -3000", summary.QuotaDelta)
	}
}

func TestSQLiteRepositoryTrendEventsByHour(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := usage.NewSQLiteRepository(store.DB())
	for _, event := range []usage.Event{
		{
			AccountID:     1,
			ProviderType:  "openai",
			RequestKind:   "responses",
			Model:         "gpt-5.2",
			Status:        "completed",
			InputTokens:   100,
			OutputTokens:  10,
			TotalTokens:   110,
			EstimatedCost: 0.1,
			CreatedAt:     time.Date(2026, 3, 15, 9, 15, 0, 0, time.UTC),
		},
		{
			AccountID:     1,
			ProviderType:  "openai",
			RequestKind:   "responses",
			Model:         "gpt-5.2",
			Status:        "completed",
			InputTokens:   200,
			OutputTokens:  20,
			TotalTokens:   220,
			EstimatedCost: 0.2,
			CreatedAt:     time.Date(2026, 3, 15, 9, 45, 0, 0, time.UTC),
		},
		{
			AccountID:     1,
			ProviderType:  "openai",
			RequestKind:   "responses",
			Model:         "gpt-5.2",
			Status:        "completed",
			InputTokens:   300,
			OutputTokens:  30,
			TotalTokens:   330,
			EstimatedCost: 0.3,
			CreatedAt:     time.Date(2026, 3, 15, 10, 5, 0, 0, time.UTC),
		},
	} {
		if err := repo.SaveEvent(event); err != nil {
			t.Fatalf("SaveEvent returned error: %v", err)
		}
	}

	points, err := repo.TrendEventsByHour(usage.EventFilter{
		From: ptrTime(time.Date(2026, 3, 15, 9, 0, 0, 0, time.UTC)),
		To:   ptrTime(time.Date(2026, 3, 15, 11, 0, 0, 0, time.UTC)),
	})
	if err != nil {
		t.Fatalf("TrendEventsByHour returned error: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("len(points) = %d, want 2", len(points))
	}
	if !points[0].Bucket.Equal(time.Date(2026, 3, 15, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("points[0].Bucket = %v, want 2026-03-15T09:00:00Z", points[0].Bucket)
	}
	if points[0].RequestCount != 2 || points[0].TotalTokens != 330 {
		t.Fatalf("points[0] = %+v, want request_count=2 total_tokens=330", points[0])
	}
	if points[1].RequestCount != 1 || points[1].EstimatedCost != 0.3 {
		t.Fatalf("points[1] = %+v, want request_count=1 estimated_cost=0.3", points[1])
	}
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
