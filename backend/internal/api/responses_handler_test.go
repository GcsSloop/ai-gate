package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/api"
	"github.com/gcssloop/codex-router/backend/internal/conversations"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

func TestResponsesHandlerProxiesOpenAICompatibleAccount(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if maybeRejectResponsesPathForCompatTests(w, r) {
			return
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("upstream path = %q, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-third-party" {
			t.Fatalf("authorization = %q, want Bearer sk-third-party", got)
		}

		var payload struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode returned error: %v", err)
		}
		if payload.Model != "gpt-5.4" {
			t.Fatalf("model = %q, want gpt-5.4", payload.Model)
		}
		if len(payload.Messages) != 1 || payload.Messages[0].Content != "ping" {
			t.Fatalf("messages = %+v, want single ping message", payload.Messages)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "pong"}},
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
		ProviderType:      accounts.ProviderOpenAICompatible,
		AllowChatFallback: true,
		AccountName:       "ppchat",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           upstream.URL + "/v1",
		CredentialRef:     "sk-third-party",
		Status:            accounts.StatusActive,
		Priority:          100,
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
		t.Fatalf("Save returned error: %v", err)
	}

	conversationRepo := conversations.NewSQLiteRepository(store.DB())
	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversationRepo)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"ping",
		"stream":false
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload struct {
		ID         string `json:"id"`
		OutputText string `json:"output_text"`
		Output     []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if !strings.HasPrefix(payload.ID, "resp_") {
		t.Fatalf("id = %q, want resp_ prefix", payload.ID)
	}
	if payload.OutputText != "pong" {
		t.Fatalf("output_text = %q, want pong", payload.OutputText)
	}
	if len(payload.Output) == 0 || len(payload.Output[0].Content) == 0 || payload.Output[0].Content[0].Text != "pong" {
		t.Fatalf("output content = %+v, want pong", payload.Output)
	}
}

func TestResponsesHandlerThinModePassthroughID(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/codex/responses" {
			t.Fatalf("upstream path = %q, want /backend-api/codex/responses", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token-1" {
			t.Fatalf("authorization = %q, want Bearer token-1", got)
		}
		if got := r.Header.Get("ChatGPT-Account-Id"); got != "acct-1" {
			t.Fatalf("ChatGPT-Account-Id = %q, want acct-1", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode returned error: %v", err)
		}
		if got := payload["stream"]; got != false {
			t.Fatalf("stream = %#v, want false", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "resp_server_123",
			"object":      "response",
			"status":      "completed",
			"output_text": "official-pong",
			"output": []map[string]any{
				{
					"id":     "msg_server_1",
					"type":   "message",
					"role":   "assistant",
					"status": "completed",
					"content": []map[string]any{
						{"type": "output_text", "text": "official-pong"},
					},
				},
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
		ProviderType: accounts.ProviderOpenAIOfficial,
		AccountName:  "official",
		AuthMode:     accounts.AuthModeLocalImport,
		BaseURL:      upstream.URL + "/backend-api/codex",
		CredentialRef: `{
			"auth_mode":"chatgpt",
			"tokens":{"access_token":"token-1","account_id":"acct-1"}
		}`,
		Status:   accounts.StatusActive,
		Priority: 100,
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
		t.Fatalf("Save returned error: %v", err)
	}

	conversationRepo := conversations.NewSQLiteRepository(store.DB())
	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversationRepo, api.WithThinGatewayMode(true))

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"ping",
		"stream":false
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload struct {
		ID         string `json:"id"`
		OutputText string `json:"output_text"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if payload.ID != "resp_server_123" {
		t.Fatalf("id = %q, want upstream id", payload.ID)
	}
	if payload.OutputText != "official-pong" {
		t.Fatalf("output_text = %q, want official-pong", payload.OutputText)
	}
}

func TestResponsesHandlerThinModePassthroughSSE(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/codex/responses" {
			t.Fatalf("upstream path = %q, want /backend-api/codex/responses", r.URL.Path)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_server_456\",\"object\":\"response\",\"status\":\"in_progress\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"official-pong\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_server_456\",\"object\":\"response\",\"status\":\"completed\",\"output_text\":\"official-pong\"}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
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
		AccountName:  "official",
		AuthMode:     accounts.AuthModeLocalImport,
		BaseURL:      upstream.URL + "/backend-api/codex",
		CredentialRef: `{
			"auth_mode":"chatgpt",
			"tokens":{"access_token":"token-1","account_id":"acct-1"}
		}`,
		Status:   accounts.StatusActive,
		Priority: 100,
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
		t.Fatalf("Save returned error: %v", err)
	}

	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()), api.WithThinGatewayMode(true))

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"ping",
		"stream":true
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "\"id\":\"resp_server_456\"") {
		t.Fatalf("response body = %s, want upstream response id", body)
	}
	if strings.Contains(body, "\"sequence_number\":") {
		t.Fatalf("response body = %s, want no synthetic sequence_number", body)
	}
	if strings.Contains(body, "\"response_id\":") {
		t.Fatalf("response body = %s, want no synthetic response_id", body)
	}
}

func TestResponsesHandlerProxiesLocalImportedOfficialAccount(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/codex/responses" {
			t.Fatalf("upstream path = %q, want /backend-api/codex/responses", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token-1" {
			t.Fatalf("authorization = %q, want Bearer token-1", got)
		}
		if got := r.Header.Get("ChatGPT-Account-Id"); got != "acct-1" {
			t.Fatalf("ChatGPT-Account-Id = %q, want acct-1", got)
		}

		var payload struct {
			Model        string `json:"model"`
			Stream       bool   `json:"stream"`
			Store        *bool  `json:"store"`
			Instructions string `json:"instructions"`
			Input        []struct {
				Role    string `json:"role"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode returned error: %v", err)
		}
		if payload.Model != "gpt-5.4" {
			t.Fatalf("model = %q, want gpt-5.4", payload.Model)
		}
		if !payload.Stream {
			t.Fatal("stream = false, want true for official codex backend")
		}
		if payload.Store == nil || *payload.Store {
			t.Fatalf("store = %#v, want explicit false", payload.Store)
		}
		if strings.TrimSpace(payload.Instructions) == "" {
			t.Fatal("instructions is empty, want default codex instructions")
		}
		if len(payload.Input) != 1 || len(payload.Input[0].Content) != 1 || payload.Input[0].Content[0].Text != "ping" {
			t.Fatalf("input = %+v, want single ping item", payload.Input)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"official-pong\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":54,\"input_tokens_details\":{\"cached_tokens\":0},\"output_tokens\":5,\"total_tokens\":59},\"store\":false}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
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
		AccountName:  "official",
		AuthMode:     accounts.AuthModeLocalImport,
		BaseURL:      upstream.URL + "/backend-api/codex",
		CredentialRef: `{
			"auth_mode":"chatgpt",
			"tokens":{"access_token":"token-1","account_id":"acct-1"}
		}`,
		Status:   accounts.StatusActive,
		Priority: 100,
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
		t.Fatalf("Save returned error: %v", err)
	}

	conversationRepo := conversations.NewSQLiteRepository(store.DB())
	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversationRepo)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"ping",
		"stream":false
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses status = %d, want %d", rec.Code, http.StatusOK)
	}

	if !strings.Contains(rec.Body.String(), "official-pong") {
		t.Fatalf("response body = %s, want official-pong", rec.Body.String())
	}

	snapshot, err := usageRepo.GetLatest(1)
	if err != nil {
		t.Fatalf("GetLatest returned error: %v", err)
	}
	if snapshot.LastTotalTokens != 59 {
		t.Fatalf("LastTotalTokens = %v, want 59", snapshot.LastTotalTokens)
	}
	if snapshot.LastInputTokens != 54 {
		t.Fatalf("LastInputTokens = %v, want 54", snapshot.LastInputTokens)
	}
	if snapshot.LastOutputTokens != 5 {
		t.Fatalf("LastOutputTokens = %v, want 5", snapshot.LastOutputTokens)
	}
}

