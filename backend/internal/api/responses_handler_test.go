package api_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/api"
	"github.com/gcssloop/codex-router/backend/internal/conversations"
	"github.com/gcssloop/codex-router/backend/internal/settings"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()

	var count int
	query := "SELECT COUNT(*) FROM " + table
	if err := db.QueryRow(query).Scan(&count); err != nil {
		t.Fatalf("count %s returned error: %v", table, err)
	}
	return count
}

func TestResponsesHandlerThinModeThirdPartyResponsesPassthrough(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %q, want /v1/responses", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-third-party" {
			t.Fatalf("authorization = %q, want Bearer sk-third-party", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_tp_1","object":"response","status":"completed","output_text":"tp-pong"}`)
	}))
	defer upstream.Close()

	handler := newResponsesHandlerTestHandler(t, accounts.Account{
		ProviderType:      accounts.ProviderOpenAICompatible,
		AccountName:       "team3",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           upstream.URL + "/v1",
		CredentialRef:     "sk-third-party",
		Status:            accounts.StatusActive,
		Priority:          100,
		SupportsResponses: true,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"output_text":"tp-pong"`) {
		t.Fatalf("body = %s, want third-party output", rec.Body.String())
	}
}

func TestResponsesHandlerThinModeRecordsUsageEventWithoutAuditRows(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id":"resp_tp_usage_1",
			"object":"response",
			"status":"completed",
			"output_text":"tp-pong",
			"usage":{"input_tokens":1200,"output_tokens":300,"total_tokens":1500}
		}`)
	}))
	defer upstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	if err := accountRepo.Create(accounts.Account{
		ProviderType:      accounts.ProviderOpenAICompatible,
		AccountName:       "team3",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           upstream.URL + "/v1",
		CredentialRef:     "sk-third-party",
		Status:            accounts.StatusActive,
		Priority:          100,
		SupportsResponses: true,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	if err := usageRepo.Save(usage.Snapshot{
		AccountID:      1,
		Balance:        100,
		QuotaRemaining: 100000,
		RPMRemaining:   100,
		TPMRemaining:   100000,
		HealthScore:    0.9,
	}); err != nil {
		t.Fatalf("Save(snapshot) returned error: %v", err)
	}

	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()))

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if count := countRows(t, store.DB(), "usage_events"); count != 1 {
		t.Fatalf("usage_events row count = %d, want 1", count)
	}
	for _, table := range []string{"conversations", "messages", "runs"} {
		if count := countRows(t, store.DB(), table); count != 0 {
			t.Fatalf("%s row count = %d, want 0", table, count)
		}
	}
}

