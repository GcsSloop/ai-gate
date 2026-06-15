package netproxy_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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

type mutableSettingsReader struct {
	value settings.AppSettings
}

func (s *mutableSettingsReader) GetAppSettings() (settings.AppSettings, error) {
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

func TestNewHTTPClientUsesStrictTLSVerificationByDefault(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer server.Close()

	client := netproxy.NewHTTPClient(stubSettingsReader{value: settings.DefaultAppSettings()})
	resp, err := client.Get(server.URL)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("client.Get succeeded, want TLS verification error by default")
	}
	if !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("client.Get error = %v, want certificate verification error", err)
	}
}

func TestNewHTTPClientCanSkipUpstreamTLSVerification(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer server.Close()

	client := netproxy.NewHTTPClient(stubSettingsReader{value: settings.DefaultAppSettings()})
	ctx := netproxy.ContextWithSkipTLSVerify(context.Background(), true)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext returned error: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Get returned error: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q, want ok", string(body))
	}
}

func TestNewHTTPClientAppliesTLSVerificationContextPerRequest(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer server.Close()

	client := netproxy.NewHTTPClient(stubSettingsReader{value: settings.DefaultAppSettings()})

	resp, err := client.Get(server.URL)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("client.Get succeeded, want TLS verification error before enabling bypass")
	}

	ctx := netproxy.ContextWithSkipTLSVerify(context.Background(), true)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext returned error: %v", err)
	}
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("client.Get returned error after enabling bypass: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q, want ok", string(body))
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

func TestResolveProxyUsesSystemProxyResolver(t *testing.T) {
	t.Parallel()

	restore := netproxy.SetSystemProxyResolverForTest(func(req *http.Request) (*url.URL, error) {
		return url.Parse("http://127.0.0.1:7897")
	})
	defer restore()

	reader := stubSettingsReader{value: settings.AppSettings{
		UpstreamProxyMode: settings.UpstreamProxyModeSystem,
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
		t.Fatal("ResolveProxy = nil, want system proxy URL")
	}
	if got.String() != "http://127.0.0.1:7897" {
		t.Fatalf("ResolveProxy = %q, want %q", got.String(), "http://127.0.0.1:7897")
	}
}

func TestResolveProxySystemModeFallsBackToEnvironment(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:8888")
	restore := netproxy.SetSystemProxyResolverForTest(func(req *http.Request) (*url.URL, error) {
		return nil, nil
	})
	defer restore()

	reader := stubSettingsReader{value: settings.AppSettings{
		UpstreamProxyMode: settings.UpstreamProxyModeSystem,
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
		t.Fatal("ResolveProxy = nil, want environment proxy URL")
	}
	if got.String() != "http://127.0.0.1:8888" {
		t.Fatalf("ResolveProxy = %q, want %q", got.String(), "http://127.0.0.1:8888")
	}
}
