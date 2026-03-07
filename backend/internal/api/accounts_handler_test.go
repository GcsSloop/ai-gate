package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/api"
	"github.com/gcssloop/codex-router/backend/internal/auth"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
)

func TestAccountsHandler(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	connector := auth.NewOAuthConnector(auth.Config{
		ClientID:     "client-id",
		AuthorizeURL: "https://auth.example.test/oauth/authorize",
		TokenURL:     "https://auth.example.test/oauth/token",
		RedirectURL:  "http://localhost:8080/callback",
		Scopes:       []string{"model.read"},
	})
	handler := api.NewAccountsHandler(repo, connector, auth.NewStateStore(5*time.Minute))

	createBody := bytes.NewBufferString(`{
		"provider_type":"openai-compatible",
		"account_name":"mirror-east",
		"auth_mode":"api_key",
		"base_url":"https://mirror.example.test/v1",
		"credential_ref":"cred-api-key"
	}`)
	createReq := httptest.NewRequest(http.MethodPost, "/accounts", createBody)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("POST /accounts status = %d, want %d", createRec.Code, http.StatusCreated)
	}

	authReq := httptest.NewRequest(http.MethodPost, "/accounts/auth/authorize", bytes.NewBufferString(`{}`))
	authReq.Header.Set("Content-Type", "application/json")
	authRec := httptest.NewRecorder()
	handler.ServeHTTP(authRec, authReq)
	if authRec.Code != http.StatusOK {
		t.Fatalf("POST /accounts/auth/authorize status = %d, want %d", authRec.Code, http.StatusOK)
	}

	cooldownUntil := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	if err := repo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAIOfficial,
		AccountName:   "official-secondary",
		AuthMode:      accounts.AuthModeOAuth,
		CredentialRef: "cred-oauth",
		Status:        accounts.StatusCooldown,
		CooldownUntil: &cooldownUntil,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/accounts", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("GET /accounts status = %d, want %d", listRec.Code, http.StatusOK)
	}

	var listed []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("GET /accounts returned %d items, want 2", len(listed))
	}
	if listed[1]["cooldown_remaining_seconds"] == nil {
		t.Fatal("cooldown_remaining_seconds missing from cooldown account")
	}

	disableReq := httptest.NewRequest(http.MethodPost, "/accounts/1/disable", nil)
	disableRec := httptest.NewRecorder()
	handler.ServeHTTP(disableRec, disableReq)
	if disableRec.Code != http.StatusNoContent {
		t.Fatalf("POST /accounts/1/disable status = %d, want %d", disableRec.Code, http.StatusNoContent)
	}
}