func TestResponsesHandlerProxiesLocalImportedOfficialAccountFromCompletedOutputOnly(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"id\":\"msg_1\",\"type\":\"message\",\"status\":\"completed\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"official-pong\",\"annotations\":[]}]}],\"usage\":{\"input_tokens\":54,\"input_tokens_details\":{\"cached_tokens\":0},\"output_tokens\":5,\"total_tokens\":59},\"store\":false}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
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
		AccountName:  "official",
		AuthMode:     accounts.AuthModeLocalImport,
		BaseURL:      upstream.URL + "/backend-api/codex",
		CredentialRef: `{
			"auth_mode":"chatgpt",
			"tokens":{"access_token":"token-1","account_id":"acct-1"}
		}`,
		Status:   accounts.StatusActive,
		Priority: 100,
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
		t.Fatalf("Save returned error: %v", err)
	}

	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()))
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"ping",
		"stream":false
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "official-pong") {
		t.Fatalf("response body = %s, want official-pong from completed output", rec.Body.String())
	}
}

func TestResponsesHandlerPrefersActiveAccountOverPriority(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if maybeRejectResponsesPathForCompatTests(w, r) {
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-active" {
			t.Fatalf("authorization = %q, want Bearer sk-active", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "pong"}},
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
	for _, item := range []accounts.Account{
		{
			ProviderType:      accounts.ProviderOpenAICompatible,
			AllowChatFallback: true,
			AccountName:       "high-priority",
			AuthMode:          accounts.AuthModeAPIKey,
			BaseURL:           upstream.URL + "/v1",
			CredentialRef:     "sk-high",
			Status:            accounts.StatusActive,
			Priority:          100,
		},
		{
			ProviderType:      accounts.ProviderOpenAICompatible,
			AllowChatFallback: true,
			AccountName:       "manual-active",
			AuthMode:          accounts.AuthModeAPIKey,
			BaseURL:           upstream.URL + "/v1",
			CredentialRef:     "sk-active",
			Status:            accounts.StatusActive,
			Priority:          1,
			IsActive:          true,
		},
	} {
		if err := accountRepo.Create(item); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
	}
	usageRepo := usage.NewSQLiteRepository(store.DB())
	for id := int64(1); id <= 2; id++ {
		if err := usageRepo.Save(usage.Snapshot{
			AccountID:      id,
			Balance:        100,
			QuotaRemaining: 100000,
			RPMRemaining:   100,
			TPMRemaining:   100000,
			HealthScore:    0.9,
		}); err != nil {
			t.Fatalf("Save returned error: %v", err)
		}
	}

	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()))
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"ping",
		"stream":false
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "pong") {
		t.Fatalf("body = %s, want pong", rec.Body.String())
	}
}

