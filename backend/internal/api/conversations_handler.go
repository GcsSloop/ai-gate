package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gcssloop/codex-router/backend/internal/conversations"
)

type ConversationQuery interface {
	ListConversations(offset, limit int) ([]conversations.Conversation, error)
	ListRuns(conversationID int64) ([]conversations.Run, error)
}

type ConversationsHandler struct {
	query ConversationQuery
}

func NewConversationsHandler(query ConversationQuery) *ConversationsHandler {
	return &ConversationsHandler{query: query}
}

func (h *ConversationsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/conversations":
		h.listConversations(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/conversations/") && strings.HasSuffix(r.URL.Path, "/runs"):
		h.listRuns(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *ConversationsHandler) listConversations(w http.ResponseWriter, r *http.Request) {
	page := parseIntDefault(r.URL.Query().Get("page"), 1)
	pageSize := parseIntDefault(r.URL.Query().Get("page_size"), 20)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	items, err := h.query.ListConversations((page-1)*pageSize, pageSize)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *ConversationsHandler) listRuns(w http.ResponseWriter, r *http.Request) {
	conversationID, err := parseConversationID(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	runs, err := h.query.ListRuns(conversationID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func parseConversationID(path string) (int64, error) {
	trimmed := strings.TrimPrefix(path, "/conversations/")
	trimmed = strings.TrimSuffix(trimmed, "/runs")
	trimmed = strings.Trim(trimmed, "/")
	return strconv.ParseInt(trimmed, 10, 64)
}

func parseIntDefault(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
