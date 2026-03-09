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
		if maybeRejectResponsesPath(w, r) {
			return
		}
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

func TestResponsesHandlerRejectsMCPToolsOnChatFallback(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if maybeRejectResponsesPath(w, r) {
			return
		}
		if r.URL.Path == "/v1/chat/completions" {
			t.Fatal("chat/completions should not be called when tools include mcp")
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	handler := newCompatResponsesHandler(t, upstream.URL+"/v1")

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"使用 obsidian mcp 搜索所有张家港会议纪要",
		"tools":[{"type":"mcp","server_label":"obsidian","server_url":"http://127.0.0.1:27123/sse"}]
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("POST /v1/responses status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	if !strings.Contains(rec.Body.String(), "chat fallback does not support mcp tools") {
		t.Fatalf("response body = %s, want explicit mcp fallback error", rec.Body.String())
	}
}

func TestResponsesHandlerPrefersResponsesForCompatibleProvider(t *testing.T) {
	t.Parallel()

	responsesCalls := 0
	chatCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/responses":
			responsesCalls++
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("Decode /responses payload error: %v", err)
			}
			if payload["model"] == nil {
				t.Fatalf("responses payload missing model: %#v", payload)
			}
			store, exists := payload["store"]
			if !exists {
				t.Fatalf("responses payload missing store field")
			}
			storeBool, ok := store.(bool)
			if !ok || storeBool {
				t.Fatalf("responses payload store = %#v, want false", payload["store"])
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "resp_test_1",
				"status":      "completed",
				"model":       "gpt-5.4",
				"output_text": "pong",
				"output": []map[string]any{
					{
						"id":     "msg_1",
						"type":   "message",
						"status": "completed",
						"role":   "assistant",
						"content": []map[string]any{
							{"type": "output_text", "text": "pong", "annotations": []any{}},
						},
					},
				},
				"usage": map[string]any{
					"input_tokens":  12,
					"output_tokens": 4,
					"total_tokens":  16,
				},
			})
		case "/v1/chat/completions":
			chatCalls++
			t.Fatalf("chat/completions should not be called when /responses is available")
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	handler := newCompatResponsesHandler(t, upstream.URL+"/v1")

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"ping"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if responsesCalls == 0 {
		t.Fatal("expected /responses to be called at least once")
	}
	if chatCalls != 0 {
		t.Fatalf("chat/completions call count = %d, want 0", chatCalls)
	}
}

func TestResponsesHandlerRejectsStoreParameter(t *testing.T) {
	t.Parallel()

	handler := newCompatResponsesHandler(t, "https://example.invalid/v1")
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"ping",
		"store":true
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /v1/responses status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "store is not supported") {
		t.Fatalf("response body = %s, want store unsupported error", rec.Body.String())
	}
}

func TestResponsesHandlerCompatibleResponsesStreamClosedDoesNotFallbackToChat(t *testing.T) {
	t.Parallel()

	chatCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/responses":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"))
		case "/v1/chat/completions":
			chatCalls++
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"fallback-ok\"}}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		default:
			http.NotFound(w, r)
		}
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
		t.Fatalf("POST /v1/responses stream status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"type":"error"`) {
		t.Fatalf("stream body missing error event: %s", body)
	}
	if !strings.Contains(body, `"status":"failed"`) {
		t.Fatalf("stream body missing failed status: %s", body)
	}
	if !strings.Contains(strings.ToLower(body), "stream closed before response.completed") {
		t.Fatalf("stream body = %s, want stream closed before response.completed", body)
	}
	if chatCalls != 0 {
		t.Fatalf("chat/completions call count = %d, want 0", chatCalls)
	}
}

