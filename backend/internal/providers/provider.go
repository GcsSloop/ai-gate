package providers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
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
