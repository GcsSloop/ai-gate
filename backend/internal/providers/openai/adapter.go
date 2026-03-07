package openai

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gcssloop/codex-router/backend/internal/providers"
)

type Adapter struct {
	baseURL string
}

func NewAdapter(baseURL string) *Adapter {
	return &Adapter{baseURL: strings.TrimRight(baseURL, "/")}
}

func (a *Adapter) BuildRequest(ctx context.Context, req providers.Request) (*http.Request, error) {
	return providers.NewJSONRequest(ctx, req.Method, a.baseURL+req.Path, req.APIKey, req.Body)
}

func (a *Adapter) Capabilities() providers.Capabilities {
	return providers.Capabilities{
		SupportsChatCompletions: true,
		SupportsStreaming:       true,
	}
}

func (a *Adapter) ClassifyError(err error) providers.ErrorClass {
	if errors.Is(err, providers.ErrInsufficientQuota) {
		return providers.ErrorClassCapacity
	}

	var httpErr providers.HTTPError
	if errors.As(err, &httpErr) {
		switch {
		case httpErr.StatusCode == http.StatusTooManyRequests:
			return providers.ErrorClassRateLimit
		case httpErr.StatusCode == http.StatusUnauthorized || httpErr.StatusCode == http.StatusForbidden:
			return providers.ErrorClassHard
		case httpErr.StatusCode >= http.StatusBadGateway:
			return providers.ErrorClassSoft
		default:
			return providers.ErrorClassSoft
		}
	}

	return providers.ErrorClassSoft
}
