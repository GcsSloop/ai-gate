package providers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

var ErrInsufficientQuota = errors.New("insufficient quota")

type ErrorClass string

const (
	ErrorClassHard      ErrorClass = "hard"
	ErrorClassSoft      ErrorClass = "soft"
	ErrorClassCapacity  ErrorClass = "capacity"
	ErrorClassRateLimit ErrorClass = "rate_limit"
)

type Request struct {
	Path   string
	Method string
	APIKey string
	Body   []byte
}

type Capabilities struct {
	SupportsChatCompletions bool
	SupportsStreaming       bool
}

type Adapter interface {
	BuildRequest(ctx context.Context, req Request) (*http.Request, error)
	Capabilities() Capabilities
	ClassifyError(err error) ErrorClass
}

type HTTPError struct {
	StatusCode int
}

func (e HTTPError) Error() string {
	return fmt.Sprintf("http status %d", e.StatusCode)
}

func LooksLikeInsufficientQuotaMessage(value string) bool {
	body := strings.ToLower(strings.Join(strings.Fields(value), " "))
	if body == "" {
		return false
	}
	for _, marker := range []string{
		"insufficient quota",
		"quota exceeded",
		"quota exhausted",
		"usage limit",
		"purchase more credits",
		"upgrade to pro",
		"upgrade to plus",
		"credits exhausted",
		"balance is not enough",
		"余额不足",
		"额度不足",
	} {
		if strings.Contains(body, marker) {
			return true
		}
	}
	return false
}

func IsInsufficientQuotaError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrInsufficientQuota) {
		return true
	}
	return LooksLikeInsufficientQuotaMessage(err.Error())
}

func NewJSONRequest(ctx context.Context, method, url, apiKey string, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	return req, nil
}