func TestResponsesHandlerPreservesOfficialCompletedOutputItems(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"id\":\"fc_1\",\"type\":\"function_call\",\"call_id\":\"call_1\",\"name\":\"grep\",\"arguments\":\"{\\\"pattern\\\":\\\"TODO\\\"}\",\"status\":\"completed\"}],\"usage\":{\"input_tokens\":12,\"input_tokens_details\":{\"cached_tokens\":0},\"output_tokens\":3,\"total_tokens\":15},\"store\":false}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
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
		AccountName:  "official",
		AuthMode:     accounts.AuthModeLocalImport,
		BaseURL:      upstream.URL + "/backend-api/codex",
		CredentialRef: `{
			"auth_mode":"chatgpt",
			"tokens":{"access_token":"token-1","account_id":"acct-1"}
		}`,
		Status:   accounts.StatusActive,
		Priority: 100,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	usageRepo := usage.NewSQLiteRepository(store.DB())
	if err := usageRepo.Save(usage.Snapshot{AccountID: 1, Balance: 100, QuotaRemaining: 100000, RPMRemaining: 100, TPMRemaining: 100000, HealthScore: 0.9}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()))
	createReq := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"run grep"
	}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses status = %d, want %d", createRec.Code, http.StatusOK)
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/responses/"+created.ID, nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /v1/responses/{id} status = %d, want %d", getRec.Code, http.StatusOK)
	}
	body := getRec.Body.String()
	if !strings.Contains(body, `"type":"function_call"`) || !strings.Contains(body, `"call_id":"call_1"`) || !strings.Contains(body, `"name":"grep"`) {
		t.Fatalf("retrieved body = %s, want preserved function_call output", body)
	}
}

func TestResponsesHandlerRetrievesMultipleOfficialOutputItems(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"done\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"id\":\"msg_1\",\"type\":\"message\",\"status\":\"completed\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"done\",\"annotations\":[]}]},{\"id\":\"fc_1\",\"type\":\"function_call\",\"call_id\":\"call_1\",\"name\":\"grep\",\"arguments\":\"{\\\"pattern\\\":\\\"TODO\\\"}\",\"status\":\"completed\"}],\"usage\":{\"input_tokens\":12,\"input_tokens_details\":{\"cached_tokens\":0},\"output_tokens\":3,\"total_tokens\":15},\"store\":false}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
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
		AccountName:  "official",
		AuthMode:     accounts.AuthModeLocalImport,
		BaseURL:      upstream.URL + "/backend-api/codex",
		CredentialRef: `{
			"auth_mode":"chatgpt",
			"tokens":{"access_token":"token-1","account_id":"acct-1"}
		}`,
		Status:   accounts.StatusActive,
		Priority: 100,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	usageRepo := usage.NewSQLiteRepository(store.DB())
	if err := usageRepo.Save(usage.Snapshot{AccountID: 1, Balance: 100, QuotaRemaining: 100000, RPMRemaining: 100, TPMRemaining: 100000, HealthScore: 0.9}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()))
	createReq := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"run grep"
	}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses status = %d, want %d", createRec.Code, http.StatusOK)
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/responses/"+created.ID, nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /v1/responses/{id} status = %d, want %d", getRec.Code, http.StatusOK)
	}
	body := getRec.Body.String()
	if !strings.Contains(body, `"type":"message"`) || !strings.Contains(body, `"text":"done"`) {
		t.Fatalf("retrieved body = %s, want preserved message output", body)
	}
	if !strings.Contains(body, `"type":"function_call"`) || !strings.Contains(body, `"call_id":"call_1"`) {
		t.Fatalf("retrieved body = %s, want preserved function_call output", body)
	}
}

