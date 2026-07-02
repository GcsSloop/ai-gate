package codex

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// Codex ChatGPT backend gates some models (e.g. gpt-5.5) behind minimum Codex versions.
	// Use a recent Codex CLI version string to avoid unnecessary model gating.
	codexClientVersion = "0.142.5"
	codexOriginator    = "codex_cli_rs"
	codexUserAgent     = "codex_cli_rs/" + codexClientVersion + " (ai-gate)"
)

type Adapter struct {
	baseURL string
}

func NewAdapter(baseURL string) *Adapter {
	return &Adapter{baseURL: strings.TrimRight(baseURL, "/")}
}

func (a *Adapter) BuildResponsesRequest(ctx context.Context, credential string, accountID string, body []byte, stream bool) (*http.Request, error) {
	return a.BuildResponsesEndpointRequest(ctx, credential, accountID, "/responses", body, stream)
}

func (a *Adapter) BuildModelsRequest(ctx context.Context, credential string, accountID string, endpointPath string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+endpointPath, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+credential)
	req.Header.Set("ChatGPT-Account-Id", accountID)
	req.Header.Set("originator", codexOriginator)
	req.Header.Set("User-Agent", codexUserAgent)
	req.Header.Set("session_id", "codex-router-"+strconvTimeID())
	return req, nil
}

func (a *Adapter) BuildResponsesEndpointRequest(ctx context.Context, credential string, accountID string, endpointPath string, body []byte, stream bool) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+endpointPath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	req.Header.Set("Authorization", "Bearer "+credential)
	req.Header.Set("ChatGPT-Account-Id", accountID)
	req.Header.Set("originator", codexOriginator)
	req.Header.Set("User-Agent", codexUserAgent)
	req.Header.Set("session_id", "codex-router-"+strconvTimeID())
	return req, nil
}

func (a *Adapter) BuildUsageRequest(ctx context.Context, credential string, accountID string) (*http.Request, error) {
	usageURL, err := a.buildUsageURL()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, usageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+credential)
	req.Header.Set("ChatGPT-Account-Id", accountID)
	req.Header.Set("originator", codexOriginator)
	req.Header.Set("User-Agent", codexUserAgent)
	return req, nil
}

func (a *Adapter) buildUsageURL() (string, error) {
	base, err := url.Parse(a.baseURL)
	if err != nil {
		return "", err
	}
	base.RawQuery = ""
	base.Fragment = ""

	if isChatGPTUsageBase(base) {
		base.Path = chatGPTBackendPath(base.Path) + "/wham/usage"
		return base.String(), nil
	}

	base.Path = strings.TrimRight(base.Path, "/") + "/api/codex/usage"
	return base.String(), nil
}

func isChatGPTUsageBase(base *url.URL) bool {
	host := strings.ToLower(base.Hostname())
	return host == "chatgpt.com" ||
		host == "chat.openai.com" ||
		strings.Contains(base.Path, "/backend-api")
}

func chatGPTBackendPath(path string) string {
	if idx := strings.Index(path, "/backend-api"); idx >= 0 {
		return path[:idx] + "/backend-api"
	}
	return "/backend-api"
}

func strconvTimeID() string {
	return time.Now().UTC().Format("20060102T150405.000000000")
}