func TestResponsesHandlerForwardsInstructionsToChatCompletions(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if maybeRejectResponsesPath(w, r) {
			return
		}
		var payload struct {
			Messages []map[string]any `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode returned error: %v", err)
		}
		if len(payload.Messages) < 2 {
			t.Fatalf("messages = %#v, want system + user at least", payload.Messages)
		}
		if payload.Messages[0]["role"] != "system" {
			t.Fatalf("messages[0] = %#v, want system instruction", payload.Messages[0])
		}
		if payload.Messages[0]["content"] != "be precise and use tools" {
			t.Fatalf("messages[0].content = %#v, want forwarded instructions", payload.Messages[0]["content"])
		}
		if payload.Messages[1]["role"] != "user" {
			t.Fatalf("messages[1] = %#v, want user message", payload.Messages[1])
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "pong"}},
			},
		})
	}))
	defer upstream.Close()

	handler := newCompatResponsesHandler(t, upstream.URL+"/v1")

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"ping",
		"instructions":"be precise and use tools"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestResponsesHandlerForwardsReasoningIncludeAndResponseFormatToChatCompletions(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if maybeRejectResponsesPath(w, r) {
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode returned error: %v", err)
		}
		reasoning, ok := payload["reasoning"].(map[string]any)
		if !ok || reasoning["effort"] != "high" {
			t.Fatalf("reasoning = %#v, want effort=high", payload["reasoning"])
		}
		if payload["reasoning_effort"] != "high" {
			t.Fatalf("reasoning_effort = %#v, want high", payload["reasoning_effort"])
		}
		include, ok := payload["include"].([]any)
		if !ok || len(include) != 1 || include[0] != "reasoning.encrypted_content" {
			t.Fatalf("include = %#v, want reasoning include passthrough", payload["include"])
		}
		responseFormat, ok := payload["response_format"].(map[string]any)
		if !ok || responseFormat["type"] != "json_object" {
			t.Fatalf("response_format = %#v, want {type:json_object}", payload["response_format"])
		}
		if payload["max_completion_tokens"] != float64(1024) {
			t.Fatalf("max_completion_tokens = %#v, want 1024", payload["max_completion_tokens"])
		}
		if payload["max_tokens"] != float64(1024) {
			t.Fatalf("max_tokens = %#v, want 1024", payload["max_tokens"])
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "pong"}},
			},
		})
	}))
	defer upstream.Close()

	handler := newCompatResponsesHandler(t, upstream.URL+"/v1")
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"ping",
		"reasoning":{"effort":"high"},
		"include":["reasoning.encrypted_content"],
		"text":{"format":{"type":"json_object"}},
		"max_output_tokens":1024
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/responses status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestResponsesHandlerBridgesToolItemsToChatCompletionsMessages(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if maybeRejectResponsesPath(w, r) {
			return
		}
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
		if maybeRejectResponsesPath(w, r) {
			return
		}
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
		if maybeRejectResponsesPath(w, r) {
			return
		}
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
		ProviderType:      accounts.ProviderOpenAICompatible,
		AllowChatFallback: true,
		AccountName:       "older",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           "https://example.invalid/v1",
		CredentialRef:     "sk-1",
		Status:            accounts.StatusActive,
		Priority:          100,
	}); err != nil {
		t.Fatalf("Create older returned error: %v", err)
	}
	if err := accountRepo.Create(accounts.Account{
		ProviderType:      accounts.ProviderOpenAICompatible,
		AllowChatFallback: true,
		AccountName:       "newer",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           "https://example.invalid/v1",
		CredentialRef:     "sk-2",
		Status:            accounts.StatusActive,
		Priority:          90,
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

func maybeRejectResponsesPath(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path == "/v1/responses" {
		http.Error(w, `{"error":{"message":"Invalid URL","type":"invalid_request_error"}}`, http.StatusNotFound)
		return true
	}
	return false
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
		ProviderType:      accounts.ProviderOpenAICompatible,
		AllowChatFallback: true,
		AccountName:       "compat",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           baseURL,
		CredentialRef:     "sk-third-party",
		Status:            accounts.StatusActive,
		Priority:          100,
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
