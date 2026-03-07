package api

import (
	"net/http"

	"github.com/gcssloop/codex-router/backend/internal/conversations"
)

type DashboardConversationSummary interface {
	CountConversations() (int, error)
	CountActiveConversations() (int, error)
	CountRuns() (int, error)
	CountFailoverRuns() (int, error)
	ListAccountCallStats() ([]conversations.AccountCallStats, error)
}

type DashboardHandler struct {
	conversations DashboardConversationSummary
}

func NewDashboardHandler(conversations DashboardConversationSummary) *DashboardHandler {
	return &DashboardHandler{conversations: conversations}
}

func (h *DashboardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/dashboard/summary":
		totalConversations, err := h.conversations.CountConversations()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		activeConversations, err := h.conversations.CountActiveConversations()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		totalRuns, err := h.conversations.CountRuns()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		failoverRuns, err := h.conversations.CountFailoverRuns()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, map[string]int{
			"total_conversations":  totalConversations,
			"active_conversations": activeConversations,
			"total_runs":           totalRuns,
			"failover_runs":        failoverRuns,
		})
	case r.Method == http.MethodGet && r.URL.Path == "/dashboard/account-stats":
		stats, err := h.conversations.ListAccountCallStats()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		type accountStatsResponseItem struct {
			AccountID  int64          `json:"account_id"`
			TotalCalls int            `json:"total_calls"`
			Models     map[string]int `json:"models"`
		}
		response := make([]accountStatsResponseItem, 0, len(stats))
		for _, stat := range stats {
			response = append(response, accountStatsResponseItem{
				AccountID:  stat.AccountID,
				TotalCalls: stat.TotalCalls,
				Models:     stat.ModelCalls,
			})
		}
		writeJSON(w, http.StatusOK, response)
	default:
		http.NotFound(w, r)
	}
}
