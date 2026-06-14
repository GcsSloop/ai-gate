package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gcssloop/codex-router/backend/internal/serverauth"
	"github.com/gcssloop/codex-router/backend/internal/serverusers"
)

type ServerMeStore interface {
	Get(id int64) (serverusers.User, error)
	ListAssignedAccounts(userID int64) ([]serverusers.AssignedAccount, error)
	UpdateAccountState(userID int64, accountID int64, update serverusers.AccountStateUpdate) error
}

type ServerMeHandler struct {
	store ServerMeStore
}

func NewServerMeHandler(store ServerMeStore) *ServerMeHandler {
	return &ServerMeHandler{store: store}
}

func (h *ServerMeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/me":
		h.me(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/me/accounts":
		h.accounts(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/me/accounts/") && strings.HasSuffix(r.URL.Path, "/state"):
		h.updateAccountState(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *ServerMeHandler) me(w http.ResponseWriter, r *http.Request) {
	session, ok := serverauth.SessionFromContext(r.Context())
	if !ok || session.UserID <= 0 {
		http.Error(w, "user session required", http.StatusUnauthorized)
		return
	}
	user, err := h.store.Get(session.UserID)
	if err != nil {
		writeServerUserStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":          user,
		"request_count": user.RequestCount,
		"total_tokens":  user.TotalTokens,
	})
}

func (h *ServerMeHandler) accounts(w http.ResponseWriter, r *http.Request) {
	session, ok := serverauth.SessionFromContext(r.Context())
	if !ok || session.UserID <= 0 {
		http.Error(w, "user session required", http.StatusUnauthorized)
		return
	}
	accounts, err := h.store.ListAssignedAccounts(session.UserID)
	if err != nil {
		writeServerUserStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, accounts)
}

func (h *ServerMeHandler) updateAccountState(w http.ResponseWriter, r *http.Request) {
	session, ok := serverauth.SessionFromContext(r.Context())
	if !ok || session.UserID <= 0 {
		http.Error(w, "user session required", http.StatusUnauthorized)
		return
	}
	accountID, ok := meAccountIDFromStatePath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	var payload serverusers.AccountStateUpdate
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid account state payload", http.StatusBadRequest)
		return
	}
	if err := h.store.UpdateAccountState(session.UserID, accountID, payload); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "assigned account not found", http.StatusNotFound)
			return
		}
		writeServerUserStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": true})
}

func meAccountIDFromStatePath(path string) (int64, bool) {
	trimmed := strings.TrimPrefix(path, "/me/accounts/")
	trimmed = strings.TrimSuffix(trimmed, "/state")
	trimmed = strings.Trim(trimmed, "/")
	id, err := strconv.ParseInt(trimmed, 10, 64)
	return id, err == nil && id > 0
}
