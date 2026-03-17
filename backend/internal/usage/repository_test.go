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
		Source:               "remote",
		Confidence:           "high",
		ProviderSnapshotJSON: `{"vendor":"official"}`,
		Stale:                true,
		LastError:            "temporary upstream timeout",
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
	if got.Source != "remote" {
		t.Fatalf("Source = %q, want %q", got.Source, "remote")
	}
	if got.Confidence != "high" {
		t.Fatalf("Confidence = %q, want %q", got.Confidence, "high")
	}
	if got.ProviderSnapshotJSON != `{"vendor":"official"}` {
		t.Fatalf("ProviderSnapshotJSON = %q, want %q", got.ProviderSnapshotJSON, `{"vendor":"official"}`)
	}
	if !got.Stale {
		t.Fatal("Stale = false, want true")
	}
	if got.LastError != "temporary upstream timeout" {
		t.Fatalf("LastError = %q, want %q", got.LastError, "temporary upstream timeout")
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
		Source:         "remote",
		Confidence:     "medium",
		Balance:        10,
		QuotaRemaining: 500,
		CheckedAt:      time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("Save(account 1, old) returned error: %v", err)
	}
	if err := repo.Save(usage.Snapshot{
		AccountID:      1,
		Source:         "mixed",
		Confidence:     "low",
		LastError:      "quota endpoint unavailable",
		Stale:          true,
		Balance:        8,
		QuotaRemaining: 300,
		CheckedAt:      time.Date(2026, 3, 7, 11, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("Save(account 1, new) returned error: %v", err)
	}
	if err := repo.Save(usage.Snapshot{
		AccountID:      2,
		Source:         "inferred",
		Confidence:     "low",
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
	if got[0].Source != "mixed" || got[0].Confidence != "low" {
		t.Fatalf("first latest metadata = source=%q confidence=%q, want mixed/low", got[0].Source, got[0].Confidence)
	}
	if !got[0].Stale || got[0].LastError != "quota endpoint unavailable" {
		t.Fatalf("first latest stale metadata = stale=%t last_error=%q", got[0].Stale, got[0].LastError)
	}
	if got[1].Source != "inferred" || got[1].Confidence != "low" {
		t.Fatalf("second latest metadata = source=%q confidence=%q, want inferred/low", got[1].Source, got[1].Confidence)
	}
	if got[1].Stale {
		t.Fatal("second latest Stale = true, want false")
	}
	if got[1].LastError != "" {
		t.Fatalf("second latest LastError = %q, want empty", got[1].LastError)
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

func TestSQLiteRepositoryModelDistribution(t *testing.T) {
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
	accountID := int64(9)

	for _, event := range []usage.Event{
		{
			AccountID:     9,
			ProviderType:  "openai",
			RequestKind:   "responses",
			Model:         "gpt-5.2",
			Status:        "completed",
			TotalTokens:   1500,
			EstimatedCost: 0.42,
			LatencyMS:     321,
			CreatedAt:     time.Date(2026, 3, 15, 10, 5, 0, 0, time.UTC),
		},
		{
			AccountID:     9,
			ProviderType:  "openai",
			RequestKind:   "responses",
			Model:         "gpt-5.4",
			Status:        "completed",
			TotalTokens:   800,
			EstimatedCost: 0.24,
			LatencyMS:     280,
			CreatedAt:     time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			AccountID:     9,
			ProviderType:  "openai",
			RequestKind:   "responses",
			Model:         "gpt-5.2",
			Status:        "rate_limited",
			TotalTokens:   200,
			EstimatedCost: 0.01,
			LatencyMS:     99,
			CreatedAt:     time.Date(2026, 3, 15, 11, 0, 0, 0, time.UTC),
		},
		{
			AccountID:     7,
			ProviderType:  "openai",
			RequestKind:   "responses",
			Model:         "gpt-4.1",
			Status:        "completed",
			TotalTokens:   999,
			EstimatedCost: 0.5,
			LatencyMS:     100,
			CreatedAt:     time.Date(2026, 3, 15, 10, 45, 0, 0, time.UTC),
		},
	} {
		if err := repo.SaveEvent(event); err != nil {
			t.Fatalf("SaveEvent(%s) returned error: %v", event.Model, err)
		}
	}

	distribution, err := repo.ModelDistribution(usage.EventFilter{
		From:      &from,
		To:        &to,
		AccountID: &accountID,
	})
	if err != nil {
		t.Fatalf("ModelDistribution returned error: %v", err)
	}
	if len(distribution) != 2 {
		t.Fatalf("len(distribution) = %d, want 2", len(distribution))
	}
	if distribution[0].Model != "gpt-5.2" || distribution[0].RequestCount != 2 {
		t.Fatalf("distribution[0] = %+v, want gpt-5.2 with 2 requests", distribution[0])
	}
	if distribution[1].Model != "gpt-5.4" || distribution[1].RequestCount != 1 {
		t.Fatalf("distribution[1] = %+v, want gpt-5.4 with 1 request", distribution[1])
	}
}

func TestSQLiteRepositorySummarizeEventsUsesDynamicCostCalculator(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := usage.NewSQLiteRepository(store.DB())
	if err := repo.SaveEvent(usage.Event{
		AccountID:    7,
		ProviderType: "codex",
		Model:        "gpt-5.4",
		Status:       "completed",
		InputTokens:  1_000_000,
		OutputTokens: 500_000,
		TotalTokens:  1_500_000,
		CreatedAt:    time.Date(2026, 3, 15, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SaveEvent returned error: %v", err)
	}

	summary, err := repo.SummarizeEvents(usage.EventFilter{
		From: ptrTime(time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)),
		To:   ptrTime(time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)),
		CostCalculator: func(accountID int64, providerType string, model string, inputTokens int64, outputTokens int64) float64 {
			if accountID != 7 || providerType != "codex" || model != "gpt-5.4" {
				t.Fatalf("unexpected calculator input: %d %s %s", accountID, providerType, model)
			}
			return 20
		},
	})
	if err != nil {
		t.Fatalf("SummarizeEvents returned error: %v", err)
	}
	if summary.EstimatedCost != 20 {
		t.Fatalf("EstimatedCost = %v, want 20", summary.EstimatedCost)
	}
}

func TestSQLiteRepositoryCompactsEventsIntoRollups(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := usage.NewSQLiteRepository(store.DB())
	now := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)
	for _, event := range []usage.Event{
		{
			AccountID:    1,
			ProviderType: "codex",
			RequestKind:  "responses",
			Model:        "gpt-5.4",
			Status:       "completed",
			InputTokens:  100,
			OutputTokens: 20,
			TotalTokens:  120,
			CreatedAt:    now.AddDate(0, 0, -8).Add(10 * time.Minute),
		},
		{
			AccountID:    1,
			ProviderType: "codex",
			RequestKind:  "responses",
			Model:        "gpt-5.4",
			Status:       "rate_limited",
			InputTokens:  40,
			OutputTokens: 0,
			TotalTokens:  40,
			CreatedAt:    now.AddDate(0, 0, -8).Add(20 * time.Minute),
		},
		{
			AccountID:    1,
			ProviderType: "codex",
			RequestKind:  "responses",
			Model:        "gpt-5.4",
			Status:       "completed",
			InputTokens:  200,
			OutputTokens: 30,
			TotalTokens:  230,
			CreatedAt:    now.AddDate(0, 0, -40).Add(2 * time.Hour),
		},
	} {
		if err := repo.SaveEvent(event); err != nil {
			t.Fatalf("SaveEvent returned error: %v", err)
		}
	}

	if err := repo.CompactEvents(now); err != nil {
		t.Fatalf("CompactEvents returned error: %v", err)
	}

	summary, err := repo.SummarizeEvents(usage.EventFilter{
		From: ptrTime(now.AddDate(0, 0, -50)),
		To:   ptrTime(now.Add(time.Hour)),
	})
	if err != nil {
		t.Fatalf("SummarizeEvents returned error: %v", err)
	}
	if summary.RequestCount != 3 || summary.TotalTokens != 390 {
		t.Fatalf("summary = %+v, want request_count=3 total_tokens=390", summary)
	}

	trends, err := repo.TrendEventsByHour(usage.EventFilter{
		From:          ptrTime(time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC)),
		To:            ptrTime(time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC)),
		BucketSize:    24 * time.Hour,
		IncludeZeroes: false,
	})
	if err != nil {
		t.Fatalf("TrendEventsByHour returned error: %v", err)
	}
	if len(trends) != 2 {
		t.Fatalf("len(trends) = %d, want 2", len(trends))
	}
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