func TestResponsesHandlerDoesNotMixFinalTextIntoFunctionCallOutputItem(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"final-answer\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"id\":\"fc_1\",\"type\":\"function_call\",\"call_id\":\"call_1\",\"name\":\"grep\",\"arguments\":\"{\\\"pattern\\\":\\\"TODO\\\"}\",\"status\":\"completed\"},{\"id\":\"msg_1\",\"type\":\"message\",\"status\":\"completed\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"final-answer\",\"annotations\":[]}]}],\"usage\":{\"input_tokens\":12,\"input_tokens_details\":{\"cached_tokens\":0},\"output_tokens\":3,\"total_tokens\":15},\"store\":false}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
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
		AccountName:  "official",
		AuthMode:     accounts.AuthModeLocalImport,
		BaseURL:      upstream.URL + "/backend-api/codex",
		CredentialRef: `{
			"auth_mode":"chatgpt",
			"tokens":{"access_token":"token-1","account_id":"acct-1"}
		}`,
		Status:   accounts.StatusActive,
		Priority: 100,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	usageRepo := usage.NewSQLiteRepository(store.DB())
	if err := usageRepo.Save(usage.Snapshot{AccountID: 1, Balance: 100, QuotaRemaining: 100000, RPMRemaining: 100, TPMRemaining: 100000, HealthScore: 0.9}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()))
	createReq := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"run grep"
	}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses status = %d, want %d", createRec.Code, http.StatusOK)
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/responses/"+created.ID, nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /v1/responses/{id} status = %d, want %d", getRec.Code, http.StatusOK)
	}

	var payload struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Type    string `json:"type"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if payload.OutputText != "final-answer" {
		t.Fatalf("output_text = %q, want %q", payload.OutputText, "final-answer")
	}
	if len(payload.Output) != 2 {
		t.Fatalf("len(output) = %d, want 2", len(payload.Output))
	}
	if payload.Output[0].Type != "function_call" {
		t.Fatalf("output[0].type = %q, want function_call", payload.Output[0].Type)
	}
	if len(payload.Output[0].Content) != 0 {
		t.Fatalf("output[0].content = %+v, want empty for function_call", payload.Output[0].Content)
	}
	if payload.Output[1].Type != "message" {
		t.Fatalf("output[1].type = %q, want message", payload.Output[1].Type)
	}
}

func TestResponsesHandlerMapsChatCompletionsToolCallsToResponsesOutput(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if maybeRejectResponsesPathForCompatTests(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role":    "assistant",
						"content": "",
						"tool_calls": []map[string]any{
							{
								"id":   "call_42",
								"type": "function",
								"function": map[string]any{
									"name":      "grep",
									"arguments": "{\"pattern\":\"TODO\"}",
								},
							},
						},
					},
				},
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
		ProviderType:      accounts.ProviderOpenAICompatible,
		AllowChatFallback: true,
		AccountName:       "ppchat",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           upstream.URL + "/v1",
		CredentialRef:     "sk-third-party",
		Status:            accounts.StatusActive,
		Priority:          100,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	usageRepo := usage.NewSQLiteRepository(store.DB())
	if err := usageRepo.Save(usage.Snapshot{AccountID: 1, Balance: 100, QuotaRemaining: 100000, RPMRemaining: 100, TPMRemaining: 100000, HealthScore: 0.9}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()))
	createReq := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"run grep"
	}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses status = %d, want %d", createRec.Code, http.StatusOK)
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/responses/"+created.ID, nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /v1/responses/{id} status = %d, want %d", getRec.Code, http.StatusOK)
	}
	body := getRec.Body.String()
	if !strings.Contains(body, `"type":"function_call"`) || !strings.Contains(body, `"call_id":"call_42"`) || !strings.Contains(body, `"name":"grep"`) {
		t.Fatalf("retrieved body = %s, want mapped function_call output", body)
	}
}

func TestResponsesHandlerUsesPreviousResponseIDForConversationReplay(t *testing.T) {
	t.Parallel()

	callCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if maybeRejectResponsesPathForCompatTests(w, r) {
			return
		}
		callCount++
		var payload struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode returned error: %v", err)
		}
		if callCount == 2 {
			if len(payload.Messages) != 3 {
				t.Fatalf("second request messages = %+v, want 3 replayed messages", payload.Messages)
			}
			if payload.Messages[0].Content != "first question" || payload.Messages[1].Content != "first answer" || payload.Messages[2].Content != "second question" {
				t.Fatalf("second request messages = %+v, want replayed conversation", payload.Messages)
			}
		}

		reply := "first answer"
		if callCount == 2 {
			reply = "second answer"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": reply}},
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
		ProviderType:      accounts.ProviderOpenAICompatible,
		AllowChatFallback: true,
		AccountName:       "ppchat",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           upstream.URL + "/v1",
		CredentialRef:     "sk-third-party",
		Status:            accounts.StatusActive,
		Priority:          100,
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
		t.Fatalf("Save returned error: %v", err)
	}

	conversationRepo := conversations.NewSQLiteRepository(store.DB())
	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversationRepo)

	firstReq := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"first question"
	}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("first POST /v1/responses status = %d, want %d", firstRec.Code, http.StatusOK)
	}

	var firstPayload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(firstRec.Body.Bytes(), &firstPayload); err != nil {
		t.Fatalf("Unmarshal first returned error: %v", err)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"previous_response_id":"`+firstPayload.ID+`",
		"input":"second question"
	}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("second POST /v1/responses status = %d, want %d", secondRec.Code, http.StatusOK)
	}
}

func TestResponsesHandlerStreamsOfficialStyleLifecycleEvents(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if maybeRejectResponsesPathForCompatTests(w, r) {
			return
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("upstream path = %q, want /v1/chat/completions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"po\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ng\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
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
		AllowChatFallback: true,
		AccountName:       "ppchat",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           upstream.URL + "/v1",
		CredentialRef:     "sk-third-party",
		Status:            accounts.StatusActive,
		Priority:          100,
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
		t.Fatalf("Save returned error: %v", err)
	}

	conversationRepo := conversations.NewSQLiteRepository(store.DB())
	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversationRepo)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"ping",
		"stream":true
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses stream status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("OpenAI-Model"); got != "gpt-5.4" {
		t.Fatalf("OpenAI-Model header = %q, want %q", got, "gpt-5.4")
	}

	body := rec.Body.String()
	expected := []string{
		`"type":"response.created"`,
		`"type":"response.in_progress"`,
		`"type":"response.output_item.added"`,
		`"type":"response.content_part.added"`,
		`"type":"response.output_text.delta"`,
		`"type":"response.output_text.done"`,
		`"type":"response.content_part.done"`,
		`"type":"response.output_item.done"`,
		`"type":"response.completed"`,
		`data: [DONE]`,
	}
	start := 0
	for _, marker := range expected {
		index := strings.Index(body[start:], marker)
		if index < 0 {
			t.Fatalf("stream body missing marker %q in %s", marker, body)
		}
		start += index + len(marker)
	}
	if strings.Contains(body, `"type":"response.failed"`) {
		t.Fatalf("stream body unexpectedly contains response.failed: %s", body)
	}
	if !strings.Contains(body, `"response_id":"resp_`) || !strings.Contains(body, `"sequence_number":`) {
		t.Fatalf("stream body missing response_id or sequence_number metadata: %s", body)
	}
	if !strings.Contains(body, `"usage":{"input_tokens":0,"input_tokens_details":{"cached_tokens":0},"output_tokens":0,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":0}`) {
		t.Fatalf("stream body missing completed usage payload: %s", body)
	}
}

