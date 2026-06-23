package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/routing"
	"github.com/gcssloop/codex-router/backend/internal/serverauth"
	"github.com/gcssloop/codex-router/backend/internal/serverusers"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

type ServerMeStore interface {
	Get(id int64) (serverusers.User, error)
	UpdateRoute(id int64, accountID *int64, locked bool) (serverusers.User, error)
}

type ServerMeAccounts interface {
	List() ([]accounts.Account, error)
	Update(account accounts.Account) error
}

type ServerMeUsage interface {
	ListLatest() ([]usage.Snapshot, error)
}

type ServerMeHandler struct {
	store    ServerMeStore
	accounts ServerMeAccounts
	usage    ServerMeUsage
	sticky   *routing.StickySelector
}

type ServerMeHandlerOption func(*ServerMeHandler)

func WithServerMeAccounts(repo ServerMeAccounts) ServerMeHandlerOption {
	return func(handler *ServerMeHandler) {
		handler.accounts = repo
	}
}

func WithServerMeUsage(repo ServerMeUsage) ServerMeHandlerOption {
	return func(handler *ServerMeHandler) {
		handler.usage = repo
	}
}

func WithServerMeStickySelector(sticky *routing.StickySelector) ServerMeHandlerOption {
	return func(handler *ServerMeHandler) {
		handler.sticky = sticky
	}
}

func NewServerMeHandler(store ServerMeStore, opts ...ServerMeHandlerOption) *ServerMeHandler {
	handler := &ServerMeHandler{store: store}
	for _, opt := range opts {
		if opt != nil {
			opt(handler)
		}
	}
	return handler
}

func (h *ServerMeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/me":
		h.me(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/me/upstreams":
		h.upstreams(w, r)
	case r.Method == http.MethodPut && r.URL.Path == "/me/route":
		h.updateRoute(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/me/upstreams/") && strings.HasSuffix(r.URL.Path, "/lock"):
		h.updateUpstreamLock(w, r)
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

func (h *ServerMeHandler) upstreams(w http.ResponseWriter, r *http.Request) {
	session, ok := serverauth.SessionFromContext(r.Context())
	if !ok || session.UserID <= 0 {
		http.Error(w, "user session required", http.StatusUnauthorized)
		return
	}
	if h.accounts == nil {
		http.Error(w, "upstream accounts are not configured", http.StatusInternalServerError)
		return
	}
	user, err := h.store.Get(session.UserID)
	if err != nil {
		writeServerUserStoreError(w, err)
		return
	}
	accountList, err := h.accounts.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	usageByAccount := map[int64]usage.Snapshot{}
	if h.usage != nil {
		if snapshots, err := h.usage.ListLatest(); err == nil {
			for _, snapshot := range snapshots {
				usageByAccount[snapshot.AccountID] = snapshot
			}
		}
	}
	currentAccountID := h.currentAccountID(user, accountList, usageByAccount)
	response := h.buildUpstreamsResponse(user, accountList, usageByAccount, currentAccountID)
	writeJSON(w, http.StatusOK, response)
}

func (h *ServerMeHandler) updateRoute(w http.ResponseWriter, r *http.Request) {
	session, ok := serverauth.SessionFromContext(r.Context())
	if !ok || session.UserID <= 0 {
		http.Error(w, "user session required", http.StatusUnauthorized)
		return
	}
	var payload struct {
		AccountID *int64 `json:"account_id"`
		Locked    bool   `json:"locked"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid route payload", http.StatusBadRequest)
		return
	}
	if payload.AccountID != nil {
		if *payload.AccountID <= 0 {
			http.Error(w, "account_id must be positive", http.StatusBadRequest)
			return
		}
		if h.accounts == nil {
			http.Error(w, "upstream accounts are not configured", http.StatusInternalServerError)
			return
		}
		account, err := h.findAccount(*payload.AccountID)
		if err != nil {
			http.Error(w, "upstream account not found", http.StatusNotFound)
			return
		}
		if !serverMeAccountManuallySelectable(account) {
			http.Error(w, "upstream account is not available", http.StatusBadRequest)
			return
		}
	}
	if payload.AccountID == nil && payload.Locked {
		http.Error(w, "locked route requires account_id", http.StatusBadRequest)
		return
	}
	user, err := h.store.UpdateRoute(session.UserID, payload.AccountID, payload.Locked)
	if err != nil {
		writeServerUserStoreError(w, err)
		return
	}
	if h.sticky != nil && payload.AccountID != nil {
		rememberServerUserSticky(h.sticky, session.UserID, *payload.AccountID)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":                 user,
		"preferred_account_id": user.PreferredAccountID,
		"route_locked":         user.RouteLocked,
	})
}

func (h *ServerMeHandler) updateUpstreamLock(w http.ResponseWriter, r *http.Request) {
	session, ok := serverauth.SessionFromContext(r.Context())
	if !ok || session.UserID <= 0 {
		http.Error(w, "user session required", http.StatusUnauthorized)
		return
	}
	if h.accounts == nil {
		http.Error(w, "upstream accounts are not configured", http.StatusInternalServerError)
		return
	}
	accountID, err := serverMeUpstreamAccountIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var payload struct {
		Locked bool `json:"locked"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid lock payload", http.StatusBadRequest)
		return
	}
	account, err := h.findAccount(accountID)
	if err != nil {
		http.Error(w, "upstream account not found", http.StatusNotFound)
		return
	}
	account.IsLocked = payload.Locked
	if err := h.accounts.Update(account); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":             account.ID,
		"account_locked": account.IsLocked,
	})
}

func serverMeUpstreamAccountIDFromPath(raw string) (int64, error) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(raw), "/me/upstreams/")
	trimmed = strings.TrimSuffix(trimmed, "/lock")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return 0, errors.New("missing account id")
	}
	return strconv.ParseInt(trimmed, 10, 64)
}

