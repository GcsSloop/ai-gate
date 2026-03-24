package netproxy_test

import (
	"net/http"
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/netproxy"
	"github.com/gcssloop/codex-router/backend/internal/settings"
)

type stubSettingsReader struct {
	value settings.AppSettings
}

func (s stubSettingsReader) GetAppSettings() (settings.AppSettings, error) {
	return s.value, nil
}

func TestResolveProxyUsesDirectMode(t *testing.T) {
	t.Parallel()

	reader := stubSettingsReader{value: settings.AppSettings{
		UpstreamProxyMode: "direct",
	}}
	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}

	got, err := netproxy.ResolveProxy(req, reader)
	if err != nil {
		t.Fatalf("ResolveProxy returned error: %v", err)
	}
	if got != nil {
		t.Fatalf("ResolveProxy = %v, want nil in direct mode", got)
	}
}

func TestResolveProxyUsesManualMode(t *testing.T) {
	t.Parallel()

	reader := stubSettingsReader{value: settings.AppSettings{
		UpstreamProxyMode:     "manual",
		UpstreamProxyURL:      "http://127.0.0.1:7890",
		UpstreamProxyUsername: "user",
		UpstreamProxyPassword: "pass",
	}}
	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}

	got, err := netproxy.ResolveProxy(req, reader)
	if err != nil {
		t.Fatalf("ResolveProxy returned error: %v", err)
	}
	if got == nil {
		t.Fatal("ResolveProxy = nil, want manual proxy URL")
	}
	if got.String() != "http://user:pass@127.0.0.1:7890" {
		t.Fatalf("ResolveProxy = %q, want %q", got.String(), "http://user:pass@127.0.0.1:7890")
	}
}
