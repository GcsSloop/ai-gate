package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/auth"
	"github.com/gcssloop/codex-router/backend/internal/providers"
	providercodex "github.com/gcssloop/codex-router/backend/internal/providers/codex"
	provideropenai "github.com/gcssloop/codex-router/backend/internal/providers/openai"
	"github.com/gcssloop/codex-router/backend/internal/settings"
	"github.com/gcssloop/codex-router/backend/internal/usage"
	luadrv "github.com/gcssloop/codex-router/backend/internal/usagedrv/lua"
	driverregistry "github.com/gcssloop/codex-router/backend/internal/usagedrv/registry"
)

const officialOpenAIBaseURL = "https://api.openai.com/v1"

type AccountsHandler struct {
	repo        accounts.Repository
	usage       AccountsUsage
	connector   *auth.OAuthConnector
	stateStore  *auth.StateStore
	client      *http.Client
	stateEvents *StateEventBus
	refresher   AccountsUsageRefresher
	settings    accountsSettingsReader
	refreshTTL  time.Duration
	drivers     *driverregistry.Registry
	luaScripts  *luadrv.ManagedScriptStore
	luaRuntime  *luadrv.Runtime
}

type accountsSettingsReader interface {
	GetAppSettings() (settings.AppSettings, error)
}

type AccountsHandlerOption func(*AccountsHandler)

func WithAccountsStateEvents(bus *StateEventBus) AccountsHandlerOption {
	return func(handler *AccountsHandler) {
		handler.stateEvents = bus
	}
}

func WithAccountsUsageRefresher(refresher AccountsUsageRefresher) AccountsHandlerOption {
	return func(handler *AccountsHandler) {
		handler.refresher = refresher
	}
}

func WithAccountsSettings(repo accountsSettingsReader) AccountsHandlerOption {
	return func(handler *AccountsHandler) {
		handler.settings = repo
	}
}

func WithAccountsDriverRegistry(reg *driverregistry.Registry) AccountsHandlerOption {
	return func(handler *AccountsHandler) {
		handler.drivers = reg
	}
}

func WithAccountsLuaScriptRoot(root string) AccountsHandlerOption {
	return func(handler *AccountsHandler) {
		if strings.TrimSpace(root) == "" {
			return
		}
		store, err := luadrv.NewManagedScriptStore(root)
		if err != nil {
			return
		}
		handler.luaScripts = store
		if handler.luaRuntime == nil {
			handler.luaRuntime = luadrv.NewRuntime(handler.client, "")
		}
	}
}

type AccountsUsage interface {
	ListLatest() ([]usage.Snapshot, error)
	Save(snapshot usage.Snapshot) error
}

type AccountsUsageRefresher interface {
	Run(ctx context.Context, runAt time.Time) error
}

func NewAccountsHandler(repo accounts.Repository, usage AccountsUsage, connector *auth.OAuthConnector, stateStore *auth.StateStore, opts ...AccountsHandlerOption) *AccountsHandler {
	handler := &AccountsHandler{
		repo:       repo,
		usage:      usage,
		connector:  connector,
		stateStore: stateStore,
		client:     http.DefaultClient,
		refreshTTL: 15 * time.Second,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(handler)
		}
	}
	return handler
}

