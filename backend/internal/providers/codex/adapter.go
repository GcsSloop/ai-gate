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
	codexClientVersion = "0.125.0"
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
	base, err := url.Parse(a.baseURL)
	if err != nil {
		return nil, err
	}
	base.Path = "/backend-api/wham/usage"
	base.RawQuery = ""
	base.Fragment = ""

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
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

func strconvTimeID() string {
	return time.Now().UTC().Format("20060102T150405.000000000")
}
