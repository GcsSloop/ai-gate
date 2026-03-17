package api

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/usage"
)

type DashboardUsageSummary interface {
	SummarizeEvents(filter usage.EventFilter) (usage.EventSummary, error)
	TrendEventsByHour(filter usage.EventFilter) ([]usage.TrendPoint, error)
	ListRecentEvents(filter usage.EventFilter) ([]usage.Event, error)
	ModelDistribution(filter usage.EventFilter) ([]usage.ModelDistributionPoint, error)
}

type DashboardHandler struct {
	usage       DashboardUsageSummary
	stateEvents *StateEventBus
}

type DashboardHandlerOption func(*DashboardHandler)

func WithDashboardStateEvents(bus *StateEventBus) DashboardHandlerOption {
	return func(handler *DashboardHandler) {
		handler.stateEvents = bus
	}
}

func NewDashboardHandler(usageRepo DashboardUsageSummary, opts ...DashboardHandlerOption) *DashboardHandler {
	handler := &DashboardHandler{usage: usageRepo}
	for _, opt := range opts {
		if opt != nil {
			opt(handler)
		}
	}
	return handler
}

func (h *DashboardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	filter := dashboardEventFilter(r)
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/dashboard/state-events":
		h.serveStateEvents(w, r)
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
	case r.Method == http.MethodGet && r.URL.Path == "/dashboard/model-distribution":
		distribution, err := h.usage.ModelDistribution(filter)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, distribution)
	default:
		http.NotFound(w, r)
	}
}

func (h *DashboardHandler) serveStateEvents(w http.ResponseWriter, r *http.Request) {
	if h.stateEvents == nil {
		http.Error(w, "state events unavailable", http.StatusServiceUnavailable)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	_, _ = io.WriteString(w, ": connected\n\n")
	flusher.Flush()

	events := h.stateEvents.Subscribe(r.Context())
	for {
		select {
		case <-r.Context().Done():
			return
		case topic, ok := <-events:
			if !ok {
				return
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", topic); err != nil {
				return
			}
			flusher.Flush()
		}
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
