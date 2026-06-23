package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/serverauth"
	"github.com/gcssloop/codex-router/backend/internal/serverusers"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

func TestServerMeHandlerReturnsOwnUsageWithoutAccountPool(t *testing.T) {
	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	userRepo := serverusers.NewSQLiteRepository(store.DB())
	created, err := userRepo.Create("alice")
	if err != nil {
		t.Fatalf("Create user returned error: %v", err)
	}
	userID := created.User.ID
	usageRepo := usage.NewSQLiteRepository(store.DB())
	if err := usageRepo.SaveEvent(usage.Event{
		AccountID:     1,
		ServerUserID:  &userID,
		ProviderType:  "openai-compatible",
		RequestKind:   "responses",
		Model:         "gpt-test",
		Status:        "completed",
		InputTokens:   10,
		OutputTokens:  20,
		TotalTokens:   30,
		EstimatedCost: 0.01,
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveEvent returned error: %v", err)
	}

	handler := NewServerMeHandler(userRepo)
	session := serverauth.Session{Authenticated: true, Role: serverusers.RoleUser, UserID: created.User.ID, Username: "alice"}

	meReq := httptest.NewRequest(http.MethodGet, "/me", nil)
	meReq = meReq.WithContext(serverauth.ContextWithSession(meReq.Context(), session))
	meRec := httptest.NewRecorder()
	handler.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("GET /me status = %d, want %d; body=%s", meRec.Code, http.StatusOK, meRec.Body.String())
	}
	var me struct {
		User         serverusers.User `json:"user"`
		RequestCount int64            `json:"request_count"`
		TotalTokens  int64            `json:"total_tokens"`
	}
	if err := json.Unmarshal(meRec.Body.Bytes(), &me); err != nil {
		t.Fatalf("unmarshal me: %v", err)
	}
	if me.User.ID != created.User.ID || me.RequestCount != 1 || me.TotalTokens != 30 {
		t.Fatalf("me = %+v, want own usage", me)
	}

	accountsReq := httptest.NewRequest(http.MethodGet, "/me/accounts", nil)
	accountsReq = accountsReq.WithContext(serverauth.ContextWithSession(accountsReq.Context(), session))
	accountsRec := httptest.NewRecorder()
	handler.ServeHTTP(accountsRec, accountsReq)
	if accountsRec.Code != http.StatusNotFound {
		t.Fatalf("GET /me/accounts status = %d, want %d; body=%s", accountsRec.Code, http.StatusNotFound, accountsRec.Body.String())
	}

	stateReq := httptest.NewRequest(http.MethodPut, "/me/accounts/1/state", nil)
	stateReq = stateReq.WithContext(serverauth.ContextWithSession(stateReq.Context(), session))
	stateRec := httptest.NewRecorder()
	handler.ServeHTTP(stateRec, stateReq)
	if stateRec.Code != http.StatusNotFound {
		t.Fatalf("PUT /me/accounts/1/state status = %d, want %d", stateRec.Code, http.StatusNotFound)
	}
}

