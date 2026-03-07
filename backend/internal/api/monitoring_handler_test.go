package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/api"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

func TestMonitoringHandler(t *testing.T) {
	t.Parallel()

	handler := api.NewMonitoringHandler(
		accountLister{items: []accounts.Account{
			{ID: 1, Status: accounts.StatusActive},
			{ID: 2, Status: accounts.StatusCooldown},
			{ID: 3, Status: accounts.StatusCooldown},
		}},
		usageSnapshotLister{items: []usage.Snapshot{
			{AccountID: 1, Balance: 10, QuotaRemaining: 1000},
			{AccountID: 2, Balance: 2, QuotaRemaining: 300},
		}},
	)

	req := httptest.NewRequest(http.MethodGet, "/monitoring/overview", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /monitoring/overview status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	statusCounts := got["status_counts"].(map[string]any)
	if statusCounts["cooldown"].(float64) != 2 {
		t.Fatalf("cooldown count = %v, want 2", statusCounts["cooldown"])
	}
}

type usageSnapshotLister struct {
	items []usage.Snapshot
}

func (l usageSnapshotLister) ListLatest() ([]usage.Snapshot, error) {
	return l.items, nil
}