func TestResponsesHandlerStreamsOfficialCompletedOutputWithoutDelta(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"id\":\"msg_1\",\"type\":\"message\",\"status\":\"completed\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"official-pong\",\"annotations\":[]}]}],\"usage\":{\"input_tokens\":54,\"input_tokens_details\":{\"cached_tokens\":0},\"output_tokens\":5,\"total_tokens\":59},\"store\":false}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
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
		AccountName:  "official",
		AuthMode:     accounts.AuthModeLocalImport,
		BaseURL:      upstream.URL + "/backend-api/codex",
		CredentialRef: `{
			"auth_mode":"chatgpt",
			"tokens":{"access_token":"token-1","account_id":"acct-1"}
		}`,
		Status:   accounts.StatusActive,
		Priority: 100,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	usageRepo := usage.NewSQLiteRepository(store.DB())
	if err := usageRepo.Save(usage.Snapshot{AccountID: 1, Balance: 100, QuotaRemaining: 100000, RPMRemaining: 100, TPMRemaining: 100000, HealthScore: 0.9}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()))
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"ping",
		"stream":true
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses stream status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"type":"response.output_text.delta"`) || !strings.Contains(body, `"delta":"official-pong"`) {
		t.Fatalf("stream body = %s, want synthetic output_text.delta from completed output", body)
	}
	if !strings.Contains(body, `"output_text":"official-pong"`) {
		t.Fatalf("stream body = %s, want completed response output_text", body)
	}
}

func TestResponsesHandlerProxiesLocalImportedOfficialAccountWithoutCompletedEventFails(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"official-partial\"}\n\n"))
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
		AccountName:  "official",
		AuthMode:     accounts.AuthModeLocalImport,
		BaseURL:      upstream.URL + "/backend-api/codex",
		CredentialRef: `{
			"auth_mode":"chatgpt",
			"tokens":{"access_token":"token-1","account_id":"acct-1"}
		}`,
		Status:   accounts.StatusActive,
		Priority: 100,
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
		t.Fatalf("Save returned error: %v", err)
	}

	conversationRepo := conversations.NewSQLiteRepository(store.DB())
	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversationRepo)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"ping",
		"stream":false
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("POST /v1/responses status = %d, want %d; body=%s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "stream closed before response.completed") {
		t.Fatalf("response body = %s, want stream closed before response.completed", rec.Body.String())
	}
}

func TestResponsesHandlerStreamsOfficialLifecycleWithoutCompletedEventReturnsError(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"))
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
		AccountName:  "official",
		AuthMode:     accounts.AuthModeLocalImport,
		BaseURL:      upstream.URL + "/backend-api/codex",
		CredentialRef: `{
			"auth_mode":"chatgpt",
			"tokens":{"access_token":"token-1","account_id":"acct-1"}
		}`,
		Status:   accounts.StatusActive,
		Priority: 100,
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
		t.Fatalf("Save returned error: %v", err)
	}

	conversationRepo := conversations.NewSQLiteRepository(store.DB())
	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversationRepo)

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
	body := rec.Body.String()
	if !strings.Contains(body, `"type":"response.completed"`) || !strings.Contains(body, `"status":"failed"`) {
		t.Fatalf("stream body missing failed response.completed event: %s", body)
	}
	if !strings.Contains(body, `"type":"error"`) {
		t.Fatalf("stream body missing error event: %s", body)
	}
	if !strings.Contains(strings.ToLower(body), "stream closed before response.completed") {
		t.Fatalf("stream body = %s, want stream closed before response.completed error", body)
	}
}

func TestResponsesHandlerStreamFailureUsesErrorEvent(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream boom", http.StatusBadGateway)
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
		AllowChatFallback: true,
		AccountName:       "ppchat",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           upstream.URL + "/v1",
		CredentialRef:     "sk-third-party",
		Status:            accounts.StatusActive,
		Priority:          100,
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
		t.Fatalf("Save returned error: %v", err)
	}

	conversationRepo := conversations.NewSQLiteRepository(store.DB())
	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversationRepo)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"ping",
		"stream":true
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses stream status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `"type":"error"`) {
		t.Fatalf("stream body missing error event: %s", body)
	}
	if !strings.Contains(body, `"type":"response.completed"`) {
		t.Fatalf("stream body missing response.completed terminal event: %s", body)
	}
	if !strings.Contains(body, `"type":"response.output_text.delta"`) {
		t.Fatalf("stream body missing output_text.delta for visible error text: %s", body)
	}
	if strings.Contains(body, `"type":"response.failed"`) {
		t.Fatalf("stream body unexpectedly contains response.failed: %s", body)
	}
	errorDetailRE := regexp.MustCompile(`"message":"[^"]+"`)
	if !errorDetailRE.MatchString(body) {
		t.Fatalf("stream error event missing message: %s", body)
	}
}

