package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/api"
	"github.com/gcssloop/codex-router/backend/internal/conversations"
	"github.com/gcssloop/codex-router/backend/internal/usage"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
)

func TestGatewayHandlerProxiesToConfiguredAccount(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("upstream path = %q, want %q", r.URL.Path, "/v1/chat/completions")
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Fatalf("Authorization = %q, want %q", r.Header.Get("Authorization"), "Bearer sk-test")
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode returned error: %v", err)
		}
		if payload["model"] != "gpt-5.2-codex" {
			t.Fatalf("model = %v, want %v", payload["model"], "gpt-5.2-codex")
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "chat.completion",
			"model":  "gpt-5.2-codex",
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "ok"}},
			},
		})
	}))
	defer upstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	if err := accountRepo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "ppchat-main",
		AuthMode:      accounts.AuthModeAPIKey,
		BaseURL:       upstream.URL + "/v1",
		CredentialRef: "sk-test",
		Status:        accounts.StatusActive,
		Priority:      100,
	}); err != nil {
		t.Fatalf("Create(account) returned error: %v", err)
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

	conversationRepo := conversations.NewSQLiteRepository(store.DB())
	handler := api.NewGatewayHandler(accountRepo, usageRepo, conversationRepo)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{
		"model":"gpt-5.2-codex",
		"stream":false,
		"messages":[{"role":"user","content":"ping"}]
	}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("gateway status = %d, want %d", rec.Code, http.StatusOK)
	}

	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if response["model"] != "gpt-5.2-codex" {
		t.Fatalf("response model = %v, want %v", response["model"], "gpt-5.2-codex")
	}
}
