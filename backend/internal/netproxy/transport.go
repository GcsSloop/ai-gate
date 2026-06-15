package netproxy

import (
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

var systemProxyResolver = resolveSystemProxy

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
	if shouldSkipTLSVerify(t.repo) {
		return t.insecure.RoundTrip(req)
	}
	return t.strict.RoundTrip(req)
}

func shouldSkipTLSVerify(repo settingsReader) bool {
	if repo == nil {
		return false
	}
	appSettings, err := repo.GetAppSettings()
	return err == nil && appSettings.UpstreamSkipTLSVerify
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
	switch appSettings.UpstreamProxyMode {
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
