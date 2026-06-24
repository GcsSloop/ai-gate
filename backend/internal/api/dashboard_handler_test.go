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

	stub := &dashboardUsageStub{
		recentPage: usage.EventPage{
			Items: []usage.Event{
				{ID: 2, AccountID: 9, Model: "gpt-5.2", Status: "completed", TotalTokens: 1500, EstimatedCost: 0.42, CreatedAt: time.Date(2026, 3, 15, 10, 5, 0, 0, time.UTC)},
			},
			Total:    12,
			Page:     2,
			PageSize: 1,
		},
	}
	handler := api.NewDashboardHandler(stub)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/recent-events?page=2&page_size=1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /dashboard/recent-events status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got usage.EventPage
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if got.Total != 12 || got.Page != 2 || got.PageSize != 1 {
		t.Fatalf("page metadata = %+v, want total=12 page=2 page_size=1", got)
	}
	if len(got.Items) != 1 {
		t.Fatalf("len(got.Items) = %d, want 1", len(got.Items))
	}
	if got.Items[0].ID != 2 || got.Items[0].EstimatedCost != 0.42 {
		t.Fatalf("recent = %+v", got.Items[0])
	}
	if stub.lastFilter.Limit != 1 || stub.lastFilter.Offset != 1 {
		t.Fatalf("filter limit/offset = %d/%d, want 1/1", stub.lastFilter.Limit, stub.lastFilter.Offset)
	}
}

func TestDashboardHandlerRequestQuality(t *testing.T) {
	t.Parallel()

	handler := api.NewDashboardHandler(&dashboardUsageStub{
		quality: usage.RequestQuality{
			RequestCount: 100,
			SuccessCount: 95,
			FailureCount: 5,
			SuccessRate:  0.95,
			AvgLatencyMS: 250,
			P95LatencyMS: 800,
			P99LatencyMS: 1200,
			MinLatencyMS: 20,
			MaxLatencyMS: 1600,
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/dashboard/request-quality?range=24h&account_id=9", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /dashboard/request-quality status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got usage.RequestQuality
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if got.SuccessRate != 0.95 || got.P95LatencyMS != 800 || got.P99LatencyMS != 1200 {
		t.Fatalf("quality = %+v, want success_rate=0.95 p95=800 p99=1200", got)
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
	previousLocal := time.Local
	time.Local = now.Location()
	defer func() {
		time.Local = previousLocal
	}()

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
	if stub.lastFilter.BucketLocation != time.Local {
		t.Fatalf("BucketLocation = %v, want time.Local", stub.lastFilter.BucketLocation)
	}
}

func TestDashboardHandlerBuildsYearlyCalendarAlignedFilter(t *testing.T) {
	t.Setenv("TZ", "Asia/Shanghai")
	now := time.Date(2026, 6, 16, 9, 30, 0, 0, time.FixedZone("CST", 8*3600))
	previousLocal := time.Local
	time.Local = now.Location()
	defer func() {
		time.Local = previousLocal
	}()

	stub := &dashboardUsageStub{
		summary: usage.EventSummary{RequestCount: 1},
	}
	handler := api.NewDashboardHandler(stub)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/summary?range=1y", nil)
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
	wantFrom := time.Date(2025, 6, 16, 16, 0, 0, 0, time.UTC)
	wantTo := time.Date(2026, 6, 16, 16, 0, 0, 0, time.UTC)
	if !stub.lastFilter.From.Equal(wantFrom) || !stub.lastFilter.To.Equal(wantTo) {
		t.Fatalf("filter bounds = %v..%v, want %v..%v", stub.lastFilter.From, stub.lastFilter.To, wantFrom, wantTo)
	}
	if stub.lastFilter.BucketSize != 24*time.Hour {
		t.Fatalf("BucketSize = %v, want %v", stub.lastFilter.BucketSize, 24*time.Hour)
	}
	if stub.lastFilter.BucketLocation != time.Local {
		t.Fatalf("BucketLocation = %v, want time.Local", stub.lastFilter.BucketLocation)
	}
}

func TestDashboardHandlerBuildsServerUserFilter(t *testing.T) {
	t.Parallel()

	stub := &dashboardUsageStub{
		summary: usage.EventSummary{RequestCount: 1},
	}
	handler := api.NewDashboardHandler(stub)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/summary?range=24h&server_user_id=42", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /dashboard/summary status = %d, want %d", rec.Code, http.StatusOK)
	}
	if stub.lastFilter.ServerUserID == nil {
		t.Fatalf("ServerUserID is nil, want 42")
	}
	if *stub.lastFilter.ServerUserID != 42 {
		t.Fatalf("ServerUserID = %d, want 42", *stub.lastFilter.ServerUserID)
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
	quality           usage.RequestQuality
	trends            []usage.TrendPoint
	recentPage        usage.EventPage
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

func (s *dashboardUsageStub) ListRecentEventsPage(filter usage.EventFilter) (usage.EventPage, error) {
	s.lastFilter = filter
	return s.recentPage, nil
}

func (s *dashboardUsageStub) AnalyzeRequestQuality(filter usage.EventFilter) (usage.RequestQuality, error) {
	s.lastFilter = filter
	return s.quality, nil
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
