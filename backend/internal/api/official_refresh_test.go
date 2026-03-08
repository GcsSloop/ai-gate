package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/auth"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

func TestAccountsHandlerRefreshesExpiredOfficialTokenBeforeWhamUsage(t *testing.T) {
	t.Parallel()

	oldRefreshURL := officialTokenRefreshURL
	officialTokenRefreshURL = ""
	t.Cleanup(func() {
		officialTokenRefreshURL = oldRefreshURL
	})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll returned error: %v", err)
			}
			body := string(raw)
			if !strings.Contains(body, "grant_type=refresh_token") {
				t.Fatalf("refresh body = %q, want grant_type=refresh_token", body)
			}
			if !strings.Contains(body, "refresh_token=rt-old") {
				t.Fatalf("refresh body = %q, want refresh_token=rt-old", body)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"token-new","refresh_token":"rt-new","id_token":"id-new","expires_in":3600}`)
		case "/backend-api/wham/usage":
			if got := r.Header.Get("Authorization"); got != "Bearer token-new" {
				t.Fatalf("authorization = %q, want Bearer token-new", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{
				"rate_limit":{"allowed":true,"limit_reached":false,"primary_window":{"used_percent":12,"reset_at":1772895924},"secondary_window":{"used_percent":35,"reset_at":1773332429}},
				"credits":{"has_credits":false,"unlimited":false,"balance":"0"}
			}`)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer upstream.Close()
	officialTokenRefreshURL = upstream.URL + "/oauth/token"

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	usageRepo := usage.NewSQLiteRepository(store.DB())
	handler := NewAccountsHandler(repo, usageRepo, auth.NewOAuthConnector(auth.Config{}), auth.NewStateStore(5*time.Minute))
	handler.client = http.DefaultClient

	expiredToken := authTestJWT(t, map[string]any{
		"exp":       time.Now().UTC().Add(-1 * time.Minute).Unix(),
		"client_id": "app-test-client",
	})
	if err := repo.Create(accounts.Account{
		ProviderType: accounts.ProviderOpenAIOfficial,
		AccountName:  "official",
		AuthMode:     accounts.AuthModeLocalImport,
		BaseURL:      upstream.URL + "/backend-api/codex",
		CredentialRef: `{
			"auth_mode":"chatgpt",
			"last_refresh":"2026-03-07T10:00:00Z",
			"tokens":{"access_token":"` + expiredToken + `","refresh_token":"rt-old","account_id":"acct-1"}
		}`,
		Status: accounts.StatusActive,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/accounts/usage", bytes.NewBuffer(nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /accounts/usage status = %d, want %d", rec.Code, http.StatusOK)
	}

	account, err := repo.GetByID(1)
	if err != nil {
		t.Fatalf("GetByID returned error: %v", err)
	}
	if !strings.Contains(account.CredentialRef, `"access_token":"token-new"`) {
		t.Fatalf("credential_ref = %q, want refreshed access token", account.CredentialRef)
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
