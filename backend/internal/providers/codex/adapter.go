package codex

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Adapter struct {
	baseURL string
}

func NewAdapter(baseURL string) *Adapter {
	return &Adapter{baseURL: strings.TrimRight(baseURL, "/")}
}

func (a *Adapter) BuildResponsesRequest(ctx context.Context, credential string, accountID string, body []byte, stream bool) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	req.Header.Set("Authorization", "Bearer "+credential)
	req.Header.Set("Origin", "https://chatgpt.com")
	req.Header.Set("Referer", "https://chatgpt.com/")
	req.Header.Set("ChatGPT-Account-Id", accountID)
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("Originator", "codex_cli_rs")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Codex/0.1.2507301649 Chrome/141.0.7390.65 Electron/37.2.6 Safari/537.36")
	req.Header.Set("Version", "0.21.0")
	req.Header.Set("Session_id", "codex-router-"+strconvTimeID())
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
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
	req.Header.Set("Origin", "https://chatgpt.com")
	req.Header.Set("Referer", "https://chatgpt.com/codex/settings/usage")
	req.Header.Set("ChatGPT-Account-Id", accountID)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Codex/0.1.2507301649 Chrome/141.0.7390.65 Electron/37.2.6 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	return req, nil
}

func strconvTimeID() string {
	return time.Now().UTC().Format("20060102T150405.000000000")
}
