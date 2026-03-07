package api

import "net/http"

type DashboardConversationSummary interface {
	CountConversations() (int, error)
	CountActiveConversations() (int, error)
	CountRuns() (int, error)
	CountFailoverRuns() (int, error)
}

type DashboardHandler struct {
	conversations DashboardConversationSummary
}

func NewDashboardHandler(conversations DashboardConversationSummary) *DashboardHandler {
	return &DashboardHandler{conversations: conversations}
}

func (h *DashboardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.URL.Path != "/dashboard/summary" {
		http.NotFound(w, r)
		return
	}

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
}