func TestResponsesHandlerStreamsToolCallOutputItem(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if maybeRejectResponsesPathForCompatTests(w, r) {
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_7\",\"type\":\"function\",\"function\":{\"name\":\"grep\",\"arguments\":\"{\\\"pattern\\\":\"}}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"TODO\\\"}\"}}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
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
		AllowChatFallback: true,
		AccountName:       "ppchat",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           upstream.URL + "/v1",
		CredentialRef:     "sk-third-party",
		Status:            accounts.StatusActive,
		Priority:          100,
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
		t.Fatalf("Save returned error: %v", err)
	}

	conversationRepo := conversations.NewSQLiteRepository(store.DB())
	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversationRepo)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"run grep",
		"stream":true
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses stream status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `"type":"response.function_call_arguments.delta"`) {
		t.Fatalf("stream body missing function_call_arguments.delta: %s", body)
	}
	if !strings.Contains(body, `"type":"response.function_call_arguments.done"`) {
		t.Fatalf("stream body missing function_call_arguments.done: %s", body)
	}
	if !strings.Contains(body, `"type":"response.output_item.done"`) {
		t.Fatalf("stream body missing output_item.done: %s", body)
	}
	if !strings.Contains(body, `"type":"response.output_item.added"`) {
		t.Fatalf("stream body missing output_item.added: %s", body)
	}
	if !strings.Contains(body, `"type":"function_call"`) || !strings.Contains(body, `"call_id":"call_7"`) || !strings.Contains(body, `"name":"grep"`) {
		t.Fatalf("stream body = %s, want function_call output item", body)
	}
	if !strings.Contains(body, `"arguments":"{\"pattern\":\"TODO\"}"`) {
		t.Fatalf("stream body = %s, want merged function arguments", body)
	}
}

func TestResponsesHandlerChatFallbackStreamWithoutDoneReturnsError(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if maybeRejectResponsesPathForCompatTests(w, r) {
			return
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q, want /v1/chat/completions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n"))
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
		AllowChatFallback: true,
		AccountName:       "ppchat",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           upstream.URL + "/v1",
		CredentialRef:     "sk-third-party",
		Status:            accounts.StatusActive,
		Priority:          100,
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
		t.Fatalf("Save returned error: %v", err)
	}

	conversationRepo := conversations.NewSQLiteRepository(store.DB())
	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversationRepo)

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
	body := rec.Body.String()
	if !strings.Contains(body, `"type":"error"`) {
		t.Fatalf("stream body missing error event: %s", body)
	}
	if !strings.Contains(body, `"status":"failed"`) {
		t.Fatalf("stream body missing failed status: %s", body)
	}
	if !strings.Contains(strings.ToLower(body), "stream closed before [done]") {
		t.Fatalf("stream body = %s, want stream closed before [DONE]", body)
	}
}

