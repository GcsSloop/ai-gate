package accountdrv_test

import (
	"context"
	"encoding/base64"
	"errors"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/accountdrv"
)

func TestAPIKeyDriverResolve(t *testing.T) {
	t.Parallel()

	driver := accountdrv.NewAPIKeyDriver()
	account := accounts.Account{
		AuthMode:      accounts.AuthModeAPIKey,
		CredentialRef: "sk-test",
	}

	resolved, err := driver.Resolve(context.Background(), account)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if resolved.Kind != "api_key" {
		t.Fatalf("Kind = %q, want %q", resolved.Kind, "api_key")
	}
	if resolved.APIKey != "sk-test" {
		t.Fatalf("APIKey = %q, want %q", resolved.APIKey, "sk-test")
	}
}

func TestOfficialDriverResolveRefreshesAndPersistsCredential(t *testing.T) {
	t.Parallel()

	var refreshCalls int
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		values, err := url.ParseQuery(string(raw))
		if err != nil {
			t.Fatalf("ParseQuery returned error: %v", err)
		}
		if got := values.Get("refresh_token"); got != "rt-old" {
			t.Fatalf("refresh_token = %q, want %q", got, "rt-old")
		}
		refreshCalls++

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "at-new",
			"refresh_token": "rt-new",
		})
	}))
	defer tokenServer.Close()

	rawCredential, err := json.Marshal(map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]any{
			"access_token":  testJWT(t, map[string]any{"exp": time.Now().UTC().Add(-1 * time.Minute).Unix(), "client_id": "app-test-client"}),
			"refresh_token": "rt-old",
			"account_id":    "acct-1",
		},
	})
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}

	updater := &capturingUpdater{}
	driver := accountdrv.NewOfficialDriver(http.DefaultClient, updater)
	driver.SetTokenURLForTest(tokenServer.URL)

	resolved, err := driver.Resolve(context.Background(), accounts.Account{
		ID:            42,
		ProviderType:  accounts.ProviderOpenAIOfficial,
		AccountName:   "official",
		AuthMode:      accounts.AuthModeLocalImport,
		CredentialRef: string(rawCredential),
	})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if resolved.Kind != "bearer" {
		t.Fatalf("Kind = %q, want %q", resolved.Kind, "bearer")
	}
	if resolved.AccessToken != "at-new" {
		t.Fatalf("AccessToken = %q, want %q", resolved.AccessToken, "at-new")
	}
	if resolved.Metadata["account_id"] != "acct-1" {
		t.Fatalf("account_id metadata = %#v, want acct-1", resolved.Metadata["account_id"])
	}
	if refreshCalls != 1 {
		t.Fatalf("refreshCalls = %d, want 1", refreshCalls)
	}
	if updater.updateCount() != 1 {
		t.Fatalf("updateCount = %d, want 1", updater.updateCount())
	}
	if updater.credentialRef() == string(rawCredential) {
		t.Fatal("credential ref was not refreshed")
	}
}

func TestOfficialDriverResolveReturnsAuthErrorForInvalidCredential(t *testing.T) {
	t.Parallel()

	driver := accountdrv.NewOfficialDriver(http.DefaultClient, nil)

	_, err := driver.Resolve(context.Background(), accounts.Account{
		ProviderType:  accounts.ProviderOpenAIOfficial,
		AuthMode:      accounts.AuthModeLocalImport,
		CredentialRef: "{invalid",
	})
	if err == nil {
		t.Fatal("Resolve returned nil error, want error")
	}
	var resolveErr *accountdrv.ResolveError
	if !errors.As(err, &resolveErr) {
		t.Fatalf("error type = %T, want *accountdrv.ResolveError", err)
	}
	if resolveErr.Kind != accountdrv.ResolveErrorKindAuth {
		t.Fatalf("error kind = %q, want %q", resolveErr.Kind, accountdrv.ResolveErrorKindAuth)
	}
}

type capturingUpdater struct {
	mu         sync.Mutex
	updates    int
	credential string
}

func (u *capturingUpdater) Update(account accounts.Account) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.updates++
	u.credential = account.CredentialRef
	return nil
}

func (u *capturingUpdater) updateCount() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.updates
}

func (u *capturingUpdater) credentialRef() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.credential
}

func testJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	headerRaw, err := json.Marshal(map[string]any{"alg": "none", "typ": "JWT"})
	if err != nil {
		t.Fatalf("Marshal header returned error: %v", err)
	}
	claimsRaw, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("Marshal claims returned error: %v", err)
	}
	return encodeJWTPart(headerRaw) + "." + encodeJWTPart(claimsRaw) + "."
}

func encodeJWTPart(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}