func TestResponsesHandlerThinModeRejectsUnsupportedActiveAccount(t *testing.T) {
	t.Parallel()

	handler := newResponsesHandlerTestHandler(t, accounts.Account{
		ProviderType:      accounts.ProviderOpenAICompatible,
		AccountName:       "legacy",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           "https://example.invalid/v1",
		CredentialRef:     "sk-third-party",
		Status:            accounts.StatusActive,
		Priority:          100,
		SupportsResponses: false,
		IsActive:          true,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "support /responses") {
		t.Fatalf("body = %s, want explicit capability error", rec.Body.String())
	}
}

func TestResponsesHandlerThinModeUsesExplicitFailoverQueueWhenEnabled(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk-queued" {
			t.Fatalf("authorization = %q, want Bearer sk-queued", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_tp_q","object":"response","status":"completed","output_text":"queued-first"}`)
	}))
	defer upstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	for _, item := range []accounts.Account{
		{
			ProviderType:      accounts.ProviderOpenAICompatible,
			AccountName:       "active-unsupported",
			AuthMode:          accounts.AuthModeAPIKey,
			BaseURL:           upstream.URL + "/v1",
			CredentialRef:     "sk-unsupported",
			Status:            accounts.StatusActive,
			Priority:          100,
			SupportsResponses: false,
			IsActive:          true,
		},
		{
			ProviderType:      accounts.ProviderOpenAICompatible,
			AccountName:       "queued-supported",
			AuthMode:          accounts.AuthModeAPIKey,
			BaseURL:           upstream.URL + "/v1",
			CredentialRef:     "sk-queued",
			Status:            accounts.StatusActive,
			Priority:          10,
			SupportsResponses: true,
		},
	} {
		if err := accountRepo.Create(item); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	for _, snapshot := range []usage.Snapshot{
		{AccountID: 1, Balance: 100, QuotaRemaining: 100000, RPMRemaining: 100, TPMRemaining: 100000, HealthScore: 0.95},
		{AccountID: 2, Balance: 100, QuotaRemaining: 100000, RPMRemaining: 100, TPMRemaining: 100000, HealthScore: 0.5},
	} {
		if err := usageRepo.Save(snapshot); err != nil {
			t.Fatalf("Save returned error: %v", err)
		}
	}

	settingsRepo := settings.NewSQLiteRepository(store.DB())
	current := settings.DefaultAppSettings()
	current.AutoFailoverEnabled = true
	if err := settingsRepo.SaveAppSettings(current); err != nil {
		t.Fatalf("SaveAppSettings returned error: %v", err)
	}
	if err := settingsRepo.SaveFailoverQueue([]int64{2, 1}); err != nil {
		t.Fatalf("SaveFailoverQueue returned error: %v", err)
	}

	handler := api.NewResponsesHandler(
		accountRepo,
		usageRepo,
		conversations.NewSQLiteRepository(store.DB()),
		api.WithResponsesSettings(settingsRepo),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"output_text":"queued-first"`) {
		t.Fatalf("body = %s, want queue-selected output", rec.Body.String())
	}
}

func TestResponsesHandlerThinModeDisablesSyntheticEndpoints(t *testing.T) {
	t.Parallel()

	handler := newResponsesHandlerTestHandler(t, accounts.Account{
		ProviderType:      accounts.ProviderOpenAICompatible,
		AccountName:       "team3",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           "https://example.invalid/v1",
		CredentialRef:     "sk-third-party",
		Status:            accounts.StatusActive,
		Priority:          100,
		SupportsResponses: true,
	})

	tests := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/v1/responses/resp_1"},
		{method: http.MethodGet, path: "/v1/responses/resp_1/input_items"},
		{method: http.MethodPost, path: "/v1/responses/resp_1/cancel"},
		{method: http.MethodDelete, path: "/v1/responses/resp_1"},
		{method: http.MethodPost, path: "/v1/responses/input_tokens"},
		{method: http.MethodPost, path: "/v1/responses/compact"},
	}

	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
			}
		})
	}
}

func TestResponsesHandlerThinModeRetriesOfficialEOFOnce(t *testing.T) {
	t.Parallel()

	attempts := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("http.ResponseWriter does not implement Hijacker")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatalf("Hijack returned error: %v", err)
			}
			_ = conn.Close()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_official_1","object":"response","status":"completed","output_text":"official-after-retry"}`)
	}))
	defer upstream.Close()

	handler := newResponsesHandlerTestHandler(t, accounts.Account{
		ProviderType: accounts.ProviderOpenAIOfficial,
		AccountName:  "official",
		AuthMode:     accounts.AuthModeLocalImport,
		BaseURL:      upstream.URL + "/backend-api/codex",
		CredentialRef: `{
			"auth_mode":"chatgpt",
			"tokens":{"access_token":"token-1","account_id":"acct-1"}
		}`,
		Status:            accounts.StatusActive,
		Priority:          100,
		SupportsResponses: true,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if !strings.Contains(rec.Body.String(), `"output_text":"official-after-retry"`) {
		t.Fatalf("body = %s, want retried output", rec.Body.String())
	}
}

func newResponsesHandlerTestHandler(t *testing.T, account accounts.Account) http.Handler {
	t.Helper()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	if err := accountRepo.Create(account); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	usageRepo := usage.NewSQLiteRepository(store.DB())
	if err := usageRepo.Save(usage.Snapshot{
		AccountID:      1,
		Balance:        100,
		QuotaRemaining: 100000,
		RPMRemaining:   100,
		TPMRemaining:   100000,
		HealthScore:    0.9,
	}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	return api.NewResponsesHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()))
}