func TestResponsesHandlerStreamFailoverFromOfficialToThirdPartyDedupesOutput(t *testing.T) {
	t.Parallel()

	official := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.failed\",\"error\":{\"message\":\"upstream failed\"}}\n\n"))
	}))
	defer official.Close()

	thirdParty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if maybeRejectResponsesPathForCompatTests(w, r) {
			return
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q, want /v1/chat/completions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello world\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer thirdParty.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	if err := accountRepo.Create(accounts.Account{
		ProviderType: accounts.ProviderOpenAIOfficial,
		AccountName:  "official",
		AuthMode:     accounts.AuthModeLocalImport,
		BaseURL:      official.URL + "/backend-api/codex",
		CredentialRef: `{
			"auth_mode":"chatgpt",
			"tokens":{"access_token":"token-1","account_id":"acct-1"}
		}`,
		Status:   accounts.StatusActive,
		Priority: 100,
	}); err != nil {
		t.Fatalf("Create official returned error: %v", err)
	}
	if err := accountRepo.Create(accounts.Account{
		ProviderType:      accounts.ProviderOpenAICompatible,
		AllowChatFallback: true,
		AccountName:       "fallback",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           thirdParty.URL + "/v1",
		CredentialRef:     "sk-third",
		Status:            accounts.StatusActive,
		Priority:          90,
	}); err != nil {
		t.Fatalf("Create fallback returned error: %v", err)
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	for id := int64(1); id <= 2; id++ {
		if err := usageRepo.Save(usage.Snapshot{
			AccountID:      id,
			Balance:        100,
			QuotaRemaining: 100000,
			RPMRemaining:   100,
			TPMRemaining:   100000,
			HealthScore:    0.9,
		}); err != nil {
			t.Fatalf("Save snapshot %d returned error: %v", id, err)
		}
	}

	conversationRepo := conversations.NewSQLiteRepository(store.DB())
	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversationRepo)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"ping",
		"stream":true
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses stream status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"output_text":"Hello world"`) {
		t.Fatalf("stream body = %s, want final output_text Hello world", body)
	}
	if strings.Contains(body, `"output_text":"HelloHello world"`) {
		t.Fatalf("stream body = %s, got duplicated output text", body)
	}

	runs, err := conversationRepo.ListRuns(1)
	if err != nil {
		t.Fatalf("ListRuns returned error: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("len(runs) = %d, want 2", len(runs))
	}
	if runs[0].Status != "soft_failed" {
		t.Fatalf("runs[0].status = %q, want soft_failed", runs[0].Status)
	}
	if runs[1].Status != "completed" {
		t.Fatalf("runs[1].status = %q, want completed", runs[1].Status)
	}
}

func TestResponsesHandlerStreamAllowsActiveAccountWhenSnapshotInfeasible(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if maybeRejectResponsesPathForCompatTests(w, r) {
			return
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q, want /v1/chat/completions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"pong\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
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
		AllowChatFallback: true,
		AccountName:       "active-zero",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           upstream.URL + "/v1",
		CredentialRef:     "sk-third-party",
		Status:            accounts.StatusActive,
		IsActive:          true,
		Priority:          100,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	if err := usageRepo.Save(usage.Snapshot{
		AccountID:      1,
		Balance:        0,
		QuotaRemaining: 0,
		RPMRemaining:   0,
		TPMRemaining:   0,
		HealthScore:    1,
	}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()))
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
	body := rec.Body.String()
	if !strings.Contains(body, `"delta":"pong"`) {
		t.Fatalf("stream body = %s, want pong delta", body)
	}
	if !strings.Contains(body, `"status":"completed"`) {
		t.Fatalf("stream body = %s, want completed status", body)
	}
}

func TestResponsesHandlerStreamFailoverRecordsCapacityFailedStatusOn429(t *testing.T) {
	t.Parallel()

	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer first.Close()

	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer second.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	for index, baseURL := range []string{first.URL + "/v1", second.URL + "/v1"} {
		if err := accountRepo.Create(accounts.Account{
			ProviderType:      accounts.ProviderOpenAICompatible,
			AllowChatFallback: true,
			AccountName:       "acc",
			AuthMode:          accounts.AuthModeAPIKey,
			BaseURL:           baseURL,
			CredentialRef:     "sk",
			Status:            accounts.StatusActive,
			Priority:          100 - index,
		}); err != nil {
			t.Fatalf("Create account %d returned error: %v", index, err)
		}
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	for id := int64(1); id <= 2; id++ {
		if err := usageRepo.Save(usage.Snapshot{
			AccountID:      id,
			Balance:        100,
			QuotaRemaining: 100000,
			RPMRemaining:   100,
			TPMRemaining:   100000,
			HealthScore:    0.9,
		}); err != nil {
			t.Fatalf("Save snapshot %d returned error: %v", id, err)
		}
	}

	conversationRepo := conversations.NewSQLiteRepository(store.DB())
	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversationRepo)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"ping",
		"stream":true
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses stream status = %d, want %d", rec.Code, http.StatusOK)
	}

	runs, err := conversationRepo.ListRuns(1)
	if err != nil {
		t.Fatalf("ListRuns returned error: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("len(runs) = %d, want 2", len(runs))
	}
	if runs[0].Status != "capacity_failed" {
		t.Fatalf("runs[0].status = %q, want capacity_failed", runs[0].Status)
	}
	if runs[1].Status != "completed" {
		t.Fatalf("runs[1].status = %q, want completed", runs[1].Status)
	}
}

func TestResponsesHandlerRetrievesStoredResponseAndInputItems(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{"role": "assistant", "content": "pong"},
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     12,
				"completion_tokens": 4,
				"total_tokens":      16,
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
		ProviderType:      accounts.ProviderOpenAICompatible,
		AllowChatFallback: true,
		AccountName:       "ppchat",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           upstream.URL + "/v1",
		CredentialRef:     "sk-third-party",
		Status:            accounts.StatusActive,
		Priority:          100,
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
		t.Fatalf("Save returned error: %v", err)
	}

	conversationRepo := conversations.NewSQLiteRepository(store.DB())
	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversationRepo)

	createReq := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"ping"
	}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses status = %d, want %d", createRec.Code, http.StatusOK)
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("Unmarshal created returned error: %v", err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/responses/"+created.ID, nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /v1/responses/{id} status = %d, want %d", getRec.Code, http.StatusOK)
	}
	if !strings.Contains(getRec.Body.String(), `"output_text":"pong"`) {
		t.Fatalf("retrieved body = %s, want pong output_text", getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), `"total_tokens":16`) {
		t.Fatalf("retrieved body = %s, want usage total_tokens", getRec.Body.String())
	}

	inputReq := httptest.NewRequest(http.MethodGet, "/v1/responses/"+created.ID+"/input_items", nil)
	inputRec := httptest.NewRecorder()
	handler.ServeHTTP(inputRec, inputReq)
	if inputRec.Code != http.StatusOK {
		t.Fatalf("GET /v1/responses/{id}/input_items status = %d, want %d", inputRec.Code, http.StatusOK)
	}
	if !strings.Contains(inputRec.Body.String(), `"text":"ping"`) {
		t.Fatalf("input items body = %s, want ping input", inputRec.Body.String())
	}
	if strings.Contains(inputRec.Body.String(), `"text":"pong"`) {
		t.Fatalf("input items body = %s, should not include assistant output", inputRec.Body.String())
	}
}

func TestResponsesHandlerInputItemsPreserveRawFunctionCallOutput(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "pong"}},
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
		ProviderType:      accounts.ProviderOpenAICompatible,
		AllowChatFallback: true,
		AccountName:       "ppchat",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           upstream.URL + "/v1",
		CredentialRef:     "sk-third-party",
		Status:            accounts.StatusActive,
		Priority:          100,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	usageRepo := usage.NewSQLiteRepository(store.DB())
	if err := usageRepo.Save(usage.Snapshot{AccountID: 1, Balance: 100, QuotaRemaining: 100000, RPMRemaining: 100, TPMRemaining: 100000, HealthScore: 0.9}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()))
	createReq := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":[
			{"type":"function_call_output","call_id":"call_123","output":"tool result"},
			{"role":"user","content":[{"type":"input_text","text":"ping"}]}
		]
	}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses status = %d, want %d", createRec.Code, http.StatusOK)
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	inputReq := httptest.NewRequest(http.MethodGet, "/v1/responses/"+created.ID+"/input_items", nil)
	inputRec := httptest.NewRecorder()
	handler.ServeHTTP(inputRec, inputReq)
	if inputRec.Code != http.StatusOK {
		t.Fatalf("GET input_items status = %d, want %d", inputRec.Code, http.StatusOK)
	}
	body := inputRec.Body.String()
	if !strings.Contains(body, `"type":"function_call_output"`) || !strings.Contains(body, `"call_id":"call_123"`) {
		t.Fatalf("input items body = %s, want preserved function_call_output", body)
	}
}

func TestResponsesHandlerInputItemsSupportPaginationMetadata(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "pong"}},
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
		ProviderType:      accounts.ProviderOpenAICompatible,
		AllowChatFallback: true,
		AccountName:       "ppchat",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           upstream.URL + "/v1",
		CredentialRef:     "sk-third-party",
		Status:            accounts.StatusActive,
		Priority:          100,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	usageRepo := usage.NewSQLiteRepository(store.DB())
	if err := usageRepo.Save(usage.Snapshot{AccountID: 1, Balance: 100, QuotaRemaining: 100000, RPMRemaining: 100, TPMRemaining: 100000, HealthScore: 0.9}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	handler := api.NewResponsesHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()))
	createReq := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":[
			{"role":"user","content":[{"type":"input_text","text":"first"}]},
			{"role":"user","content":[{"type":"input_text","text":"second"}]},
			{"role":"user","content":[{"type":"input_text","text":"third"}]}
		]
	}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses status = %d, want %d", createRec.Code, http.StatusOK)
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}

	inputReq := httptest.NewRequest(http.MethodGet, "/v1/responses/"+created.ID+"/input_items?limit=2&order=desc", nil)
	inputRec := httptest.NewRecorder()
	handler.ServeHTTP(inputRec, inputReq)
	if inputRec.Code != http.StatusOK {
		t.Fatalf("GET input_items status = %d, want %d", inputRec.Code, http.StatusOK)
	}
	body := inputRec.Body.String()
	if !strings.Contains(body, `"has_more":true`) {
		t.Fatalf("input items body = %s, want has_more true", body)
	}
	if !strings.Contains(body, `"first_id":"msg_input_1_2"`) || !strings.Contains(body, `"last_id":"msg_input_1_1"`) {
		t.Fatalf("input items body = %s, want descending first/last ids", body)
	}
}

func TestResponsesHandlerDeletesResponse(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	handler := api.NewResponsesHandler(accounts.NewSQLiteRepository(store.DB()), usage.NewSQLiteRepository(store.DB()), conversations.NewSQLiteRepository(store.DB()))
	req := httptest.NewRequest(http.MethodDelete, "/v1/responses/resp_1_seq_2_123", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE /v1/responses/{id} status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"deleted":true`) {
		t.Fatalf("delete body = %s, want deleted true", rec.Body.String())
	}
}

