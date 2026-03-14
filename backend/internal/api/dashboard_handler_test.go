package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/api"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

func TestDashboardHandlerSummary(t *testing.T) {
	t.Parallel()

	handler := api.NewDashboardHandler(dashboardUsageStub{
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

	req := httptest.NewRequest(http.MethodGet, "/dashboard/summary?hours=72&account_id=9&model=gpt-5.2", nil)
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

	handler := api.NewDashboardHandler(dashboardUsageStub{
		trends: []usage.TrendPoint{
			{Bucket: time.Date(2026, 3, 15, 9, 0, 0, 0, time.UTC), RequestCount: 2, TotalTokens: 330},
			{Bucket: time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC), RequestCount: 1, TotalTokens: 220},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/dashboard/trends?hours=24", nil)
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

	handler := api.NewDashboardHandler(dashboardUsageStub{
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

type dashboardUsageStub struct {
	summary usage.EventSummary
	trends  []usage.TrendPoint
	recent  []usage.Event
}

func (s dashboardUsageStub) SummarizeEvents(_ usage.EventFilter) (usage.EventSummary, error) {
	return s.summary, nil
}

func (s dashboardUsageStub) TrendEventsByHour(_ usage.EventFilter) ([]usage.TrendPoint, error) {
	return s.trends, nil
}

func (s dashboardUsageStub) ListRecentEvents(_ usage.EventFilter) ([]usage.Event, error) {
	return s.recent, nil
}