func TestServerMeHandlerReturnsSanitizedUpstreamsAndUpdatesRoute(t *testing.T) {
	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	userRepo := serverusers.NewSQLiteRepository(store.DB())
	created, err := userRepo.Create("alice")
	if err != nil {
		t.Fatalf("Create user returned error: %v", err)
	}
	accountRepo := accounts.NewSQLiteRepository(store.DB())
	for _, account := range []accounts.Account{
		{
			ProviderType:      accounts.ProviderOpenAICompatible,
			AccountName:       "account-a",
			SourceIcon:        "openai",
			AuthMode:          accounts.AuthModeAPIKey,
			BaseURL:           "https://upstream-a.example/v1",
			CredentialRef:     "sk-secret-a",
			UsageConfigJSON:   `{"secret":"do-not-leak"}`,
			Status:            accounts.StatusActive,
			Priority:          100,
			SupportsResponses: true,
		},
		{
			ProviderType:      accounts.ProviderOpenAICompatible,
			AccountName:       "account-b",
			SourceIcon:        "ppchat",
			AuthMode:          accounts.AuthModeAPIKey,
			BaseURL:           "https://upstream-b.example/v1",
			CredentialRef:     "sk-secret-b",
			Status:            accounts.StatusDisabled,
			Priority:          10,
			SupportsResponses: true,
		},
	} {
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account returned error: %v", err)
		}
	}
	usageRepo := usage.NewSQLiteRepository(store.DB())
	if err := usageRepo.Save(usage.Snapshot{
		AccountID:        1,
		Balance:          10,
		QuotaRemaining:   1000,
		RPMRemaining:     20,
		TPMRemaining:     3000,
		HealthScore:      0.9,
		LastTotalTokens:  120,
		LastInputTokens:  40,
		LastOutputTokens: 80,
		CheckedAt:        time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Save usage returned error: %v", err)
	}
	if err := usageRepo.Save(usage.Snapshot{
		AccountID:      2,
		QuotaRemaining: -2,
		HealthScore:    1,
		CheckedAt:      time.Now().UTC(),
		ProviderSnapshotJSON: `{
			"payload": {
				"data": {
					"token_info": {
						"today_used_quota": 22,
						"today_added_quota": 20,
						"remain_quota_display": -2
					}
				}
			}
		}`,
	}); err != nil {
		t.Fatalf("Save ppchat usage returned error: %v", err)
	}

	handler := NewServerMeHandler(userRepo, WithServerMeAccounts(accountRepo), WithServerMeUsage(usageRepo))
	session := serverauth.Session{Authenticated: true, Role: serverusers.RoleUser, UserID: created.User.ID, Username: "alice"}

	upstreamsReq := httptest.NewRequest(http.MethodGet, "/me/upstreams", nil)
	upstreamsReq = upstreamsReq.WithContext(serverauth.ContextWithSession(upstreamsReq.Context(), session))
	upstreamsRec := httptest.NewRecorder()
	handler.ServeHTTP(upstreamsRec, upstreamsReq)
	if upstreamsRec.Code != http.StatusOK {
		t.Fatalf("GET /me/upstreams status = %d, want %d; body=%s", upstreamsRec.Code, http.StatusOK, upstreamsRec.Body.String())
	}
	body := upstreamsRec.Body.String()
	for _, leaked := range []string{"sk-secret-a", "sk-secret-b", "credential_ref", "usage_config_json", "do-not-leak"} {
		if strings.Contains(body, leaked) {
			t.Fatalf("GET /me/upstreams body leaked %q: %s", leaked, body)
		}
	}
	var upstreams struct {
		TotalAccounts     int   `json:"total_accounts"`
		AvailableAccounts int   `json:"available_accounts"`
		CurrentAccountID  int64 `json:"current_account_id"`
		RouteLocked       bool  `json:"route_locked"`
		Accounts          []struct {
			ID                        int64   `json:"id"`
			AccountName               string  `json:"account_name"`
			BaseURL                   string  `json:"base_url"`
			Status                    string  `json:"status"`
			Available                 bool    `json:"available"`
			Current                   bool    `json:"current"`
			Balance                   float64 `json:"balance"`
			QuotaRemaining            float64 `json:"quota_remaining"`
			LastTotalTokens           float64 `json:"last_total_tokens"`
			PPChatTodayUsedQuota      float64 `json:"ppchat_today_used_quota"`
			PPChatTodayAddedQuota     float64 `json:"ppchat_today_added_quota"`
			PPChatTodayRemainingQuota float64 `json:"ppchat_today_remaining_quota"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal(upstreamsRec.Body.Bytes(), &upstreams); err != nil {
		t.Fatalf("unmarshal upstreams: %v", err)
	}
	if upstreams.TotalAccounts != 2 || upstreams.AvailableAccounts != 1 || upstreams.CurrentAccountID != 1 {
		t.Fatalf("upstreams summary = %+v, want total 2 available 1 current 1", upstreams)
	}
	if len(upstreams.Accounts) != 2 || !upstreams.Accounts[0].Current || upstreams.Accounts[0].Balance != 10 || upstreams.Accounts[0].LastTotalTokens != 120 {
		t.Fatalf("upstream accounts = %+v, want first account current with usage", upstreams.Accounts)
	}
	if upstreams.Accounts[1].PPChatTodayUsedQuota != 22 || upstreams.Accounts[1].PPChatTodayAddedQuota != 20 || upstreams.Accounts[1].PPChatTodayRemainingQuota != -2 {
		t.Fatalf("ppchat upstream usage = %+v, want cached ppchat quota fields", upstreams.Accounts[1])
	}

	routeReq := httptest.NewRequest(http.MethodPut, "/me/route", bytes.NewBufferString(`{"account_id":1,"locked":true}`))
	routeReq.Header.Set("Content-Type", "application/json")
	routeReq = routeReq.WithContext(serverauth.ContextWithSession(routeReq.Context(), session))
	routeRec := httptest.NewRecorder()
	handler.ServeHTTP(routeRec, routeReq)
	if routeRec.Code != http.StatusOK {
		t.Fatalf("PUT /me/route status = %d, want %d; body=%s", routeRec.Code, http.StatusOK, routeRec.Body.String())
	}
	updated, err := userRepo.Get(created.User.ID)
	if err != nil {
		t.Fatalf("Get updated user returned error: %v", err)
	}
	if updated.PreferredAccountID == nil || *updated.PreferredAccountID != 1 || !updated.RouteLocked {
		t.Fatalf("updated route = account:%v locked:%v, want account 1 locked", updated.PreferredAccountID, updated.RouteLocked)
	}
}

func TestServerMeHandlerLocksUpstreamAccountAndAllowsManualRoute(t *testing.T) {
	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	userRepo := serverusers.NewSQLiteRepository(store.DB())
	created, err := userRepo.Create("alice")
	if err != nil {
		t.Fatalf("Create user returned error: %v", err)
	}
	accountRepo := accounts.NewSQLiteRepository(store.DB())
	if err := accountRepo.Create(accounts.Account{
		ProviderType:      accounts.ProviderOpenAICompatible,
		AccountName:       "manual-locked",
		SourceIcon:        "openai",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           "https://locked.example/v1",
		CredentialRef:     "sk-locked",
		Status:            accounts.StatusActive,
		Priority:          100,
		SupportsResponses: true,
	}); err != nil {
		t.Fatalf("Create account returned error: %v", err)
	}

	handler := NewServerMeHandler(userRepo, WithServerMeAccounts(accountRepo))
	session := serverauth.Session{Authenticated: true, Role: serverusers.RoleUser, UserID: created.User.ID, Username: "alice"}

	lockReq := httptest.NewRequest(http.MethodPut, "/me/upstreams/1/lock", bytes.NewBufferString(`{"locked":true}`))
	lockReq.Header.Set("Content-Type", "application/json")
	lockReq = lockReq.WithContext(serverauth.ContextWithSession(lockReq.Context(), session))
	lockRec := httptest.NewRecorder()
	handler.ServeHTTP(lockRec, lockReq)
	if lockRec.Code != http.StatusOK {
		t.Fatalf("PUT /me/upstreams/1/lock status = %d, want %d; body=%s", lockRec.Code, http.StatusOK, lockRec.Body.String())
	}

	upstreamsReq := httptest.NewRequest(http.MethodGet, "/me/upstreams", nil)
	upstreamsReq = upstreamsReq.WithContext(serverauth.ContextWithSession(upstreamsReq.Context(), session))
	upstreamsRec := httptest.NewRecorder()
	handler.ServeHTTP(upstreamsRec, upstreamsReq)
	if upstreamsRec.Code != http.StatusOK {
		t.Fatalf("GET /me/upstreams status = %d, want %d; body=%s", upstreamsRec.Code, http.StatusOK, upstreamsRec.Body.String())
	}
	var upstreams struct {
		AvailableAccounts int `json:"available_accounts"`
		Accounts          []struct {
			ID            int64 `json:"id"`
			Available     bool  `json:"available"`
			AccountLocked bool  `json:"account_locked"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal(upstreamsRec.Body.Bytes(), &upstreams); err != nil {
		t.Fatalf("unmarshal upstreams: %v", err)
	}
	if upstreams.AvailableAccounts != 0 || len(upstreams.Accounts) != 1 || upstreams.Accounts[0].Available || !upstreams.Accounts[0].AccountLocked {
		t.Fatalf("upstreams = %+v, want locked unavailable account", upstreams)
	}

	routeReq := httptest.NewRequest(http.MethodPut, "/me/route", bytes.NewBufferString(`{"account_id":1,"locked":false}`))
	routeReq.Header.Set("Content-Type", "application/json")
	routeReq = routeReq.WithContext(serverauth.ContextWithSession(routeReq.Context(), session))
	routeRec := httptest.NewRecorder()
	handler.ServeHTTP(routeRec, routeReq)
	if routeRec.Code != http.StatusOK {
		t.Fatalf("PUT /me/route locked upstream status = %d, want %d; body=%s", routeRec.Code, http.StatusOK, routeRec.Body.String())
	}
	updated, err := userRepo.Get(created.User.ID)
	if err != nil {
		t.Fatalf("Get updated user returned error: %v", err)
	}
	if updated.PreferredAccountID == nil || *updated.PreferredAccountID != 1 || updated.RouteLocked {
		t.Fatalf("updated route = account:%v locked:%v, want unlocked manual account 1", updated.PreferredAccountID, updated.RouteLocked)
	}
}