func TestResponsesHandlerCancelsResponse(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	handler := api.NewResponsesHandler(accounts.NewSQLiteRepository(store.DB()), usage.NewSQLiteRepository(store.DB()), conversations.NewSQLiteRepository(store.DB()))
	req := httptest.NewRequest(http.MethodPost, "/v1/responses/resp_1_seq_2_123/cancel", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses/{id}/cancel status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"status":"cancelled"`) {
		t.Fatalf("cancel body = %s, want cancelled status", rec.Body.String())
	}
}

func TestResponsesHandlerEstimatesInputTokens(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	handler := api.NewResponsesHandler(accounts.NewSQLiteRepository(store.DB()), usage.NewSQLiteRepository(store.DB()), conversations.NewSQLiteRepository(store.DB()))
	req := httptest.NewRequest(http.MethodPost, "/v1/responses/input_tokens", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":[{"role":"user","content":[{"type":"input_text","text":"hello world"}]}]
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses/input_tokens status = %d, want %d", rec.Code, http.StatusOK)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	inputTokens, ok := payload["input_tokens"].(float64)
	if !ok || inputTokens <= 0 {
		t.Fatalf("input_tokens = %v, want positive number", payload["input_tokens"])
	}
}

func TestResponsesHandlerRetrievesModelByID(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	handler := api.NewResponsesHandler(accounts.NewSQLiteRepository(store.DB()), usage.NewSQLiteRepository(store.DB()), conversations.NewSQLiteRepository(store.DB()))
	req := httptest.NewRequest(http.MethodGet, "/v1/models/gpt-5.4", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/models/{id} status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"id":"gpt-5.4"`) {
		t.Fatalf("model body = %s, want gpt-5.4", rec.Body.String())
	}
}

func maybeRejectResponsesPathForCompatTests(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path == "/v1/responses" {
		http.Error(w, `{"error":{"message":"Invalid URL","type":"invalid_request_error"}}`, http.StatusNotFound)
		return true
	}
	return false
}