func TestResponsesHandlerThinModePassesThroughPreviousResponseID(t *testing.T) {
	t.Parallel()

	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Fatalf("Decode returned error: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_tp_2","object":"response","status":"completed","output_text":"next"}`)
	}))
	defer upstream.Close()

	handler := newResponsesHandlerTestHandler(t, accounts.Account{
		ProviderType:      accounts.ProviderOpenAICompatible,
		AccountName:       "team3",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           upstream.URL + "/v1",
		CredentialRef:     "sk-third-party",
		Status:            accounts.StatusActive,
		Priority:          100,
		SupportsResponses: true,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"next","previous_response_id":"resp_prev_1"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got, _ := upstreamBody["previous_response_id"].(string); got != "resp_prev_1" {
		t.Fatalf("previous_response_id = %q, want resp_prev_1", got)
	}
}

func TestResponsesHandlerThinModeMarksOfficialUsageLimitedFrom429Body(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"You've hit your usage limit. Visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Mar 19th, 2026 8:37 AM."}}`)
	}))
	defer upstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	if err := accountRepo.Create(accounts.Account{
		ProviderType: accounts.ProviderOpenAIOfficial,
		AccountName:  "official-usage-limit",
		AuthMode:     accounts.AuthModeLocalImport,
		BaseURL:      upstream.URL + "/backend-api/codex",
		CredentialRef: `{
			"auth_mode":"chatgpt",
			"tokens":{"access_token":"token-1","account_id":"acct-1"}
		}`,
		Status:            accounts.StatusActive,
		Priority:          100,
		SupportsResponses: true,
		IsActive:          true,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	if err := usageRepo.Save(usage.Snapshot{
		AccountID:      1,
		Balance:        5.39,
		QuotaRemaining: 100000,
		RPMRemaining:   85,
		TPMRemaining:   40,
		HealthScore:    0.9,
		CheckedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()))

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusTooManyRequests, rec.Body.String())
	}

	events, err := usageRepo.ListRecentEvents(usage.EventFilter{Limit: 5})
	if err != nil {
		t.Fatalf("ListRecentEvents returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	if events[0].Status != "usage_limited" {
		t.Fatalf("event status = %q, want usage_limited", events[0].Status)
	}
}