type serverMeUpstreamsResponse struct {
	TotalAccounts      int                       `json:"total_accounts"`
	AvailableAccounts  int                       `json:"available_accounts"`
	CurrentAccountID   int64                     `json:"current_account_id,omitempty"`
	PreferredAccountID *int64                    `json:"preferred_account_id,omitempty"`
	RouteLocked        bool                      `json:"route_locked"`
	Accounts           []serverMeUpstreamAccount `json:"accounts"`
}

type serverMeUpstreamAccount struct {
	ID                              int64                 `json:"id"`
	ProviderType                    accounts.ProviderType `json:"provider_type"`
	AccountName                     string                `json:"account_name"`
	AuthMode                        accounts.AuthMode     `json:"auth_mode"`
	SourceIcon                      string                `json:"source_icon"`
	BaseURL                         string                `json:"base_url"`
	Status                          accounts.Status       `json:"status"`
	Available                       bool                  `json:"available"`
	Current                         bool                  `json:"current"`
	Preferred                       bool                  `json:"preferred"`
	AccountLocked                   bool                  `json:"account_locked"`
	SupportsResponses               bool                  `json:"supports_responses"`
	CooldownRemainingSeconds        *int64                `json:"cooldown_remaining_seconds,omitempty"`
	RoutingCooldownRemainingSeconds *int64                `json:"routing_cooldown_remaining_seconds,omitempty"`
	RoutingCooldownReason           string                `json:"routing_cooldown_reason,omitempty"`
	Balance                         float64               `json:"balance"`
	QuotaRemaining                  float64               `json:"quota_remaining"`
	RPMRemaining                    float64               `json:"rpm_remaining"`
	TPMRemaining                    float64               `json:"tpm_remaining"`
	HealthScore                     float64               `json:"health_score"`
	RecentErrorRate                 float64               `json:"recent_error_rate"`
	LastTotalTokens                 float64               `json:"last_total_tokens"`
	LastInputTokens                 float64               `json:"last_input_tokens"`
	LastOutputTokens                float64               `json:"last_output_tokens"`
	ModelContextWindow              float64               `json:"model_context_window"`
	PrimaryUsedPercent              float64               `json:"primary_used_percent"`
	SecondaryUsedPercent            float64               `json:"secondary_used_percent"`
	PrimaryResetsAt                 *time.Time            `json:"primary_resets_at,omitempty"`
	SecondaryResetsAt               *time.Time            `json:"secondary_resets_at,omitempty"`
	CheckedAt                       *time.Time            `json:"checked_at,omitempty"`
	Stale                           bool                  `json:"stale"`
	LastError                       string                `json:"last_error,omitempty"`
	PPChatTodayUsedQuota            float64               `json:"ppchat_today_used_quota,omitempty"`
	PPChatTodayAddedQuota           float64               `json:"ppchat_today_added_quota,omitempty"`
	PPChatTodayRemainingQuota       float64               `json:"ppchat_today_remaining_quota,omitempty"`
	UsageDisplay                    map[string]any        `json:"usage_display,omitempty"`
}

