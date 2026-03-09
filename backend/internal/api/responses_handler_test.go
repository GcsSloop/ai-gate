package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/api"
	"github.com/gcssloop/codex-router/backend/internal/conversations"
	"github.com/gcssloop/codex-router/backend/internal/settings"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

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
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "supports /responses") {
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