func TestResponsesHandlerThinModeFailsOverAfterOfficialUsageLimit(t *testing.T) {
	t.Parallel()

	primaryCalls := 0
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryCalls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"You've hit your usage limit. Upgrade to Pro or try again later."}}`)
	}))
	defer primary.Close()

	secondaryCalls := 0
	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondaryCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_fallback_1","object":"response","status":"completed","output_text":"fallback-success"}`)
	}))
	defer secondary.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	for _, account := range []accounts.Account{
		{
			ProviderType: accounts.ProviderOpenAIOfficial,
			AccountName:  "official-primary",
			AuthMode:     accounts.AuthModeLocalImport,
			BaseURL:      primary.URL + "/backend-api/codex",
			CredentialRef: `{
				"auth_mode":"chatgpt",
				"tokens":{"access_token":"token-primary","account_id":"acct-primary"}
			}`,
			Status:            accounts.StatusActive,
			Priority:          100,
			SupportsResponses: true,
			IsActive:          true,
		},
		{
			ProviderType:      accounts.ProviderOpenAICompatible,
			AccountName:       "fallback-third-party",
			AuthMode:          accounts.AuthModeAPIKey,
			BaseURL:           secondary.URL + "/v1",
			CredentialRef:     "sk-fallback",
			Status:            accounts.StatusActive,
			Priority:          90,
			SupportsResponses: true,
		},
	} {
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	for _, snapshot := range []usage.Snapshot{
		{
			AccountID:            1,
			Balance:              5.39,
			QuotaRemaining:       100000,
			RPMRemaining:         85,
			TPMRemaining:         40,
			HealthScore:          0.9,
			PrimaryUsedPercent:   72,
			SecondaryUsedPercent: 41,
			PrimaryResetsAt:      timePtr(time.Now().UTC().Add(2 * time.Hour)),
			SecondaryResetsAt:    timePtr(time.Now().UTC().Add(24 * time.Hour)),
			CheckedAt:            time.Now().UTC(),
		},
		{
			AccountID:      2,
			Balance:        100,
			QuotaRemaining: 100000,
			RPMRemaining:   100,
			TPMRemaining:   100000,
			HealthScore:    0.8,
			CheckedAt:      time.Now().UTC(),
		},
	} {
		if err := usageRepo.Save(snapshot); err != nil {
			t.Fatalf("Save returned error: %v", err)
		}
	}

	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()))

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"output_text":"fallback-success"`) {
		t.Fatalf("body = %s, want fallback output", rec.Body.String())
	}
	if primaryCalls != 1 {
		t.Fatalf("primaryCalls = %d, want 1", primaryCalls)
	}
	if secondaryCalls != 1 {
		t.Fatalf("secondaryCalls = %d, want 1", secondaryCalls)
	}

	primaryAccount, err := accountRepo.GetByID(1)
	if err != nil {
		t.Fatalf("GetByID returned error: %v", err)
	}
	if primaryAccount.Status != accounts.StatusCooldown {
		t.Fatalf("status = %q, want %q", primaryAccount.Status, accounts.StatusCooldown)
	}
	if primaryAccount.CooldownUntil == nil {
		t.Fatal("CooldownUntil = nil, want cooldown timestamp")
	}

	events, err := usageRepo.ListRecentEvents(usage.EventFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListRecentEvents returned error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2", len(events))
	}
	if events[0].AccountID != 2 || events[0].Status != "completed" {
		t.Fatalf("latest event = %+v, want fallback completed", events[0])
	}
	if events[1].AccountID != 1 || events[1].Status != "usage_limited" {
		t.Fatalf("previous event = %+v, want primary usage_limited", events[1])
	}
}

func TestResponsesHandlerThinModeSkipsOfficialBelowRemainingThreshold(t *testing.T) {
	t.Parallel()

	primaryCalls := 0
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_should_not_run","object":"response","status":"completed","output_text":"should-not-run"}`)
	}))
	defer primary.Close()

	secondaryCalls := 0
	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondaryCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_threshold_fallback","object":"response","status":"completed","output_text":"threshold-fallback"}`)
	}))
	defer secondary.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	for _, account := range []accounts.Account{
		{
			ProviderType: accounts.ProviderOpenAIOfficial,
			AccountName:  "official-low-remaining",
			AuthMode:     accounts.AuthModeLocalImport,
			BaseURL:      primary.URL + "/backend-api/codex",
			CredentialRef: `{
				"auth_mode":"chatgpt",
				"tokens":{"access_token":"token-primary","account_id":"acct-primary"}
			}`,
			Status:            accounts.StatusActive,
			Priority:          100,
			SupportsResponses: true,
			IsActive:          true,
		},
		{
			ProviderType:      accounts.ProviderOpenAICompatible,
			AccountName:       "fallback-third-party",
			AuthMode:          accounts.AuthModeAPIKey,
			BaseURL:           secondary.URL + "/v1",
			CredentialRef:     "sk-fallback",
			Status:            accounts.StatusActive,
			Priority:          90,
			SupportsResponses: true,
		},
	} {
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	for _, snapshot := range []usage.Snapshot{
		{
			AccountID:            1,
			Balance:              5.39,
			QuotaRemaining:       100000,
			RPMRemaining:         2,
			TPMRemaining:         2,
			HealthScore:          0.02,
			PrimaryUsedPercent:   98.5,
			SecondaryUsedPercent: 97.4,
			PrimaryResetsAt:      timePtr(time.Now().UTC().Add(2 * time.Hour)),
			SecondaryResetsAt:    timePtr(time.Now().UTC().Add(24 * time.Hour)),
			CheckedAt:            time.Now().UTC(),
		},
		{
			AccountID:      2,
			Balance:        100,
			QuotaRemaining: 100000,
			RPMRemaining:   100,
			TPMRemaining:   100000,
			HealthScore:    0.8,
			CheckedAt:      time.Now().UTC(),
		},
	} {
		if err := usageRepo.Save(snapshot); err != nil {
			t.Fatalf("Save returned error: %v", err)
		}
	}

	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()))

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"output_text":"threshold-fallback"`) {
		t.Fatalf("body = %s, want threshold fallback output", rec.Body.String())
	}
	if primaryCalls != 0 {
		t.Fatalf("primaryCalls = %d, want 0", primaryCalls)
	}
	if secondaryCalls != 1 {
		t.Fatalf("secondaryCalls = %d, want 1", secondaryCalls)
	}
}

func timePtr(value time.Time) *time.Time {
	return &value
}