func (h *ServerMeHandler) buildUpstreamsResponse(user serverusers.User, accountList []accounts.Account, usageByAccount map[int64]usage.Snapshot, currentAccountID int64) serverMeUpstreamsResponse {
	now := time.Now().UTC()
	items := make([]serverMeUpstreamAccount, 0, len(accountList))
	availableCount := 0
	for _, account := range accountList {
		account = applyBuiltInDriverDefaults(account)
		snapshot := usageByAccount[account.ID]
		available := serverMeAccountAvailable(account, now)
		if available {
			availableCount++
		}
		preferred := user.PreferredAccountID != nil && *user.PreferredAccountID == account.ID
		ppchatSummary := parsePPChatUsageSummary(snapshot.ProviderSnapshotJSON)
		item := serverMeUpstreamAccount{
			ID:                        account.ID,
			ProviderType:              account.ProviderType,
			AccountName:               account.AccountName,
			AuthMode:                  account.AuthMode,
			SourceIcon:                normalizeAccountSourceIcon(account.SourceIcon),
			BaseURL:                   account.BaseURL,
			Status:                    account.Status,
			Available:                 available,
			Current:                   currentAccountID == account.ID,
			Preferred:                 preferred,
			AccountLocked:             account.IsLocked,
			SupportsResponses:         account.NativeResponsesCapable(),
			Balance:                   snapshot.Balance,
			QuotaRemaining:            snapshot.QuotaRemaining,
			RPMRemaining:              snapshot.RPMRemaining,
			TPMRemaining:              snapshot.TPMRemaining,
			HealthScore:               snapshot.HealthScore,
			RecentErrorRate:           snapshot.RecentErrorRate,
			LastTotalTokens:           snapshot.LastTotalTokens,
			LastInputTokens:           snapshot.LastInputTokens,
			LastOutputTokens:          snapshot.LastOutputTokens,
			ModelContextWindow:        snapshot.ModelContextWindow,
			PrimaryUsedPercent:        snapshot.PrimaryUsedPercent,
			SecondaryUsedPercent:      snapshot.SecondaryUsedPercent,
			PrimaryResetsAt:           snapshot.PrimaryResetsAt,
			SecondaryResetsAt:         snapshot.SecondaryResetsAt,
			CheckedAt:                 nilIfZeroTime(snapshot.CheckedAt),
			Stale:                     snapshot.Stale,
			LastError:                 snapshot.LastError,
			PPChatTodayUsedQuota:      ppchatSummary.TodayUsedQuota,
			PPChatTodayAddedQuota:     ppchatSummary.TodayAddedQuota,
			PPChatTodayRemainingQuota: ppchatSummary.TodayRemainingQuota,
			UsageDisplay:              parseUsageDisplay(snapshot.ProviderSnapshotJSON),
		}
		if account.CooldownUntil != nil {
			remaining := int64(account.CooldownUntil.Sub(now).Seconds())
			if remaining < 0 {
				remaining = 0
			}
			item.CooldownRemainingSeconds = &remaining
			item.RoutingCooldownRemainingSeconds = &remaining
			item.RoutingCooldownReason = account.CooldownReason
		}
		items = append(items, item)
	}
	return serverMeUpstreamsResponse{
		TotalAccounts:      len(items),
		AvailableAccounts:  availableCount,
		CurrentAccountID:   currentAccountID,
		PreferredAccountID: user.PreferredAccountID,
		RouteLocked:        user.RouteLocked,
		Accounts:           items,
	}
}

func (h *ServerMeHandler) currentAccountID(user serverusers.User, accountList []accounts.Account, usageByAccount map[int64]usage.Snapshot) int64 {
	if user.RouteLocked && user.PreferredAccountID != nil {
		return *user.PreferredAccountID
	}
	if h.sticky != nil {
		if accountID, ok := h.sticky.Current(serverUserRouteScope(user.ID, "responses")); ok {
			return accountID
		}
		if accountID, ok := h.sticky.Current(serverUserRouteScope(user.ID, "chat_completions")); ok {
			return accountID
		}
	}
	if user.PreferredAccountID != nil {
		return *user.PreferredAccountID
	}
	candidates := make([]routing.Candidate, 0, len(accountList))
	for _, account := range accountList {
		if !serverMeAccountAvailable(account, time.Now().UTC()) {
			continue
		}
		candidates = append(candidates, routing.Candidate{Account: account, Snapshot: usageByAccount[account.ID]})
	}
	scored := routing.ScoreCandidates(candidates)
	if len(scored) == 0 {
		return 0
	}
	return scored[0].Account.ID
}

func (h *ServerMeHandler) findAccount(accountID int64) (accounts.Account, error) {
	accountList, err := h.accounts.List()
	if err != nil {
		return accounts.Account{}, err
	}
	for _, account := range accountList {
		if account.ID == accountID {
			return account, nil
		}
	}
	return accounts.Account{}, errors.New("account not found")
}

func serverMeAccountAvailable(account accounts.Account, now time.Time) bool {
	switch account.Status {
	case accounts.StatusDisabled, accounts.StatusInvalid:
		return false
	}
	if account.IsLocked {
		return false
	}
	if account.RoutingCooldownActive(now) {
		return false
	}
	return true
}

func serverMeAccountManuallySelectable(account accounts.Account) bool {
	switch account.Status {
	case accounts.StatusDisabled, accounts.StatusInvalid:
		return false
	}
	return true
}