func (h *AccountsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/accounts":
		h.createAccount(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/accounts/usage/refresh":
		h.refreshAccountsUsage(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/accounts/usage":
		h.listAccountsUsage(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/accounts":
		h.listAccounts(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/accounts/auth/authorize":
		h.createAuthSession(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/accounts/import-local":
		h.importLocalAuth(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/accounts/import-current":
		h.importCurrentAuth(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/accounts/import-shared":
		h.importSharedAccount(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/accounts/usage-scripts":
		h.listLuaUsageScripts(w)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/accounts/usage-scripts/"):
		h.getLuaUsageScript(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/accounts/usage-scripts/"):
		h.putLuaUsageScript(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/accounts/") && countPathSegments(r.URL.Path) == 2:
		h.updateAccount(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/accounts/") && countPathSegments(r.URL.Path) == 2:
		h.deleteAccount(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/accounts/") && strings.HasSuffix(r.URL.Path, "/duplicate"):
		h.duplicateAccount(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/accounts/") && strings.HasSuffix(r.URL.Path, "/share"):
		h.shareAccount(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/accounts/") && strings.HasSuffix(r.URL.Path, "/test"):
		h.testAccount(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/accounts/") && strings.HasSuffix(r.URL.Path, "/usage-lua-test"):
		h.testLuaUsage(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/accounts/") && strings.HasSuffix(r.URL.Path, "/ppchat-token-logs"):
		h.getPPChatTokenLogs(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/accounts/") && strings.HasSuffix(r.URL.Path, "/disable"):
		h.disableAccount(w, r)
	default:
		http.NotFound(w, r)
	}
}

type createAccountRequest struct {
	ProviderType      accounts.ProviderType `json:"provider_type"`
	AccountName       string                `json:"account_name"`
	SourceIcon        string                `json:"source_icon"`
	AuthMode          accounts.AuthMode     `json:"auth_mode"`
	BaseURL           string                `json:"base_url"`
	CredentialRef     string                `json:"credential_ref"`
	AccountDriver     string                `json:"account_driver"`
	UsageDriver       string                `json:"usage_driver"`
	UsageConfigJSON   string                `json:"usage_config_json"`
	SupportsResponses *bool                 `json:"supports_responses"`
}

type importLocalAuthRequest struct {
	AccountName string `json:"account_name"`
	Content     string `json:"content"`
	Path        string `json:"path"`
}

type importCurrentAuthRequest struct {
	AccountName string `json:"account_name"`
}

type importSharedAccountRequest struct {
	Payload string `json:"payload"`
}

type accountShareEnvelope struct {
	Kind          string              `json:"kind"`
	SchemaVersion int                 `json:"schema_version"`
	ExportedAt    string              `json:"exported_at"`
	Account       accountSharePayload `json:"account"`
}

type accountSharePayload struct {
	ProviderType      accounts.ProviderType `json:"provider_type"`
	AccountName       string                `json:"account_name"`
	SourceIcon        string                `json:"source_icon"`
	AuthMode          accounts.AuthMode     `json:"auth_mode"`
	BaseURL           string                `json:"base_url"`
	CredentialRef     string                `json:"credential_ref"`
	AccountDriver     string                `json:"account_driver"`
	UsageDriver       string                `json:"usage_driver"`
	UsageConfigJSON   string                `json:"usage_config_json"`
	SupportsResponses bool                  `json:"supports_responses"`
}

type updateAccountRequest struct {
	AccountName       string          `json:"account_name"`
	SourceIcon        string          `json:"source_icon"`
	BaseURL           string          `json:"base_url"`
	CredentialRef     string          `json:"credential_ref"`
	AccountDriver     *string         `json:"account_driver"`
	UsageDriver       *string         `json:"usage_driver"`
	UsageConfigJSON   *string         `json:"usage_config_json"`
	Status            accounts.Status `json:"status"`
	Priority          *int            `json:"priority"`
	IsActive          *bool           `json:"is_active"`
	SupportsResponses *bool           `json:"supports_responses"`
}

type accountTestResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
	Content string `json:"content,omitempty"`
}

type accountChatTestRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type luaUsageScriptRequest struct {
	Content string `json:"content"`
}

type luaUsageScriptResponse struct {
	Key     string `json:"key"`
	Content string `json:"content,omitempty"`
}

type luaUsageScriptListResponse struct {
	Items []string `json:"items"`
}

type luaUsageTestRequest struct {
	UsageConfigJSON string `json:"usage_config_json"`
	ScriptContent   string `json:"script_content"`
}

const accountShareKind = "aigate-account-share"
const accountShareSchemaVersion = 1

func (h *AccountsHandler) createAccount(w http.ResponseWriter, r *http.Request) {
	var req createAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	supportsResponses := true
	if req.SupportsResponses != nil {
		supportsResponses = *req.SupportsResponses
	}
	if req.ProviderType == accounts.ProviderOpenAIOfficial {
		supportsResponses = true
	}

	account := applyBuiltInDriverDefaults(accounts.Account{
		ProviderType:      req.ProviderType,
		AccountName:       req.AccountName,
		SourceIcon:        normalizeAccountSourceIcon(req.SourceIcon),
		AuthMode:          req.AuthMode,
		BaseURL:           req.BaseURL,
		CredentialRef:     req.CredentialRef,
		AccountDriver:     strings.TrimSpace(req.AccountDriver),
		UsageDriver:       strings.TrimSpace(req.UsageDriver),
		UsageConfigJSON:   strings.TrimSpace(req.UsageConfigJSON),
		Status:            accounts.StatusActive,
		SupportsResponses: supportsResponses,
	})

	err := h.repo.Create(account)
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
		SourceIcon               string                `json:"source_icon"`
		BaseURL                  string                `json:"base_url"`
		Status                   accounts.Status       `json:"status"`
		CooldownRemainingSeconds *int64                `json:"cooldown_remaining_seconds,omitempty"`
		Balance                  float64               `json:"balance"`
		QuotaRemaining           float64               `json:"quota_remaining"`
		RPMRemaining             float64               `json:"rpm_remaining"`
		TPMRemaining             float64               `json:"tpm_remaining"`
		HealthScore              float64               `json:"health_score"`
		RecentErrorRate          float64               `json:"recent_error_rate"`
		LastTotalTokens          float64               `json:"last_total_tokens"`
		LastInputTokens          float64               `json:"last_input_tokens"`
		LastOutputTokens         float64               `json:"last_output_tokens"`
		ModelContextWindow       float64               `json:"model_context_window"`
		PrimaryUsedPercent       float64               `json:"primary_used_percent"`
		SecondaryUsedPercent     float64               `json:"secondary_used_percent"`
		PrimaryResetsAt          *time.Time            `json:"primary_resets_at,omitempty"`
		SecondaryResetsAt        *time.Time            `json:"secondary_resets_at,omitempty"`
		AccountDriver            string                `json:"account_driver"`
		UsageDriver              string                `json:"usage_driver"`
		UsageConfigJSON          string                `json:"usage_config_json"`
		Priority                 int                   `json:"priority"`
		IsActive                 bool                  `json:"is_active"`
		SupportsResponses        bool                  `json:"supports_responses"`
	}

	response := make([]responseItem, 0, len(accountList))
	now := time.Now().UTC()
	for _, account := range accountList {
		account = applyBuiltInDriverDefaults(account)
		item := responseItem{
			ID:                   account.ID,
			ProviderType:         account.ProviderType,
			AccountName:          account.AccountName,
			AuthMode:             account.AuthMode,
			SourceIcon:           normalizeAccountSourceIcon(account.SourceIcon),
			BaseURL:              account.BaseURL,
			Status:               account.Status,
			Priority:             account.Priority,
			IsActive:             account.IsActive,
			SupportsResponses:    account.SupportsResponses,
			Balance:              0,
			QuotaRemaining:       0,
			RPMRemaining:         0,
			TPMRemaining:         0,
			HealthScore:          0,
			RecentErrorRate:      0,
			LastTotalTokens:      0,
			LastInputTokens:      0,
			LastOutputTokens:     0,
			ModelContextWindow:   0,
			PrimaryUsedPercent:   0,
			SecondaryUsedPercent: 0,
			AccountDriver:        account.AccountDriver,
			UsageDriver:          account.UsageDriver,
			UsageConfigJSON:      account.UsageConfigJSON,
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

func (h *AccountsHandler) listAccountsUsage(w http.ResponseWriter, _ *http.Request) {
	accountList, err := h.repo.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type responseItem struct {
		AccountID                 int64      `json:"account_id"`
		Balance                   float64    `json:"balance"`
		QuotaRemaining            float64    `json:"quota_remaining"`
		RPMRemaining              float64    `json:"rpm_remaining"`
		TPMRemaining              float64    `json:"tpm_remaining"`
		HealthScore               float64    `json:"health_score"`
		RecentErrorRate           float64    `json:"recent_error_rate"`
		LastTotalTokens           float64    `json:"last_total_tokens"`
		LastInputTokens           float64    `json:"last_input_tokens"`
		LastOutputTokens          float64    `json:"last_output_tokens"`
		ModelContextWindow        float64    `json:"model_context_window"`
		PrimaryUsedPercent        float64    `json:"primary_used_percent"`
		SecondaryUsedPercent      float64    `json:"secondary_used_percent"`
		PrimaryResetsAt           *time.Time `json:"primary_resets_at,omitempty"`
		SecondaryResetsAt         *time.Time `json:"secondary_resets_at,omitempty"`
		CheckedAt                 *time.Time `json:"checked_at,omitempty"`
		Stale                     bool       `json:"stale"`
		LastError                 string     `json:"last_error,omitempty"`
		PPChatTodayUsedQuota      float64    `json:"ppchat_today_used_quota,omitempty"`
		PPChatTodayAddedQuota     float64    `json:"ppchat_today_added_quota,omitempty"`
		PPChatTodayRemainingQuota float64    `json:"ppchat_today_remaining_quota,omitempty"`
	}

	usageByAccount := map[int64]usage.Snapshot{}
	if h.usage != nil {
		if snapshots, err := h.usage.ListLatest(); err == nil {
			for _, snapshot := range snapshots {
				usageByAccount[snapshot.AccountID] = snapshot
			}
		}
	}

	response := make([]responseItem, 0, len(accountList))
	for _, account := range accountList {
		snapshot := usageByAccount[account.ID]
		ppchatSummary := parsePPChatUsageSummary(snapshot.ProviderSnapshotJSON)
		response = append(response, responseItem{
			AccountID:                 account.ID,
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
		})
	}

	writeJSON(w, http.StatusOK, response)
}

func nilIfZeroTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	normalized := value.UTC()
	return &normalized
}

type ppchatUsageSummary struct {
	TodayUsedQuota      float64
	TodayAddedQuota     float64
	TodayRemainingQuota float64
}

func parsePPChatUsageSummary(raw string) ppchatUsageSummary {
	if strings.TrimSpace(raw) == "" {
		return ppchatUsageSummary{}
	}
	var decoded struct {
		Payload struct {
			Data struct {
				TokenInfo struct {
					TodayUsedQuota     float64 `json:"today_used_quota"`
					TodayAddedQuota    float64 `json:"today_added_quota"`
					RemainQuotaDisplay float64 `json:"remain_quota_display"`
				} `json:"token_info"`
			} `json:"data"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return ppchatUsageSummary{}
	}
	addedQuota := decoded.Payload.Data.TokenInfo.TodayAddedQuota
	if addedQuota <= 0 {
		addedQuota = decoded.Payload.Data.TokenInfo.TodayUsedQuota + maxFloat64(decoded.Payload.Data.TokenInfo.RemainQuotaDisplay, 0)
	}
	return ppchatUsageSummary{
		TodayUsedQuota:      decoded.Payload.Data.TokenInfo.TodayUsedQuota,
		TodayAddedQuota:     addedQuota,
		TodayRemainingQuota: decoded.Payload.Data.TokenInfo.RemainQuotaDisplay,
	}
}

func maxFloat64(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func (h *AccountsHandler) refreshAccountsUsage(w http.ResponseWriter, r *http.Request) {
	if h.refresher == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	timeout := h.refreshTTL
	if h.settings != nil {
		if appSettings, err := h.settings.GetAppSettings(); err == nil && appSettings.UsageRequestTimeoutSeconds > 0 {
			timeout = time.Duration(appSettings.UsageRequestTimeoutSeconds) * time.Second
		}
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	if err := h.refresher.Run(ctx, time.Now().UTC()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

func (h *AccountsHandler) importLocalAuth(w http.ResponseWriter, r *http.Request) {
	accountName, raw, err := decodeLocalImportRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if accountName == "" {
		accountName = "local-codex"
	}

	err = h.repo.Create(applyBuiltInDriverDefaults(accounts.Account{
		ProviderType:      accounts.ProviderOpenAIOfficial,
		AccountName:       accountName,
		SourceIcon:        "openai",
		AuthMode:          accounts.AuthModeLocalImport,
		CredentialRef:     string(raw),
		BaseURL:           officialCodexBaseURL,
		Status:            accounts.StatusActive,
		SupportsResponses: true,
	}))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *AccountsHandler) importCurrentAuth(w http.ResponseWriter, r *http.Request) {
	var req importCurrentAuthRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	accountName := strings.TrimSpace(req.AccountName)
	if accountName == "" {
		accountName = "local-codex"
	}

	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	authPath := filepath.Join(home, ".codex", "auth.json")
	_, raw, err := auth.LoadLocalAuthFile(authPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err = h.repo.Create(applyBuiltInDriverDefaults(accounts.Account{
		ProviderType:      accounts.ProviderOpenAIOfficial,
		AccountName:       accountName,
		SourceIcon:        "openai",
		AuthMode:          accounts.AuthModeLocalImport,
		CredentialRef:     string(raw),
		BaseURL:           officialCodexBaseURL,
		Status:            accounts.StatusActive,
		SupportsResponses: true,
	}))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *AccountsHandler) shareAccount(w http.ResponseWriter, r *http.Request) {
	id, err := accountIDFromPath(strings.TrimSuffix(strings.TrimSuffix(r.URL.Path, "/share"), "/"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	account, err := h.repo.GetByID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	account = applyBuiltInDriverDefaults(account)

	payload, err := json.Marshal(accountShareEnvelope{
		Kind:          accountShareKind,
		SchemaVersion: accountShareSchemaVersion,
		ExportedAt:    time.Now().UTC().Format(time.RFC3339),
		Account: accountSharePayload{
			ProviderType:      account.ProviderType,
			AccountName:       account.AccountName,
			SourceIcon:        normalizeAccountSourceIcon(account.SourceIcon),
			AuthMode:          account.AuthMode,
			BaseURL:           account.BaseURL,
			CredentialRef:     account.CredentialRef,
			AccountDriver:     account.AccountDriver,
			UsageDriver:       account.UsageDriver,
			UsageConfigJSON:   account.UsageConfigJSON,
			SupportsResponses: account.NativeResponsesCapable(),
		},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"payload": string(payload)})
}

func (h *AccountsHandler) importSharedAccount(w http.ResponseWriter, r *http.Request) {
	var req importSharedAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	envelope, err := parseSharedAccountPayload(req.Payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	account := applyBuiltInDriverDefaults(accounts.Account{
		ProviderType:      envelope.Account.ProviderType,
		AccountName:       envelope.Account.AccountName,
		SourceIcon:        normalizeAccountSourceIcon(envelope.Account.SourceIcon),
		AuthMode:          envelope.Account.AuthMode,
		BaseURL:           strings.TrimSpace(envelope.Account.BaseURL),
		CredentialRef:     envelope.Account.CredentialRef,
		AccountDriver:     strings.TrimSpace(envelope.Account.AccountDriver),
		UsageDriver:       strings.TrimSpace(envelope.Account.UsageDriver),
		UsageConfigJSON:   strings.TrimSpace(envelope.Account.UsageConfigJSON),
		Status:            accounts.StatusActive,
		Priority:          0,
		IsActive:          false,
		SupportsResponses: envelope.Account.SupportsResponses,
	})

	if err := h.repo.Create(account); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (h *AccountsHandler) updateAccount(w http.ResponseWriter, r *http.Request) {
	id, err := accountIDFromPath(strings.TrimSuffix(r.URL.Path, "/"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	current, err := h.repo.GetByID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	var req updateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.AccountName != "" {
		current.AccountName = req.AccountName
	}
	if req.SourceIcon != "" {
		current.SourceIcon = normalizeAccountSourceIcon(req.SourceIcon)
	}
	if req.BaseURL != "" {
		current.BaseURL = req.BaseURL
	}
	if req.CredentialRef != "" {
		current.CredentialRef = req.CredentialRef
	}
	if req.AccountDriver != nil {
		current.AccountDriver = strings.TrimSpace(*req.AccountDriver)
	}
	if req.UsageDriver != nil {
		current.UsageDriver = strings.TrimSpace(*req.UsageDriver)
	}
	if req.UsageConfigJSON != nil {
		current.UsageConfigJSON = strings.TrimSpace(*req.UsageConfigJSON)
	}
	if req.Status != "" {
		current.Status = req.Status
	}
	if req.Priority != nil {
		current.Priority = *req.Priority
	}
	if req.IsActive != nil {
		current.IsActive = *req.IsActive
		if *req.IsActive {
			if err := h.repo.SetActive(id); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			log.Printf("accounts: active account updated account_id=%d account_name=%q", current.ID, current.AccountName)
			h.publishAccountRoutingStateChanged()
		}
	}
	if req.SupportsResponses != nil {
		current.SupportsResponses = *req.SupportsResponses
	}
	current = applyBuiltInDriverDefaults(current)

	if err := h.repo.Update(current); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *AccountsHandler) publishAccountRoutingStateChanged() {
	if h.stateEvents != nil {
		h.stateEvents.Publish(AccountRoutingStateChangedTopic)
	}
}

func applyBuiltInDriverDefaults(account accounts.Account) accounts.Account {
	if strings.TrimSpace(account.AccountDriver) == "" {
		switch {
		case account.AuthMode == accounts.AuthModeAPIKey:
			account.AccountDriver = "builtin_api_key"
		case account.AuthMode == accounts.AuthModeLocalImport && account.ProviderType == accounts.ProviderOpenAIOfficial:
			account.AccountDriver = "builtin_openai_official_session"
		}
	}
	if strings.TrimSpace(account.UsageDriver) == "" {
		switch {
		case account.ProviderType == accounts.ProviderOpenAIOfficial:
			account.UsageDriver = "builtin_openai_official"
		case strings.Contains(strings.ToLower(strings.TrimSpace(account.BaseURL)), "ppchat.vip"):
			account.UsageDriver = "builtin_ppchat"
		}
	}
	return account
}

func (h *AccountsHandler) testAccount(w http.ResponseWriter, r *http.Request) {
	id, err := accountIDFromPath(strings.TrimSuffix(strings.TrimSuffix(r.URL.Path, "/test"), "/"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	account, err := h.repo.GetByID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	credential, err := resolveCredential(account)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if usesOfficialCodexAdapter(account) {
		if err := ensureOfficialAccountSession(r.Context(), h.client, h.repo, &account); err != nil {
			writeJSON(w, http.StatusOK, accountTestResponse{OK: false, Message: "官方账户会话刷新失败", Details: err.Error()})
			return
		}
		credential, err = resolveCredential(account)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	var reqBody accountChatTestRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if reqBody.Model == "" {
		reqBody.Model = defaultTestModelForAccount(account)
	}
	if reqBody.Input == "" {
		reqBody.Input = "ping"
	}

	writeJSON(w, http.StatusOK, h.runAccountTest(r.Context(), account, credential, reqBody.Model, reqBody.Input))
}

func (h *AccountsHandler) getLuaUsageScript(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/accounts/usage-scripts/"))
	if h.luaScripts == nil {
		http.Error(w, "lua script storage is not configured", http.StatusNotFound)
		return
	}
	content, err := h.luaScripts.Load(key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, luaUsageScriptResponse{Key: key, Content: content})
}

func (h *AccountsHandler) listLuaUsageScripts(w http.ResponseWriter) {
	if h.luaScripts == nil {
		http.Error(w, "lua script storage is not configured", http.StatusNotFound)
		return
	}
	items, err := h.luaScripts.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, luaUsageScriptListResponse{Items: items})
}

func (h *AccountsHandler) putLuaUsageScript(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/accounts/usage-scripts/"))
	if h.luaScripts == nil {
		http.Error(w, "lua script storage is not configured", http.StatusNotFound)
		return
	}
	var req luaUsageScriptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		http.Error(w, "script content is required", http.StatusBadRequest)
		return
	}
	if err := h.luaScripts.Save(key, req.Content); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, luaUsageScriptResponse{Key: key})
}

func (h *AccountsHandler) testLuaUsage(w http.ResponseWriter, r *http.Request) {
	id, err := accountIDFromPath(strings.TrimSuffix(strings.TrimSuffix(r.URL.Path, "/usage-lua-test"), "/"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if h.drivers == nil {
		http.Error(w, "driver registry is not configured", http.StatusInternalServerError)
		return
	}
	account, err := h.repo.GetByID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	var req luaUsageTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	configJSON := strings.TrimSpace(req.UsageConfigJSON)
	if configJSON == "" {
		configJSON = account.UsageConfigJSON
	}
	cfg, err := luadrv.ParseDriverConfig(configJSON)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	account.UsageDriver = "lua"
	account.UsageConfigJSON = configJSON

	accountDriver, err := h.drivers.AccountDriverFor(account)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	credential, err := accountDriver.Resolve(r.Context(), account)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	scriptContent := strings.TrimSpace(req.ScriptContent)
	if scriptContent == "" {
		if key, ok := luadrv.ParseManagedScriptKey(cfg.Script); ok && h.luaScripts != nil {
			scriptContent, err = h.luaScripts.Load(key)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		} else {
			http.Error(w, "script_content is required for non-managed lua test", http.StatusBadRequest)
			return
		}
	}

	if h.luaRuntime == nil {
		h.luaRuntime = luadrv.NewRuntime(h.client, "")
	}
	result, err := h.luaRuntime.ExecuteSource(r.Context(), scriptContent, cfg.Script, account, credential, cfg.Raw)
	if err != nil {
		writeJSON(w, http.StatusOK, accountTestResponse{
			OK:      false,
			Message: "Lua usage 测试失败",
			Details: err.Error(),
		})
		return
	}
	pretty, err := json.MarshalIndent(map[string]any{
		"source":                 result.Source,
		"confidence":             result.Confidence,
		"balance":                result.Limits.Balance,
		"quota_remaining":        result.Limits.QuotaRemaining,
		"rpm_remaining":          result.Limits.RPMRemaining,
		"tpm_remaining":          result.Limits.TPMRemaining,
		"daily_remaining":        result.Limits.DailyRemaining,
		"monthly_remaining":      result.Limits.MonthlyRemaining,
		"primary_used_percent":   result.Limits.PrimaryUsedPercent,
		"secondary_used_percent": result.Limits.SecondaryUsedPercent,
		"primary_resets_at":      result.Limits.PrimaryResetsAt,
		"secondary_resets_at":    result.Limits.SecondaryResetsAt,
		"meta":                   result.Meta,
		"payload":                result.Payload,
	}, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, accountTestResponse{
		OK:      true,
		Message: "Lua usage 测试成功",
		Details: "脚本已返回标准化 usage 结果",
		Content: string(pretty),
	})
}

func (h *AccountsHandler) getPPChatTokenLogs(w http.ResponseWriter, r *http.Request) {
	id, err := accountIDFromPath(strings.TrimSuffix(strings.TrimSuffix(r.URL.Path, "/ppchat-token-logs"), "/"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	account, err := h.repo.GetByID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if !strings.Contains(strings.ToLower(account.BaseURL), "ppchat.vip") {
		http.Error(w, "account is not a ppchat provider", http.StatusBadRequest)
		return
	}

	credential, err := resolveCredential(account)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	credential = strings.TrimSpace(strings.TrimPrefix(credential, "Bearer "))
	if credential == "" {
		http.Error(w, "missing credential token", http.StatusBadRequest)
		return
	}

	page := strings.TrimSpace(r.URL.Query().Get("page"))
	if page == "" {
		page = "1"
	}
	pageSize := strings.TrimSpace(r.URL.Query().Get("page_size"))
	if pageSize == "" {
		pageSize = "10"
	}

	endpoint := (&url.URL{
		Scheme: "https",
		Host:   "his.ppchat.vip",
		Path:   "/api/token-logs",
		RawQuery: url.Values{
			"token_key": []string{credential},
			"page":      []string{page},
			"page_size": []string{pageSize},
		}.Encode(),
	}).String()

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, endpoint, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp, err := h.client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if resp.StatusCode >= 400 {
		http.Error(w, string(body), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (h *AccountsHandler) runAccountTest(ctx context.Context, account accounts.Account, credential string, requestedModel string, input string) accountTestResponse {
	model := strings.TrimSpace(requestedModel)
	if model == "" {
		model = defaultTestModelForAccount(account)
	}

	if account.AuthMode == accounts.AuthModeLocalImport || account.ProviderType == accounts.ProviderOpenAIOfficial {
		return h.runResponsesTest(ctx, account, credential, model, input)
	}

	result := h.runChatCompletionTest(ctx, account, credential, model, input)
	if result.OK || strings.TrimSpace(requestedModel) != "" {
		return result
	}

	fallbackModel, err := h.discoverFallbackModel(ctx, account, credential, model)
	if err != nil || fallbackModel == "" || fallbackModel == model {
		return result
	}

	fallbackResult := h.runChatCompletionTest(ctx, account, credential, fallbackModel, input)
	if fallbackResult.OK {
		fallbackResult.Details = fmt.Sprintf("模型 %s 已返回响应（自动从 %s 切换）", fallbackModel, model)
	}
	return fallbackResult
}

func (h *AccountsHandler) runResponsesTest(ctx context.Context, account accounts.Account, credential string, model string, input string) accountTestResponse {
	if usesOfficialCodexAdapter(account) {
		accountID, err := resolveLocalAccountID(account)
		if err != nil {
			return accountTestResponse{OK: false, Message: "本地凭证缺少账户信息", Details: err.Error()}
		}
		body, err := json.Marshal(map[string]any{
			"model":  model,
			"stream": true,
			"store":  false,
			"input": []map[string]any{
				{
					"role": "user",
					"content": []map[string]any{
						{
							"type": "input_text",
							"text": input,
						},
					},
				},
			},
			"instructions": effectiveCodexInstructions(""),
		})
		if err != nil {
			return accountTestResponse{OK: false, Message: "构造测试请求失败", Details: err.Error()}
		}
		adapter := providercodex.NewAdapter(resolveAccountBaseURL(account))
		req, err := adapter.BuildResponsesRequest(ctx, credential, accountID, body, true)
		if err != nil {
			return accountTestResponse{OK: false, Message: "构造测试请求失败", Details: err.Error()}
		}
		resp, err := h.client.Do(req)
		if err != nil {
			return accountTestResponse{OK: false, Message: "请求上游失败", Details: err.Error()}
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			raw, _ := io.ReadAll(resp.Body)
			return accountTestResponse{
				OK:      false,
				Message: "上游测试失败",
				Details: buildUpstreamErrorDetails(resp.Status, raw),
				Content: strings.TrimSpace(string(raw)),
			}
		}
		collector := newResponsesUsageCollector(account.ID)
		var builder strings.Builder
		if err := consumeResponsesStream(resp.Body, func(delta string) error {
			builder.WriteString(delta)
			return nil
		}, collector.Observe); err != nil {
			return accountTestResponse{OK: false, Message: "读取上游流失败", Details: err.Error()}
		}
		collector.Save(h.usage)
		return accountTestResponse{
			OK:      true,
			Message: "OpenAI responses 测试成功",
			Details: "模型 " + model + " 已返回响应",
			Content: builder.String(),
		}
	}

	body, err := json.Marshal(map[string]any{
		"model": model,
		"input": input,
	})
	if err != nil {
		return accountTestResponse{OK: false, Message: "构造测试请求失败", Details: err.Error()}
	}

	req, err := providers.NewJSONRequest(ctx, http.MethodPost, strings.TrimRight(resolveAccountBaseURL(account), "/")+"/responses", credential, body)
	if err != nil {
		return accountTestResponse{OK: false, Message: "构造测试请求失败", Details: err.Error()}
	}
	return h.executeUpstreamTest(req, model, parseResponsesContent, "OpenAI responses 测试成功")
}

func (h *AccountsHandler) runChatCompletionTest(ctx context.Context, account accounts.Account, credential string, model string, input string) accountTestResponse {
	body, err := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": input},
		},
		"stream": false,
	})
	if err != nil {
		return accountTestResponse{OK: false, Message: "构造测试请求失败", Details: err.Error()}
	}

	adapter := provideropenai.NewAdapter(resolveAccountBaseURL(account))
	req, err := adapter.BuildRequest(ctx, providers.Request{
		Path:   "/chat/completions",
		Method: http.MethodPost,
		APIKey: credential,
		Body:   body,
	})
	if err != nil {
		return accountTestResponse{OK: false, Message: "构造测试请求失败", Details: err.Error()}
	}

	return h.executeUpstreamTest(req, model, parseChatCompletionsContent, "远端连通性测试成功")
}

func (h *AccountsHandler) executeUpstreamTest(req *http.Request, model string, parser func([]byte) string, successMessage string) accountTestResponse {
	resp, err := h.client.Do(req)
	if err != nil {
		return accountTestResponse{OK: false, Message: "请求上游失败", Details: err.Error()}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return accountTestResponse{OK: false, Message: "读取上游响应失败", Details: err.Error()}
	}

	if resp.StatusCode >= 400 {
		return accountTestResponse{
			OK:      false,
			Message: "上游测试失败",
			Details: buildUpstreamErrorDetails(resp.Status, raw),
			Content: strings.TrimSpace(string(raw)),
		}
	}

	return accountTestResponse{
		OK:      true,
		Message: successMessage,
		Details: "模型 " + model + " 已返回响应",
		Content: parser(raw),
	}
}

func (h *AccountsHandler) discoverFallbackModel(ctx context.Context, account accounts.Account, credential string, currentModel string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(resolveAccountBaseURL(account), "/")+"/models", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+credential)

	resp, err := h.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("upstream models request failed: %s", resp.Status)
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}

	available := make(map[string]struct{}, len(payload.Data))
	for _, model := range payload.Data {
		if strings.TrimSpace(model.ID) != "" {
			available[model.ID] = struct{}{}
		}
	}

	for _, candidate := range preferredTestModels(account) {
		if candidate == currentModel {
			continue
		}
		if _, ok := available[candidate]; ok {
			return candidate, nil
		}
	}
	for _, model := range payload.Data {
		if model.ID != "" && model.ID != currentModel {
			return model.ID, nil
		}
	}
	return "", nil
}

func defaultTestModelForAccount(account accounts.Account) string {
	if account.AuthMode == accounts.AuthModeLocalImport || account.ProviderType == accounts.ProviderOpenAIOfficial {
		return "gpt-5.4"
	}
	return "gpt-5.4"
}

func preferredTestModels(account accounts.Account) []string {
	if account.AuthMode == accounts.AuthModeLocalImport || account.ProviderType == accounts.ProviderOpenAIOfficial {
		return []string{"gpt-5.4", "gpt-5.2-codex", "gpt-5.1-codex-max", "gpt-4.1"}
	}
	return []string{"gpt-5.4", "gpt-5.2-codex", "gpt-5.1-codex-max", "gpt-4.1"}
}

func parseResponsesContent(raw []byte) string {
	var payload struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return strings.TrimSpace(string(raw))
	}
	if strings.TrimSpace(payload.OutputText) != "" {
		return payload.OutputText
	}
	for _, item := range payload.Output {
		for _, content := range item.Content {
			if strings.TrimSpace(content.Text) != "" {
				return content.Text
			}
		}
	}
	return strings.TrimSpace(string(raw))
}

func parseChatCompletionsContent(raw []byte) string {
	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return strings.TrimSpace(string(raw))
	}
	if len(payload.Choices) > 0 {
		return payload.Choices[0].Message.Content
	}
	return strings.TrimSpace(string(raw))
}

func buildUpstreamErrorDetails(status string, raw []byte) string {
	body := strings.TrimSpace(string(raw))
	if body == "" {
		return "上游返回错误：" + status
	}
	return "上游返回错误：" + status + "\n" + body
}

func buildUpstreamStatusError(statusCode int, raw []byte) error {
	body := compactErrorText(strings.TrimSpace(string(raw)), 512)
	if providers.LooksLikeInsufficientQuotaMessage(body) {
		if body == "" {
			return providers.ErrInsufficientQuota
		}
		return fmt.Errorf("http status %d: %s: %w", statusCode, body, providers.ErrInsufficientQuota)
	}
	if body == "" {
		return providers.HTTPError{StatusCode: statusCode}
	}
	return fmt.Errorf("http status %d: %s: %w", statusCode, body, providers.HTTPError{StatusCode: statusCode})
}

func classifyThinResponseStatus(account accounts.Account, resp *http.Response, raw []byte) string {
	if resp.StatusCode == http.StatusTooManyRequests {
		if looksLikeOfficialUsageLimit(account, raw) {
			return "usage_limited"
		}
		return "rate_limited"
	}
	if providers.LooksLikeInsufficientQuotaMessage(string(raw)) {
		return "capacity_failed"
	}
	return runStatusForErrorClass(classifyRunError(buildUpstreamStatusError(resp.StatusCode, raw)))
}

func looksLikeOfficialUsageLimit(account accounts.Account, raw []byte) bool {
	if !usesOfficialCodexAdapter(account) {
		return false
	}
	body := strings.ToLower(strings.Join(strings.Fields(string(raw)), " "))
	if body == "" {
		return false
	}
	for _, marker := range []string{
		"usage limit",
		"purchase more credits",
		"upgrade to pro",
		"upgrade to plus",
		"continue using codex",
		"send a request to your admin",
	} {
		if strings.Contains(body, marker) {
			return true
		}
	}
	return false
}

func (h *AccountsHandler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	id, err := accountIDFromPath(strings.TrimSuffix(r.URL.Path, "/"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.repo.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AccountsHandler) duplicateAccount(w http.ResponseWriter, r *http.Request) {
	id, err := accountIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	source, err := h.repo.GetByID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	accountList, err := h.repo.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	duplicate := source
	duplicate.ID = 0
	duplicate.AccountName = nextDuplicatedAccountName(source.AccountName, accountList)
	duplicate.IsActive = false
	duplicate.CooldownUntil = nil

	if err := h.repo.Create(duplicate); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated)
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
	trimmed = strings.TrimSuffix(trimmed, "/duplicate")
	trimmed = strings.TrimSuffix(trimmed, "/share")
	trimmed = strings.TrimSuffix(trimmed, "/test")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return 0, errors.New("missing account id")
	}
	return strconv.ParseInt(trimmed, 10, 64)
}

func parseSharedAccountPayload(raw string) (accountShareEnvelope, error) {
	var envelope accountShareEnvelope
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &envelope); err != nil {
		return accountShareEnvelope{}, fmt.Errorf("invalid shared account payload: %w", err)
	}
	if envelope.Kind != accountShareKind {
		return accountShareEnvelope{}, fmt.Errorf("invalid shared account kind: %q", envelope.Kind)
	}
	if envelope.SchemaVersion != accountShareSchemaVersion {
		return accountShareEnvelope{}, fmt.Errorf("unsupported shared account schema version: %d", envelope.SchemaVersion)
	}
	if err := validateSharedAccount(envelope.Account); err != nil {
		return accountShareEnvelope{}, err
	}
	return envelope, nil
}

func validateSharedAccount(payload accountSharePayload) error {
	if !isSupportedProviderType(payload.ProviderType) {
		return fmt.Errorf("unsupported provider_type: %q", payload.ProviderType)
	}
	if strings.TrimSpace(payload.AccountName) == "" {
		return errors.New("missing account_name")
	}
	if !isSupportedAuthMode(payload.AuthMode) {
		return fmt.Errorf("unsupported auth_mode: %q", payload.AuthMode)
	}
	if strings.TrimSpace(payload.CredentialRef) == "" {
		return errors.New("missing credential_ref")
	}
	if err := validateAbsoluteBaseURL(payload.BaseURL); err != nil {
		return err
	}
	if err := validateUsageConfigJSON(payload.UsageConfigJSON); err != nil {
		return err
	}
	return nil
}

func isSupportedProviderType(value accounts.ProviderType) bool {
	switch value {
	case accounts.ProviderOpenAIOfficial, accounts.ProviderOpenAICompatible:
		return true
	default:
		return false
	}
}

func isSupportedAuthMode(value accounts.AuthMode) bool {
	switch value {
	case accounts.AuthModeOAuth, accounts.AuthModeAPIKey, accounts.AuthModeLocalImport:
		return true
	default:
		return false
	}
}

func validateAbsoluteBaseURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid base_url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("invalid base_url scheme: %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return errors.New("invalid base_url: missing host")
	}
	return nil
}

func validateUsageConfigJSON(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	var payload any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return fmt.Errorf("invalid usage_config_json: %w", err)
	}
	return nil
}

func nextDuplicatedAccountName(baseName string, accountList []accounts.Account) string {
	baseName = strings.TrimSpace(baseName)
	if baseName == "" {
		baseName = "account"
	}

	used := make(map[string]struct{}, len(accountList))
	for _, account := range accountList {
		used[strings.TrimSpace(account.AccountName)] = struct{}{}
	}

	for index := 1; ; index += 1 {
		candidate := fmt.Sprintf("%s %d", baseName, index)
		if _, exists := used[candidate]; !exists {
			return candidate
		}
	}
}

func countPathSegments(path string) int {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return 0
	}
	return len(strings.Split(trimmed, "/"))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func decodeLocalImportRequest(r *http.Request) (string, []byte, error) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			return "", nil, err
		}
		accountName := r.FormValue("account_name")
		file, _, err := r.FormFile("auth_file")
		if err != nil {
			return "", nil, err
		}
		defer file.Close()
		raw, err := io.ReadAll(file)
		if err != nil {
			return "", nil, err
		}
		if _, err := auth.LoadLocalAuthFileContent(raw); err != nil {
			return "", nil, err
		}
		return accountName, raw, nil
	}

	var req importLocalAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return "", nil, err
	}
	raw := []byte(req.Content)
	if len(bytes.TrimSpace(raw)) == 0 && strings.TrimSpace(req.Path) != "" {
		_, fileRaw, err := auth.LoadLocalAuthFile(req.Path)
		if err != nil {
			return "", nil, err
		}
		raw = fileRaw
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", nil, errors.New("missing auth.json content")
	}
	if _, err := auth.LoadLocalAuthFileContent(raw); err != nil {
		return "", nil, err
	}
	return req.AccountName, raw, nil
}

func normalizeAccountSourceIcon(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "claude_code":
		return "claude_code"
	case "ppchat":
		return "ppchat"
	default:
		return "openai"
	}
}

func resolveAccountBaseURL(account accounts.Account) string {
	if usesOfficialCodexAdapter(account) {
		baseURL := strings.TrimSpace(account.BaseURL)
		if baseURL == "" || baseURL == officialOpenAIBaseURL {
			return officialCodexBaseURL
		}
		return baseURL
	}
	if strings.TrimSpace(account.BaseURL) != "" {
		return account.BaseURL
	}
	return account.BaseURL
}
