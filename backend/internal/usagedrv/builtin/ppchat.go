package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gcssloop/codex-router/backend/internal/accountdrv"
	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/providers"
	"github.com/gcssloop/codex-router/backend/internal/usagedrv"
)

type PPChatDriver struct {
	client           *http.Client
	tokenLogsBaseURL string
}

func NewPPChatDriver(client *http.Client) *PPChatDriver {
	return &PPChatDriver{
		client:           client,
		tokenLogsBaseURL: "https://his.ppchat.vip",
	}
}

func (d *PPChatDriver) Name() string {
	return "builtin_ppchat"
}

func (d *PPChatDriver) Supports(account accounts.Account) bool {
	baseURL := strings.ToLower(strings.TrimSpace(account.BaseURL))
	return strings.Contains(baseURL, "ppchat.vip")
}

func (d *PPChatDriver) Fetch(ctx context.Context, account accounts.Account, credential accountdrv.ResolvedCredential) (usagedrv.RawUsageResult, error) {
	token := strings.TrimSpace(credential.AccessToken)
	if token == "" {
		token = strings.TrimSpace(credential.APIKey)
	}
	token = strings.TrimSpace(strings.TrimPrefix(token, "Bearer "))
	if token == "" {
		return usagedrv.RawUsageResult{}, &FetchError{Kind: FetchErrorKindAuth, Op: "build_usage_request", Err: fmt.Errorf("missing ppchat token")}
	}

	base, err := url.Parse(d.tokenLogsBaseURL)
	if err != nil {
		return usagedrv.RawUsageResult{}, &FetchError{Kind: FetchErrorKindUpstream, Op: "build_usage_request", Err: err}
	}
	base.Path = "/api/token-logs"
	base.RawQuery = url.Values{
		"token_key": []string{token},
		"page":      []string{"1"},
		"page_size": []string{"1"},
	}.Encode()
	base.Fragment = ""

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return usagedrv.RawUsageResult{}, &FetchError{Kind: FetchErrorKindUpstream, Op: "build_usage_request", Err: err}
	}
	client := d.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return usagedrv.RawUsageResult{}, &FetchError{Kind: FetchErrorKindUpstream, Op: "do_usage_request", Err: err}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return usagedrv.RawUsageResult{}, &FetchError{Kind: FetchErrorKindUpstream, Op: "read_usage_response", Err: err}
	}
	if resp.StatusCode >= 400 {
		return usagedrv.RawUsageResult{}, classifyPPChatStatusError(resp.StatusCode, raw)
	}

	result, ok := parsePPChatRawUsage(raw)
	if !ok {
		return usagedrv.RawUsageResult{}, &FetchError{Kind: FetchErrorKindUpstream, Op: "parse_usage_payload", Err: fmt.Errorf("invalid ppchat usage payload")}
	}
	return result, nil
}

func (d *PPChatDriver) SetTokenLogsBaseURLForTest(baseURL string) {
	d.tokenLogsBaseURL = strings.TrimSpace(baseURL)
}

func parsePPChatRawUsage(raw []byte) (usagedrv.RawUsageResult, bool) {
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			TokenInfo struct {
				RemainQuotaDisplay any `json:"remain_quota_display"`
			} `json:"token_info"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return usagedrv.RawUsageResult{}, false
	}
	if !payload.Success {
		return usagedrv.RawUsageResult{}, false
	}
	quota, ok := asFloatAny(payload.Data.TokenInfo.RemainQuotaDisplay)
	if !ok {
		return usagedrv.RawUsageResult{}, false
	}

	var anyPayload map[string]any
	_ = json.Unmarshal(raw, &anyPayload)
	return usagedrv.RawUsageResult{
		Source:     "remote",
		Confidence: "high",
		Payload:    anyPayload,
		Limits: usagedrv.UsageLimits{
			QuotaRemaining: floatPtr(quota),
		},
	}, true
}

func classifyPPChatStatusError(statusCode int, raw []byte) error {
	body := strings.TrimSpace(string(raw))
	switch {
	case statusCode == http.StatusUnauthorized:
		return &FetchError{Kind: FetchErrorKindAuth, Op: "usage_status", Err: fmt.Errorf("http status %d: %s", statusCode, body)}
	case statusCode == http.StatusForbidden && providers.LooksLikeInsufficientQuotaMessage(body):
		return &FetchError{Kind: FetchErrorKindQuota, Op: "usage_status", Err: fmt.Errorf("http status %d: %s", statusCode, body)}
	case providers.LooksLikeInsufficientQuotaMessage(body):
		return &FetchError{Kind: FetchErrorKindQuota, Op: "usage_status", Err: fmt.Errorf("http status %d: %s", statusCode, body)}
	default:
		return &FetchError{Kind: FetchErrorKindUpstream, Op: "usage_status", Err: fmt.Errorf("http status %d: %s", statusCode, body)}
	}
}

func asFloatAny(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int8:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case uint16:
		return float64(typed), true
	case uint8:
		return float64(typed), true
	case string:
		value := strings.TrimSpace(typed)
		if value == "" {
			return 0, false
		}
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}
