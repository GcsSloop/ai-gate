package codex

import (
	"context"
	"testing"
)

func TestAdapterBuildUsageRequestUsesLatestClientVersionAndChatGPTUsagePath(t *testing.T) {
	t.Parallel()

	req, err := NewAdapter("https://chatgpt.com/backend-api/codex").BuildUsageRequest(
		context.Background(),
		"token-1",
		"acct-1",
	)
	if err != nil {
		t.Fatalf("BuildUsageRequest returned error: %v", err)
	}
	if req.URL.String() != "https://chatgpt.com/backend-api/wham/usage" {
		t.Fatalf("url = %q, want ChatGPT WHAM usage endpoint", req.URL.String())
	}
	if got := req.Header.Get("User-Agent"); got != "codex_cli_rs/0.142.5 (ai-gate)" {
		t.Fatalf("User-Agent = %q, want latest Codex client version", got)
	}
}

func TestAdapterBuildUsageRequestUsesCodexAPIPathForNonChatGPTBase(t *testing.T) {
	t.Parallel()

	req, err := NewAdapter("https://codex.example.test/root").BuildUsageRequest(
		context.Background(),
		"token-1",
		"acct-1",
	)
	if err != nil {
		t.Fatalf("BuildUsageRequest returned error: %v", err)
	}
	if req.URL.String() != "https://codex.example.test/root/api/codex/usage" {
		t.Fatalf("url = %q, want Codex API usage endpoint", req.URL.String())
	}
}
