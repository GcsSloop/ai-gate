package netproxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/settings"
)

type settingsReader interface {
	GetAppSettings() (settings.AppSettings, error)
}

type skipTLSVerifyContextKey struct{}

type proxyOverrideContextKey struct{}

// ProxyOverride is a per-account override of the global upstream proxy setting.
type ProxyOverride string

const (
	// ProxyOverrideDirect never uses a proxy, ignoring the global setting.
	ProxyOverrideDirect ProxyOverride = "direct"
	// ProxyOverrideProxy always uses a proxy, reusing the globally configured address.
	ProxyOverrideProxy ProxyOverride = "proxy"
)

var systemProxyResolver = resolveSystemProxy

func ContextWithSkipTLSVerify(ctx context.Context, skip bool) context.Context {
	return context.WithValue(ctx, skipTLSVerifyContextKey{}, skip)
}

// ContextWithProxyOverride scopes the global upstream proxy setting to one request, which is how a
// per-account override reaches the shared transport.
func ContextWithProxyOverride(ctx context.Context, override ProxyOverride) context.Context {
	if override != ProxyOverrideDirect && override != ProxyOverrideProxy {
		return ctx
	}
	return context.WithValue(ctx, proxyOverrideContextKey{}, override)
}

// ProxyOverrideForAccountMode translates a stored account proxy mode into an override. Accounts
// persist "direct" or "proxy" for an explicit override and the empty string to inherit; anything
// else inherits, matching accounts.NormalizeProxyMode.
func ProxyOverrideForAccountMode(mode string) ProxyOverride {
	switch ProxyOverride(mode) {
	case ProxyOverrideDirect:
		return ProxyOverrideDirect
	case ProxyOverrideProxy:
		return ProxyOverrideProxy
	default:
		return ""
	}
}

func SetSystemProxyResolverForTest(resolver func(*http.Request) (*url.URL, error)) func() {
	previous := systemProxyResolver
	if resolver == nil {
		systemProxyResolver = resolveSystemProxy
	} else {
		systemProxyResolver = resolver
	}
	return func() {
		systemProxyResolver = previous
	}
}

func NewHTTPClient(repo settingsReader) *http.Client {
	return &http.Client{Transport: newSettingsTransport(repo)}
}

type settingsTransport struct {
	repo     settingsReader
	strict   *http.Transport
	insecure *http.Transport
}

func newSettingsTransport(repo settingsReader) http.RoundTripper {
	strict := newTransport(repo)
	insecure := newTransport(repo)
	tlsConfig := insecure.TLSClientConfig
	if tlsConfig == nil {
		tlsConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	} else {
		tlsConfig = tlsConfig.Clone()
		if tlsConfig.MinVersion == 0 {
			tlsConfig.MinVersion = tls.VersionTLS12
		}
	}
	tlsConfig.InsecureSkipVerify = true
	insecure.TLSClientConfig = tlsConfig
	return &settingsTransport{
		repo:     repo,
		strict:   strict,
		insecure: insecure,
	}
}

func newTransport(repo settingsReader) *http.Transport {
	transport := defaultTransportClone()
	transport.Proxy = func(req *http.Request) (*url.URL, error) {
		return ResolveProxy(req, repo)
	}
	return transport
}

func (t *settingsTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if shouldSkipTLSVerify(req) {
		return t.insecure.RoundTrip(req)
	}
	return t.strict.RoundTrip(req)
}

func shouldSkipTLSVerify(req *http.Request) bool {
	if req == nil {
		return false
	}
	value, _ := req.Context().Value(skipTLSVerifyContextKey{}).(bool)
	return value
}

func ResolveProxy(req *http.Request, repo settingsReader) (*url.URL, error) {
	if req == nil {
		return nil, nil
	}
	if repo == nil {
		return http.ProxyFromEnvironment(req)
	}
	appSettings, err := repo.GetAppSettings()
	if err != nil {
		return nil, fmt.Errorf("load app settings: %w", err)
	}

	mode := appSettings.UpstreamProxyMode
	if override := proxyOverride(req); override != "" {
		if override == ProxyOverrideDirect {
			return nil, nil
		}
		// Forced proxy: reuse the global address when there is one, otherwise fall back to the
		// system/env proxy so a single account can still be routed through a proxy.
		mode = settings.UpstreamProxyModeManual
		if appSettings.UpstreamProxyMode != settings.UpstreamProxyModeManual {
			mode = settings.UpstreamProxyModeSystem
		}
	}

	switch mode {
	case settings.UpstreamProxyModeDirect:
		return nil, nil
	case settings.UpstreamProxyModeManual:
		proxyURL, err := parseManualProxy(appSettings)
		if err != nil {
			return nil, err
		}
		return proxyURL, nil
	case "", settings.UpstreamProxyModeSystem:
		proxyURL, err := systemProxyResolver(req)
		if err != nil {
			return nil, err
		}
		if proxyURL != nil {
			return proxyURL, nil
		}
		return http.ProxyFromEnvironment(req)
	default:
		return http.ProxyFromEnvironment(req)
	}
}

func proxyOverride(req *http.Request) ProxyOverride {
	if req == nil {
		return ""
	}
	override, _ := req.Context().Value(proxyOverrideContextKey{}).(ProxyOverride)
	return override
}

func parseManualProxy(appSettings settings.AppSettings) (*url.URL, error) {
	rawURL := strings.TrimSpace(appSettings.UpstreamProxyURL)
	if rawURL == "" {
		return nil, fmt.Errorf("manual upstream proxy url is empty")
	}
	proxyURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse upstream proxy url: %w", err)
	}
	if proxyURL.Scheme == "" || proxyURL.Host == "" {
		return nil, fmt.Errorf("upstream proxy url must include scheme and host")
	}
	if proxyURL.User == nil && strings.TrimSpace(appSettings.UpstreamProxyUsername) != "" {
		proxyURL.User = url.UserPassword(strings.TrimSpace(appSettings.UpstreamProxyUsername), strings.TrimSpace(appSettings.UpstreamProxyPassword))
	}
	return proxyURL, nil
}

func defaultTransportClone() *http.Transport {
	base, ok := http.DefaultTransport.(*http.Transport)
	if ok && base != nil {
		return base.Clone()
	}
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}
