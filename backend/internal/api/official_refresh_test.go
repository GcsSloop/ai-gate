package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/conversations"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

func TestThinModeRefreshFailureTerminal(t *testing.T) {
	t.Parallel()

	oldRefreshURL := officialTokenRefreshURL
	officialTokenRefreshURL = ""
	t.Cleanup(func() {
		officialTokenRefreshURL = oldRefreshURL
	})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			http.Error(w, "refresh failed", http.StatusBadGateway)
		default:
			t.Fatalf("unexpected upstream path %q", r.URL.Path)
		}
	}))
	defer upstream.Close()
	officialTokenRefreshURL = upstream.URL + "/oauth/token"

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	usageRepo := usage.NewSQLiteRepository(store.DB())
	handler := NewResponsesHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()))
	handler.client = http.DefaultClient

	expiredToken := authTestJWT(t, map[string]any{
		"exp":       time.Now().UTC().Add(-1 * time.Minute).Unix(),
		"client_id": "app-test-client",
	})
	if err := accountRepo.Create(accounts.Account{
		ProviderType: accounts.ProviderOpenAIOfficial,
		AccountName:  "official",
		AuthMode:     accounts.AuthModeLocalImport,
		BaseURL:      upstream.URL + "/backend-api/codex",
		CredentialRef: `{
			"auth_mode":"chatgpt",
			"last_refresh":"2026-03-07T10:00:00Z",
			"tokens":{"access_token":"` + expiredToken + `","refresh_token":"rt-old","account_id":"acct-1"}
		}`,
		Status:   accounts.StatusActive,
		Priority: 100,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
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

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"ping",
		"stream":true
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"type":"error"`) {
		t.Fatalf("response body = %s, want error event", body)
	}
	if !strings.Contains(body, `data: [DONE]`) {
		t.Fatalf("response body = %s, want DONE marker", body)
	}
}

func authTestJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	headerRaw, err := json.Marshal(map[string]any{"alg": "none", "typ": "JWT"})
	if err != nil {
		t.Fatalf("Marshal header returned error: %v", err)
	}
	claimsRaw, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("Marshal claims returned error: %v", err)
	}
	return authEncodeJWTPart(headerRaw) + "." + authEncodeJWTPart(claimsRaw) + "."
}

func authEncodeJWTPart(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}
