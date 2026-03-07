package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/auth"
)

type AccountsHandler struct {
	repo       accounts.Repository
	connector  *auth.OAuthConnector
	stateStore *auth.StateStore
}

func NewAccountsHandler(repo accounts.Repository, connector *auth.OAuthConnector, stateStore *auth.StateStore) *AccountsHandler {
	return &AccountsHandler{repo: repo, connector: connector, stateStore: stateStore}
}

func (h *AccountsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/accounts":
		h.createAccount(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/accounts":
		h.listAccounts(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/accounts/auth/authorize":
		h.createAuthSession(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/accounts/") && strings.HasSuffix(r.URL.Path, "/disable"):
		h.disableAccount(w, r)
	default:
		http.NotFound(w, r)
	}
}

type createAccountRequest struct {
	ProviderType  accounts.ProviderType `json:"provider_type"`
	AccountName   string                `json:"account_name"`
	AuthMode      accounts.AuthMode     `json:"auth_mode"`
	BaseURL       string                `json:"base_url"`
	CredentialRef string                `json:"credential_ref"`
}

func (h *AccountsHandler) createAccount(w http.ResponseWriter, r *http.Request) {
	var req createAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := h.repo.Create(accounts.Account{
		ProviderType:  req.ProviderType,
		AccountName:   req.AccountName,
		AuthMode:      req.AuthMode,
		BaseURL:       req.BaseURL,
		CredentialRef: req.CredentialRef,
		Status:        accounts.StatusActive,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *AccountsHandler) listAccounts(w http.ResponseWriter, _ *http.Request) {
	accountList, err := h.repo.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type responseItem struct {
		ID                       int64                 `json:"id"`
		ProviderType             accounts.ProviderType `json:"provider_type"`
		AccountName              string                `json:"account_name"`
		AuthMode                 accounts.AuthMode     `json:"auth_mode"`
		BaseURL                  string                `json:"base_url"`
		Status                   accounts.Status       `json:"status"`
		CooldownRemainingSeconds *int64                `json:"cooldown_remaining_seconds,omitempty"`
	}

	response := make([]responseItem, 0, len(accountList))
	now := time.Now().UTC()
	for _, account := range accountList {
		item := responseItem{
			ID:           account.ID,
			ProviderType: account.ProviderType,
			AccountName:  account.AccountName,
			AuthMode:     account.AuthMode,
			BaseURL:      account.BaseURL,
			Status:       account.Status,
		}
		if account.CooldownUntil != nil {
			remaining := int64(account.CooldownUntil.Sub(now).Seconds())
			if remaining < 0 {
				remaining = 0
			}
			item.CooldownRemainingSeconds = &remaining
		}
		response = append(response, item)
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *AccountsHandler) createAuthSession(w http.ResponseWriter, _ *http.Request) {
	authURL, state, err := h.connector.AuthorizationURL()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := h.stateStore.New(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"authorization_url": authURL,
		"state":             state,
	})
}

func (h *AccountsHandler) disableAccount(w http.ResponseWriter, r *http.Request) {
	id, err := accountIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.repo.UpdateStatus(id, accounts.StatusDisabled); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func accountIDFromPath(path string) (int64, error) {
	trimmed := strings.TrimPrefix(path, "/accounts/")
	trimmed = strings.TrimSuffix(trimmed, "/disable")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return 0, errors.New("missing account id")
	}
	return strconv.ParseInt(trimmed, 10, 64)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
