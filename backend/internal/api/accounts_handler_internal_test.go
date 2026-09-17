package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/netproxy"
	"github.com/gcssloop/codex-router/backend/internal/settings"
)

func TestNewAccountsHandlerDefaultsToFifteenSecondRefreshTTL(t *testing.T) {
	t.Parallel()

	handler := NewAccountsHandler(nil, nil, nil, nil)
	if handler.refreshTTL != 15*time.Second {
		t.Fatalf("refreshTTL = %s, want %s", handler.refreshTTL, 15*time.Second)
	}
}

type accountsSettingsStub struct {
	value settings.AppSettings
}

func (s accountsSettingsStub) GetAppSettings() (settings.AppSettings, error) {
	return s.value, nil
}

type refresherStub struct {
	deadline time.Time
}

func (s *refresherStub) Run(ctx context.Context, _ time.Time) error {
	s.deadline, _ = ctx.Deadline()
	return nil
}

func TestAccountsHandlerRefreshUsesLatestSettingsTimeout(t *testing.T) {
	t.Parallel()

	current := settings.DefaultAppSettings()
	current.UsageRequestTimeoutSeconds = 3
	refresher := &refresherStub{}
	handler := NewAccountsHandler(nil, nil, nil, nil, WithAccountsUsageRefresher(refresher), WithAccountsSettings(accountsSettingsStub{value: current}))

	start := time.Now()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/accounts/usage/refresh", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("POST /accounts/usage/refresh status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if refresher.deadline.IsZero() {
		t.Fatal("refresh deadline was not captured")
	}
	remaining := time.Until(refresher.deadline)
	if remaining < 2*time.Second || remaining > 4*time.Second {
		t.Fatalf("remaining timeout = %s, want about 3s (start=%s)", remaining, start)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// doAccountRequest is the single place where per-account network options reach the shared upstream
// transport, so this pins that the proxy override survives the context hand-off.
func TestDoAccountRequestAppliesAccountProxyOverride(t *testing.T) {
	t.Parallel()

	restore := netproxy.SetSystemProxyResolverForTest(func(*http.Request) (*url.URL, error) {
		return url.Parse("http://127.0.0.1:7897")
	})
	defer restore()

	manual := accountsSettingsStub{value: settings.AppSettings{
		UpstreamProxyMode: settings.UpstreamProxyModeManual,
		UpstreamProxyURL:  "http://127.0.0.1:7890",
	}}
	direct := accountsSettingsStub{value: settings.AppSettings{
		UpstreamProxyMode: settings.UpstreamProxyModeDirect,
	}}

	resolvedProxy := func(t *testing.T, reader accountsSettingsStub, account accounts.Account) string {
		t.Helper()

		client := netproxy.NewHTTPClient(reader)
		client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			proxyURL, err := netproxy.ResolveProxy(req, reader)
			if err != nil {
				return nil, err
			}
			body := "none"
			if proxyURL != nil {
				body = proxyURL.String()
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		})

		req, err := http.NewRequest(http.MethodGet, "https://example.com/models", nil)
		if err != nil {
			t.Fatalf("NewRequest returned error: %v", err)
		}
		resp, err := doAccountRequest(client, req, account)
		if err != nil {
			t.Fatalf("doAccountRequest returned error: %v", err)
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		return string(raw)
	}

	cases := []struct {
		name   string
		reader accountsSettingsStub
		mode   accounts.ProxyMode
		want   string
	}{
		{name: "inherit follows the global manual proxy", reader: manual, mode: accounts.ProxyModeInherit, want: "http://127.0.0.1:7890"},
		{name: "direct override bypasses the global proxy", reader: manual, mode: accounts.ProxyModeDirect, want: "none"},
		{name: "proxy override reuses the global manual proxy", reader: manual, mode: accounts.ProxyModeProxy, want: "http://127.0.0.1:7890"},
		{name: "proxy override ignores a global direct mode", reader: direct, mode: accounts.ProxyModeProxy, want: "http://127.0.0.1:7897"},
		{name: "inherit follows the global direct mode", reader: direct, mode: accounts.ProxyModeInherit, want: "none"},
	}
	for _, testCase := range cases {
		testCase := testCase
		// Subtests stay sequential: the stubbed system proxy resolver is restored when this test
		// returns, and t.Parallel() would let them run after that restore.
		t.Run(testCase.name, func(t *testing.T) {
			account := accounts.Account{
				ProviderType: accounts.ProviderOpenAICompatible,
				AuthMode:     accounts.AuthModeAPIKey,
				ProxyMode:    testCase.mode,
			}
			if got := resolvedProxy(t, testCase.reader, account); got != testCase.want {
				t.Fatalf("resolved proxy = %q, want %q", got, testCase.want)
			}
		})
	}
}
