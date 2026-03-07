package api

import (
	"net/http"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

type UsageSnapshotLister interface {
	ListLatest() ([]usage.Snapshot, error)
}

type MonitoringHandler struct {
	accounts accountListerAPI
	usage    UsageSnapshotLister
}

type accountListerAPI interface {
	List() ([]accounts.Account, error)
}

func NewMonitoringHandler(accounts accountListerAPI, usage UsageSnapshotLister) *MonitoringHandler {
	return &MonitoringHandler{accounts: accounts, usage: usage}
}

func (h *MonitoringHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.URL.Path != "/monitoring/overview" {
		http.NotFound(w, r)
		return
	}

	accountList, err := h.accounts.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	snapshots, err := h.usage.ListLatest()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	statusCounts := make(map[string]int)
	for _, account := range accountList {
		statusCounts[string(account.Status)]++
	}

	totalBalance := 0.0
	totalQuota := 0.0
	for _, snapshot := range snapshots {
		totalBalance += snapshot.Balance
		totalQuota += snapshot.QuotaRemaining
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status_counts": statusCounts,
		"totals": map[string]float64{
			"balance": totalBalance,
			"quota":   totalQuota,
		},
	})
}
