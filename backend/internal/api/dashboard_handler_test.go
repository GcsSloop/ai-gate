package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/api"
	"github.com/gcssloop/codex-router/backend/internal/conversations"
)

func TestDashboardHandler(t *testing.T) {
	t.Parallel()

	handler := api.NewDashboardHandler(dashboardSummaryStub{
		totalConversations:  12,
		activeConversations: 4,
		totalRuns:           28,
		failoverRuns:        3,
	})

	req := httptest.NewRequest(http.MethodGet, "/dashboard/summary", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /dashboard/summary status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got map[string]int
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if got["active_conversations"] != 4 {
		t.Fatalf("active_conversations = %d, want 4", got["active_conversations"])
	}
	if got["failover_runs"] != 3 {
		t.Fatalf("failover_runs = %d, want 3", got["failover_runs"])
	}
}

func TestDashboardHandlerAccountStats(t *testing.T) {
	t.Parallel()

	handler := api.NewDashboardHandler(dashboardSummaryStub{
		accountCallStats: []conversations.AccountCallStats{
			{
				AccountID:  1,
				TotalCalls: 5,
				ModelCalls: map[string]int{
					"gpt-5.4": 3,
					"gpt-4.1": 2,
				},
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/dashboard/account-stats", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /dashboard/account-stats status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0]["total_calls"].(float64) != 5 {
		t.Fatalf("total_calls = %v, want 5", got[0]["total_calls"])
	}
}

type dashboardSummaryStub struct {
	totalConversations  int
	activeConversations int
	totalRuns           int
	failoverRuns        int
	accountCallStats    []conversations.AccountCallStats
}

func (s dashboardSummaryStub) CountConversations() (int, error) {
	return s.totalConversations, nil
}

func (s dashboardSummaryStub) CountActiveConversations() (int, error) {
	return s.activeConversations, nil
}

func (s dashboardSummaryStub) CountRuns() (int, error) {
	return s.totalRuns, nil
}

func (s dashboardSummaryStub) CountFailoverRuns() (int, error) {
	return s.failoverRuns, nil
}

func (s dashboardSummaryStub) ListAccountCallStats() ([]conversations.AccountCallStats, error) {
	return s.accountCallStats, nil
}
