package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gcssloop/codex-router/backend/internal/serverusers"
)

type ServerUsersStore interface {
	Create(name string) (serverusers.CreatedUser, error)
	List() ([]serverusers.User, error)
	Disable(id int64) error
	RotateToken(id int64) (serverusers.CreatedUser, error)
	ListAccountAssignments(userID int64) ([]serverusers.AccountAssignment, error)
	SetAccountAssignments(userID int64, accountIDs []int64) error
}

type ServerUsersHandler struct {
	store ServerUsersStore
}

func NewServerUsersHandler(store ServerUsersStore) *ServerUsersHandler {
	return &ServerUsersHandler{store: store}
}

func (h *ServerUsersHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/server-users":
		h.list(w)
	case r.Method == http.MethodPost && r.URL.Path == "/server-users":
		h.create(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/server-users/") && strings.HasSuffix(r.URL.Path, "/accounts"):
		h.listAccountAssignments(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/server-users/") && strings.HasSuffix(r.URL.Path, "/accounts"):
		h.setAccountAssignments(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/server-users/") && strings.HasSuffix(r.URL.Path, "/disable"):
		h.disable(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/server-users/") && strings.HasSuffix(r.URL.Path, "/rotate-token"):
		h.rotate(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *ServerUsersHandler) listAccountAssignments(w http.ResponseWriter, r *http.Request) {
	id, ok := serverUserIDFromActionPath(r.URL.Path, "/accounts")
	if !ok {
		http.NotFound(w, r)
		return
	}
	assignments, err := h.store.ListAccountAssignments(id)
	if err != nil {
		writeServerUserStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, assignments)
}

func (h *ServerUsersHandler) setAccountAssignments(w http.ResponseWriter, r *http.Request) {
	id, ok := serverUserIDFromActionPath(r.URL.Path, "/accounts")
	if !ok {
		http.NotFound(w, r)
		return
	}
	var payload struct {
		AccountIDs []int64 `json:"account_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid server user account assignment payload", http.StatusBadRequest)
		return
	}
	if err := h.store.SetAccountAssignments(id, payload.AccountIDs); err != nil {
		writeServerUserStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"assigned": true})
}

func (h *ServerUsersHandler) list(w http.ResponseWriter) {
	users, err := h.store.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (h *ServerUsersHandler) create(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid server user payload", http.StatusBadRequest)
		return
	}
	created, err := h.store.Create(payload.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *ServerUsersHandler) disable(w http.ResponseWriter, r *http.Request) {
	id, ok := serverUserIDFromActionPath(r.URL.Path, "/disable")
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := h.store.Disable(id); err != nil {
		writeServerUserStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"disabled": true})
}

func (h *ServerUsersHandler) rotate(w http.ResponseWriter, r *http.Request) {
	id, ok := serverUserIDFromActionPath(r.URL.Path, "/rotate-token")
	if !ok {
		http.NotFound(w, r)
		return
	}
	created, err := h.store.RotateToken(id)
	if err != nil {
		writeServerUserStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, created)
}

func serverUserIDFromActionPath(path string, suffix string) (int64, bool) {
	trimmed := strings.TrimSuffix(strings.TrimPrefix(path, "/server-users/"), suffix)
	trimmed = strings.Trim(trimmed, "/")
	id, err := strconv.ParseInt(trimmed, 10, 64)
	return id, err == nil && id > 0
}

func writeServerUserStoreError(w http.ResponseWriter, err error) {
	if err == sql.ErrNoRows {
		http.Error(w, "server user not found", http.StatusNotFound)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
