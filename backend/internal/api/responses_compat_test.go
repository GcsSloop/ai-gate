package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/api"
	"github.com/gcssloop/codex-router/backend/internal/conversations"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

func TestResponsesHandlerForwardsToolsToChatCompletions(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode returned error: %v", err)
		}
		tools, ok := payload["tools"].([]any)
		if !ok || len(tools) != 1 {
			t.Fatalf("tools = %#v, want one forwarded tool", payload["tools"])
		}
		toolChoice, ok := payload["tool_choice"].(map[string]any)
		if !ok || toolChoice["name"] != "grep" {
			t.Fatalf("tool_choice = %#v, want grep function choice", payload["tool_choice"])
		}
		parallel, ok := payload["parallel_tool_calls"].(bool)
		if !ok || !parallel {
			t.Fatalf("parallel_tool_calls = %#v, want true", payload["parallel_tool_calls"])
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "pong"}},
			},
			"usage": map[string]any{
				"prompt_tokens":     11,
				"completion_tokens": 5,
				"total_tokens":      16,
			},
		})
	}))
	defer upstream.Close()

	handler := newCompatResponsesHandler(t, upstream.URL+"/v1")

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"ping",
		"tools":[{"type":"function","function":{"name":"grep","description":"search","parameters":{"type":"object"}}}],
		"tool_choice":{"type":"function","name":"grep"},
		"parallel_tool_calls":true
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestResponsesHandlerBridgesToolItemsToChatCompletionsMessages(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []map[string]any `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode returned error: %v", err)
		}
		if len(payload.Messages) != 3 {
			t.Fatalf("messages = %#v, want user + assistant tool_calls + tool", payload.Messages)
		}
		if payload.Messages[0]["role"] != "user" {
			t.Fatalf("messages[0] = %#v, want user message", payload.Messages[0])
		}
		if payload.Messages[1]["role"] != "assistant" {
			t.Fatalf("messages[1] = %#v, want assistant tool_calls message", payload.Messages[1])
		}
		toolCalls, ok := payload.Messages[1]["tool_calls"].([]any)
		if !ok || len(toolCalls) != 1 {
			t.Fatalf("messages[1].tool_calls = %#v, want one tool call", payload.Messages[1]["tool_calls"])
		}
		if payload.Messages[2]["role"] != "tool" {
			t.Fatalf("messages[2] = %#v, want tool role message", payload.Messages[2])
		}
		if payload.Messages[2]["tool_call_id"] != "call_1" {
			t.Fatalf("messages[2].tool_call_id = %#v, want call_1", payload.Messages[2]["tool_call_id"])
		}
		if payload.Messages[2]["content"] != "{\"matches\":2}" {
			t.Fatalf("messages[2].content = %#v, want tool output", payload.Messages[2]["content"])
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "done"}},
			},
			"usage": map[string]any{
				"prompt_tokens":     20,
				"completion_tokens": 4,
				"total_tokens":      24,
			},
		})
	}))
	defer upstream.Close()

	handler := newCompatResponsesHandler(t, upstream.URL+"/v1")

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"run grep"}]},
			{"type":"function_call","call_id":"call_1","name":"grep","arguments":"{\"pattern\":\"TODO\"}"},
			{"type":"function_call_output","call_id":"call_1","output":"{\"matches\":2}"}
		]
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestResponsesHandlerModelDetailIncludesCapabilities(t *testing.T) {
	t.Parallel()

	handler := newCompatResponsesHandler(t, "https://example.invalid/v1")

	req := httptest.NewRequest(http.MethodGet, "/v1/models/gpt-5.4", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/models/{id} status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	for _, marker := range []string{
		`"context_window"`,
		`"supports_responses":true`,
		`"supports_streaming":true`,
		`"supports_tools":true`,
	} {
		if !strings.Contains(body, marker) {
			t.Fatalf("model detail = %s, want marker %q", body, marker)
		}
	}
}

func TestResponsesHandlerStreamsMultipleToolCalls(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"grep\",\"arguments\":\"{\\\"pattern\\\":\"}},{\"index\":1,\"id\":\"call_2\",\"type\":\"function\",\"function\":{\"name\":\"ls\",\"arguments\":\"{\\\"path\\\":\"}}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"TODO\\\"}\"}},{\"index\":1,\"function\":{\"arguments\":\"\\\"src\\\"}\"}}]}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	handler := newCompatResponsesHandler(t, upstream.URL+"/v1")

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"run tools",
		"stream":true
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses stream status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	for _, marker := range []string{
		`"call_id":"call_1"`,
		`"name":"grep"`,
		`"call_id":"call_2"`,
		`"name":"ls"`,
	} {
		if !strings.Contains(body, marker) {
			t.Fatalf("stream body = %s, want marker %q", body, marker)
		}
	}
}

func TestResponsesHandlerStreamsUsageFromChatCompletions(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode returned error: %v", err)
		}
		streamOptions, ok := payload["stream_options"].(map[string]any)
		if !ok || streamOptions["include_usage"] != true {
			t.Fatalf("stream_options = %#v, want include_usage=true", payload["stream_options"])
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"pong\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":5,\"total_tokens\":17}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	handler := newCompatResponsesHandler(t, upstream.URL+"/v1")

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
	for _, marker := range []string{
		`"input_tokens":12`,
		`"output_tokens":5`,
		`"total_tokens":17`,
	} {
		if !strings.Contains(body, marker) {
			t.Fatalf("stream body = %s, want marker %q", body, marker)
		}
	}
}

func TestResponsesHandlerRetrieveUsesMostRecentUsageSnapshot(t *testing.T) {
	t.Parallel()

	handler := newCompatResponsesHandler(t, "https://example.invalid/v1")

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"ping"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("POST /v1/responses status = %d, want %d when no real upstream exists", rec.Code, http.StatusBadGateway)
	}

	// Build a real response record in a fresh handler with two snapshots.
	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	if err := accountRepo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "older",
		AuthMode:      accounts.AuthModeAPIKey,
		BaseURL:       "https://example.invalid/v1",
		CredentialRef: "sk-1",
		Status:        accounts.StatusActive,
		Priority:      100,
	}); err != nil {
		t.Fatalf("Create older returned error: %v", err)
	}
	if err := accountRepo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "newer",
		AuthMode:      accounts.AuthModeAPIKey,
		BaseURL:       "https://example.invalid/v1",
		CredentialRef: "sk-2",
		Status:        accounts.StatusActive,
		Priority:      90,
	}); err != nil {
		t.Fatalf("Create newer returned error: %v", err)
	}
	usageRepo := usage.NewSQLiteRepository(store.DB())
	olderCheckedAt := mustParseTime(t, "2026-03-08T10:00:00Z")
	newerCheckedAt := mustParseTime(t, "2026-03-08T11:00:00Z")
	if err := usageRepo.Save(usage.Snapshot{AccountID: 1, LastInputTokens: 10, LastOutputTokens: 5, LastTotalTokens: 15, CheckedAt: olderCheckedAt}); err != nil {
		t.Fatalf("Save older returned error: %v", err)
	}
	if err := usageRepo.Save(usage.Snapshot{AccountID: 2, LastInputTokens: 21, LastOutputTokens: 8, LastTotalTokens: 29, CheckedAt: newerCheckedAt}); err != nil {
		t.Fatalf("Save newer returned error: %v", err)
	}
	conversationRepo := conversations.NewSQLiteRepository(store.DB())
	conversationID, err := conversationRepo.CreateConversation(conversations.Conversation{ClientID: "test", TargetProviderFamily: "codex-router", DefaultModel: "gpt-5.4", State: "active"})
	if err != nil {
		t.Fatalf("CreateConversation returned error: %v", err)
	}
	if err := conversationRepo.AppendMessage(conversations.Message{ConversationID: conversationID, Role: "user", Content: "ping", SequenceNo: 0}); err != nil {
		t.Fatalf("Append user returned error: %v", err)
	}
	if err := conversationRepo.AppendMessage(conversations.Message{ConversationID: conversationID, Role: "assistant", Content: "pong", SequenceNo: 1}); err != nil {
		t.Fatalf("Append assistant returned error: %v", err)
	}

	retrieveHandler := api.NewResponsesHandler(accountRepo, usageRepo, conversationRepo)
	retrieveReq := httptest.NewRequest(http.MethodGet, "/v1/responses/resp_1_seq_1_test", nil)
	retrieveRec := httptest.NewRecorder()
	retrieveHandler.ServeHTTP(retrieveRec, retrieveReq)
	if retrieveRec.Code != http.StatusOK {
		t.Fatalf("GET /v1/responses/{id} status = %d, want %d", retrieveRec.Code, http.StatusOK)
	}
	body := retrieveRec.Body.String()
	if !strings.Contains(body, `"input_tokens":21`) || !strings.Contains(body, `"output_tokens":8`) || !strings.Contains(body, `"total_tokens":29`) {
		t.Fatalf("retrieve body = %s, want newest usage snapshot", body)
	}
}

func newCompatResponsesHandler(t *testing.T, baseURL string) *api.ResponsesHandler {
	t.Helper()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	if err := accountRepo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "compat",
		AuthMode:      accounts.AuthModeAPIKey,
		BaseURL:       baseURL,
		CredentialRef: "sk-third-party",
		Status:        accounts.StatusActive,
		Priority:      100,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	if err := usageRepo.Save(usage.Snapshot{
		AccountID:          1,
		Balance:            100,
		QuotaRemaining:     100000,
		RPMRemaining:       100,
		TPMRemaining:       100000,
		HealthScore:        0.9,
		ModelContextWindow: 272000,
	}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	return api.NewResponsesHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()))
}

func mustParseTime(t *testing.T, value string) time.Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("time.Parse returned error: %v", err)
	}
	return parsed
}
