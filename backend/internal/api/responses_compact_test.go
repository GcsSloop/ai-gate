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
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

func TestResponsesHandlerCompactEndpointExists(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	usageRepo := usage.NewSQLiteRepository(store.DB())
	conversationRepo := conversations.NewSQLiteRepository(store.DB())

	// One active account is enough to keep handler initialization consistent, even if
	// compaction is currently a local operation.
	if err := accountRepo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "ppchat",
		AuthMode:      accounts.AuthModeAPIKey,
		BaseURL:       "http://unused.local/v1",
		CredentialRef: "sk-unused",
		Status:        accounts.StatusActive,
		Priority:      100,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversationRepo)

	reqBody := map[string]any{
		"model":        "gpt-5.4",
		"instructions": "summarize",
		"input": []any{
			map[string]any{
				"type": "message",
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": "hello"},
				},
			},
		},
	}
	raw, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses/compact", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses/compact status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var payload struct {
		Output []map[string]any `json:"output"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if len(payload.Output) == 0 {
		t.Fatalf("compact output is empty, want at least 1 item")
	}
}

func TestResponsesHandlerThinModeDisablesCompact(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	usageRepo := usage.NewSQLiteRepository(store.DB())
	conversationRepo := conversations.NewSQLiteRepository(store.DB())
	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversationRepo, api.WithThinGatewayMode(true))

	req := httptest.NewRequest(http.MethodPost, "/v1/responses/compact", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":[{"role":"user","content":[{"type":"input_text","text":"hello"}]}]
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("POST /v1/responses/compact status = %d, want %d, body=%s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("thin gateway mode")) {
		t.Fatalf("response body = %s, want thin gateway mode error", rec.Body.String())
	}
}
