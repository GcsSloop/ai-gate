package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
)

func TestEnsureOfficialAccountSessionSharesConcurrentRefreshes(t *testing.T) {
	originalTokenURL := officialTokenRefreshURL
	originalRefreshes := officialRefreshes
	t.Cleanup(func() {
		officialTokenRefreshURL = originalTokenURL
		officialRefreshes = originalRefreshes
	})
	officialRefreshes = newOfficialRefreshCoordinator()

	var refreshMu sync.Mutex
	refreshCalls := 0
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
			t.Fatalf("refresh_token = %q, want rt-old", got)
		}

		refreshMu.Lock()
		refreshCalls++
		callNumber := refreshCalls
		refreshMu.Unlock()

		if callNumber > 1 {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}

		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "at-new",
			"refresh_token": "rt-new",
		})
	}))
	defer tokenServer.Close()
	officialTokenRefreshURL = tokenServer.URL

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

	updater := &capturingAccountUpdater{}
	const callers = 6
	results := make([]error, callers)
	credentials := make([]string, callers)
	start := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start

			account := accounts.Account{
				ID:            42,
				ProviderType:  accounts.ProviderOpenAIOfficial,
				AccountName:   "official",
				AuthMode:      accounts.AuthModeLocalImport,
				CredentialRef: string(rawCredential),
			}
			results[index] = ensureOfficialAccountSession(context.Background(), http.DefaultClient, updater, &account)
			credentials[index] = account.CredentialRef
		}(i)
	}

	close(start)
	wg.Wait()

	for i, err := range results {
		if err != nil {
			t.Fatalf("results[%d] returned error: %v", i, err)
		}
		if credentials[i] != updater.credentialRef() {
			t.Fatalf("credentials[%d] was not updated to the shared refresh result", i)
		}
	}
	if got := updater.updateCount(); got != 1 {
		t.Fatalf("updateCount = %d, want 1", got)
	}
	if refreshCalls != 1 {
		t.Fatalf("refreshCalls = %d, want 1", refreshCalls)
	}
}

type capturingAccountUpdater struct {
	mu        sync.Mutex
	updates   int
	credential string
}

func (u *capturingAccountUpdater) Update(account accounts.Account) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.updates++
	u.credential = account.CredentialRef
	return nil
}

func (u *capturingAccountUpdater) updateCount() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.updates
}

func (u *capturingAccountUpdater) credentialRef() string {
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
