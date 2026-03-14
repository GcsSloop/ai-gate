package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/usage"
)

type DashboardUsageSummary interface {
	SummarizeEvents(filter usage.EventFilter) (usage.EventSummary, error)
	TrendEventsByHour(filter usage.EventFilter) ([]usage.TrendPoint, error)
	ListRecentEvents(filter usage.EventFilter) ([]usage.Event, error)
}

type DashboardHandler struct {
	usage DashboardUsageSummary
}

func NewDashboardHandler(usageRepo DashboardUsageSummary) *DashboardHandler {
	return &DashboardHandler{usage: usageRepo}
}

func (h *DashboardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	filter := dashboardEventFilter(r)
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/dashboard/summary":
		summary, err := h.usage.SummarizeEvents(filter)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, summary)
	case r.Method == http.MethodGet && r.URL.Path == "/dashboard/trends":
		trends, err := h.usage.TrendEventsByHour(filter)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, trends)
	case r.Method == http.MethodGet && r.URL.Path == "/dashboard/recent-events":
		events, err := h.usage.ListRecentEvents(filter)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, events)
	default:
		http.NotFound(w, r)
	}
}

func dashboardEventFilter(r *http.Request) usage.EventFilter {
	query := r.URL.Query()
	filter := usage.EventFilter{}

	hours, _ := strconv.Atoi(query.Get("hours"))
	if hours <= 0 {
		hours = 24
	}
	from := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)
	filter.From = &from

	if raw := query.Get("account_id"); raw != "" {
		if accountID, err := strconv.ParseInt(raw, 10, 64); err == nil {
			filter.AccountID = &accountID
		}
	}
	filter.Model = query.Get("model")

	if raw := query.Get("limit"); raw != "" {
		if limit, err := strconv.Atoi(raw); err == nil && limit > 0 {
			filter.Limit = limit
		}
	}
	if filter.Limit == 0 {
		filter.Limit = 20
	}
	return filter
}
