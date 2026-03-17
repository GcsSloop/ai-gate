package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/api"
	"github.com/gcssloop/codex-router/backend/internal/settings"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

func TestDashboardHandlerSummary(t *testing.T) {
	t.Parallel()

	handler := api.NewDashboardHandler(&dashboardUsageStub{
		summary: usage.EventSummary{
			RequestCount:  12,
			SuccessCount:  10,
			FailureCount:  2,
			InputTokens:   12000,
			OutputTokens:  4000,
			TotalTokens:   16000,
			EstimatedCost: 1.23,
			BalanceDelta:  -4.5,
			QuotaDelta:    -8000,
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/dashboard/summary?range=7d&account_id=9&model=gpt-5.2", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /dashboard/summary status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got usage.EventSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if got.RequestCount != 12 || got.EstimatedCost != 1.23 {
		t.Fatalf("summary = %+v, want request_count=12 estimated_cost=1.23", got)
	}
}

func TestDashboardHandlerTrends(t *testing.T) {
	t.Parallel()

	handler := api.NewDashboardHandler(&dashboardUsageStub{
		trends: []usage.TrendPoint{
			{Bucket: time.Date(2026, 3, 15, 9, 0, 0, 0, time.UTC), RequestCount: 2, TotalTokens: 330},
			{Bucket: time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC), RequestCount: 1, TotalTokens: 220},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/dashboard/trends?range=24h", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /dashboard/trends status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []usage.TrendPoint
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].RequestCount != 2 || got[1].TotalTokens != 220 {
		t.Fatalf("trends = %+v", got)
	}
}

func TestDashboardHandlerRecentEvents(t *testing.T) {
	t.Parallel()

	handler := api.NewDashboardHandler(&dashboardUsageStub{
		recent: []usage.Event{
			{ID: 2, AccountID: 9, Model: "gpt-5.2", Status: "completed", TotalTokens: 1500, EstimatedCost: 0.42, CreatedAt: time.Date(2026, 3, 15, 10, 5, 0, 0, time.UTC)},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/dashboard/recent-events?limit=20", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /dashboard/recent-events status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []usage.Event
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].ID != 2 || got[0].EstimatedCost != 0.42 {
		t.Fatalf("recent = %+v", got[0])
	}
}

func TestDashboardHandlerModelDistribution(t *testing.T) {
	t.Parallel()

	handler := api.NewDashboardHandler(&dashboardUsageStub{
		modelDistribution: []usage.ModelDistributionPoint{
			{Model: "gpt-5.2", RequestCount: 8},
			{Model: "gpt-5.4", RequestCount: 4},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/dashboard/model-distribution?range=24h&account_id=9&model=gpt-5.2", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /dashboard/model-distribution status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []usage.ModelDistributionPoint
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Model != "gpt-5.2" || got[0].RequestCount != 8 {
		t.Fatalf("distribution[0] = %+v, want gpt-5.2 with count 8", got[0])
	}
}

func TestDashboardHandlerBuildsCalendarAlignedFilter(t *testing.T) {
	t.Setenv("TZ", "Asia/Shanghai")
	now := time.Date(2026, 3, 17, 13, 45, 0, 0, time.FixedZone("CST", 8*3600))

	stub := &dashboardUsageStub{
		summary: usage.EventSummary{RequestCount: 1},
	}
	handler := api.NewDashboardHandler(stub)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/summary?range=24h", nil)
	rec := httptest.NewRecorder()

	restore := api.SetDashboardNowForTest(func() time.Time { return now })
	defer restore()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /dashboard/summary status = %d, want %d", rec.Code, http.StatusOK)
	}
	if stub.lastFilter.From == nil || stub.lastFilter.To == nil {
		t.Fatalf("expected filter bounds to be set")
	}
	wantFrom := time.Date(2026, 3, 16, 16, 0, 0, 0, time.UTC)
	wantTo := time.Date(2026, 3, 17, 16, 0, 0, 0, time.UTC)
	if !stub.lastFilter.From.Equal(wantFrom) || !stub.lastFilter.To.Equal(wantTo) {
		t.Fatalf("filter bounds = %v..%v, want %v..%v", stub.lastFilter.From, stub.lastFilter.To, wantFrom, wantTo)
	}
	if stub.lastFilter.BucketSize != time.Hour {
		t.Fatalf("BucketSize = %v, want %v", stub.lastFilter.BucketSize, time.Hour)
	}
}

func TestDashboardHandlerUsesPricingSettingsForCostCalculation(t *testing.T) {
	stub := &dashboardUsageStub{
		summary: usage.EventSummary{
			RequestCount:  1,
			InputTokens:   1_000_000,
			OutputTokens:  500_000,
			TotalTokens:   1_500_000,
			EstimatedCost: 0,
		},
	}
	settingsRepo := dashboardSettingsStub{
		value: settings.AppSettings{
			AccountPricing: map[string]settings.PricingRule{
				settings.AccountPricingKey(7): {InputPerMillion: 10, OutputPerMillion: 20},
			},
		},
	}
	handler := api.NewDashboardHandler(stub, api.WithDashboardSettings(settingsRepo))

	req := httptest.NewRequest(http.MethodGet, "/dashboard/summary?range=24h", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /dashboard/summary status = %d, want %d", rec.Code, http.StatusOK)
	}
	if stub.lastFilter.CostCalculator == nil {
		t.Fatalf("expected cost calculator on filter")
	}
	got := stub.lastFilter.CostCalculator(7, "codex", "gpt-5.4", 1_000_000, 500_000)
	if got != 20 {
		t.Fatalf("calculated cost = %v, want 20", got)
	}
}

func TestDashboardHandlerDoesNotCompactEventsOnReadRequests(t *testing.T) {
	t.Parallel()

	stub := &dashboardUsageStub{
		summary: usage.EventSummary{RequestCount: 1},
	}
	handler := api.NewDashboardHandler(stub)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/summary?range=24h", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /dashboard/summary status = %d, want %d", rec.Code, http.StatusOK)
	}
	if stub.compactCalls != 0 {
		t.Fatalf("CompactEvents calls = %d, want 0", stub.compactCalls)
	}
}

type dashboardUsageStub struct {
	summary           usage.EventSummary
	trends            []usage.TrendPoint
	recent            []usage.Event
	modelDistribution []usage.ModelDistributionPoint
	lastFilter        usage.EventFilter
	compactCalls      int
}

func (s *dashboardUsageStub) SummarizeEvents(filter usage.EventFilter) (usage.EventSummary, error) {
	s.lastFilter = filter
	return s.summary, nil
}

func (s *dashboardUsageStub) TrendEventsByHour(filter usage.EventFilter) ([]usage.TrendPoint, error) {
	s.lastFilter = filter
	return s.trends, nil
}

func (s *dashboardUsageStub) ListRecentEvents(filter usage.EventFilter) ([]usage.Event, error) {
	s.lastFilter = filter
	return s.recent, nil
}

func (s *dashboardUsageStub) ModelDistribution(filter usage.EventFilter) ([]usage.ModelDistributionPoint, error) {
	s.lastFilter = filter
	return s.modelDistribution, nil
}

func (s *dashboardUsageStub) CompactEvents(time.Time) error {
	s.compactCalls++
	return nil
}

type dashboardSettingsStub struct {
	value settings.AppSettings
}

func (s dashboardSettingsStub) GetAppSettings() (settings.AppSettings, error) {
	return s.value, nil
}

func (s dashboardSettingsStub) ListFailoverQueue() ([]int64, error) {
	return nil, nil
}
