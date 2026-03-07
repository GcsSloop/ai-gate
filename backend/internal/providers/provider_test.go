package providers_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/providers"
	"github.com/gcssloop/codex-router/backend/internal/providers/openai"
)

func TestOpenAIAdapterBuildRequest(t *testing.T) {
	t.Parallel()

	adapter := openai.NewAdapter("https://api.example.test")

	req, err := adapter.BuildRequest(context.Background(), providers.Request{
		Path:   "/v1/chat/completions",
		Method: http.MethodPost,
		APIKey: "test-key",
		Body:   []byte(`{"model":"gpt-4.1"}`),
	})
	if err != nil {
		t.Fatalf("BuildRequest returned error: %v", err)
	}

	if req.URL.String() != "https://api.example.test/v1/chat/completions" {
		t.Fatalf("request url = %q, want %q", req.URL.String(), "https://api.example.test/v1/chat/completions")
	}
	if req.Header.Get("Authorization") != "Bearer test-key" {
		t.Fatalf("Authorization header = %q, want %q", req.Header.Get("Authorization"), "Bearer test-key")
	}
}

func TestOpenAIAdapterCapabilities(t *testing.T) {
	t.Parallel()

	adapter := openai.NewAdapter("https://api.example.test")
	capabilities := adapter.Capabilities()

	if !capabilities.SupportsChatCompletions {
		t.Fatal("SupportsChatCompletions = false, want true")
	}
	if !capabilities.SupportsStreaming {
		t.Fatal("SupportsStreaming = false, want true")
	}
}

func TestOpenAIAdapterClassifyError(t *testing.T) {
	t.Parallel()

	adapter := openai.NewAdapter("https://api.example.test")

	tests := []struct {
		name string
		err  error
		want providers.ErrorClass
	}{
		{name: "rate limit", err: providers.HTTPError{StatusCode: http.StatusTooManyRequests}, want: providers.ErrorClassRateLimit},
		{name: "unauthorized", err: providers.HTTPError{StatusCode: http.StatusUnauthorized}, want: providers.ErrorClassHard},
		{name: "server error", err: providers.HTTPError{StatusCode: http.StatusBadGateway}, want: providers.ErrorClassSoft},
		{name: "capacity", err: providers.ErrInsufficientQuota, want: providers.ErrorClassCapacity},
		{name: "generic", err: errors.New("boom"), want: providers.ErrorClassSoft},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := adapter.ClassifyError(tt.err); got != tt.want {
				t.Fatalf("ClassifyError() = %q, want %q", got, tt.want)
			}
		})
	}
}
