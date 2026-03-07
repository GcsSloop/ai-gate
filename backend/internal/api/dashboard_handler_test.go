package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/api"
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

type dashboardSummaryStub struct {
	totalConversations  int
	activeConversations int
	totalRuns           int
	failoverRuns        int
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
