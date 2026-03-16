package openai_test

import (
	"strings"
	"testing"

	gatewayopenai "github.com/gcssloop/codex-router/backend/internal/gateway/openai"
)

func TestParseChatCompletionRequest(t *testing.T) {
	t.Parallel()

	req, err := gatewayopenai.ParseChatCompletionRequest(strings.NewReader(`{
		"model":"gpt-4.1",
		"stream":true,
		"messages":[
			{"role":"system","content":"be precise"},
			{"role":"user","content":"hello"}
		]
	}`))
	if err != nil {
		t.Fatalf("ParseChatCompletionRequest returned error: %v", err)
	}

	if req.Model != "gpt-4.1" {
		t.Fatalf("Model = %q, want %q", req.Model, "gpt-4.1")
	}
	if !req.Stream {
		t.Fatal("Stream = false, want true")
	}
	if len(req.Messages) != 2 {
		t.Fatalf("Messages len = %d, want 2", len(req.Messages))
	}
}

func TestParseChatCompletionRequestRejectsMalformedPayload(t *testing.T) {
	t.Parallel()

	_, err := gatewayopenai.ParseChatCompletionRequest(strings.NewReader(`{"model":1}`))
	if err == nil {
		t.Fatal("ParseChatCompletionRequest returned nil error, want parse error")
	}
}

func TestParseChatCompletionRequestAcceptsArrayContent(t *testing.T) {
	t.Parallel()

	req, err := gatewayopenai.ParseChatCompletionRequest(strings.NewReader(`{
		"model":"gpt-4.1",
		"stream":false,
		"messages":[
			{
				"role":"user",
				"content":[
					{"type":"text","text":"hello"},
					{"type":"text","text":"world"}
				]
			}
		]
	}`))
	if err != nil {
		t.Fatalf("ParseChatCompletionRequest returned error: %v", err)
	}
	if req.Model != "gpt-4.1" {
		t.Fatalf("Model = %q, want %q", req.Model, "gpt-4.1")
	}
	if req.Stream {
		t.Fatal("Stream = true, want false")
	}
	if len(req.Messages) != 1 {
		t.Fatalf("Messages len = %d, want 1", len(req.Messages))
	}
	if got := strings.TrimSpace(string(req.Messages[0].Content)); got == "" || got[0] != '[' {
		t.Fatalf("Content raw = %q, want raw array payload", got)
	}
}
