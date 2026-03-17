package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accountdrv"
	"github.com/gcssloop/codex-router/backend/internal/accounts"
	providercodex "github.com/gcssloop/codex-router/backend/internal/providers/codex"
	"github.com/gcssloop/codex-router/backend/internal/usagedrv"
)

const defaultOfficialCodexBaseURL = "https://chatgpt.com/backend-api/codex"

type FetchErrorKind string

const (
	FetchErrorKindAuth     FetchErrorKind = "auth"
	FetchErrorKindQuota    FetchErrorKind = "quota"
	FetchErrorKindUpstream FetchErrorKind = "upstream"
)

type FetchError struct {
	Kind FetchErrorKind
	Op   string
	Err  error
}

func (e *FetchError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%s: %v", e.Op, e.Err)
}

func (e *FetchError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type OpenAIOfficialDriver struct {
	client *http.Client
}

func NewOpenAIOfficialDriver(client *http.Client) *OpenAIOfficialDriver {
	return &OpenAIOfficialDriver{client: client}
}

func (d *OpenAIOfficialDriver) Name() string {
	return "builtin_openai_official"
}

func (d *OpenAIOfficialDriver) Supports(account accounts.Account) bool {
	return account.ProviderType == accounts.ProviderOpenAIOfficial
}

func (d *OpenAIOfficialDriver) Fetch(ctx context.Context, account accounts.Account, credential accountdrv.ResolvedCredential) (usagedrv.RawUsageResult, error) {
	token := strings.TrimSpace(credential.AccessToken)
	if token == "" {
		token = strings.TrimSpace(credential.APIKey)
	}
	if token == "" {
		return usagedrv.RawUsageResult{}, &FetchError{Kind: FetchErrorKindAuth, Op: "build_usage_request", Err: fmt.Errorf("missing access token")}
	}
	accountID, _ := credential.Metadata["account_id"].(string)
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return usagedrv.RawUsageResult{}, &FetchError{Kind: FetchErrorKindAuth, Op: "build_usage_request", Err: fmt.Errorf("missing account_id metadata")}
	}

	baseURL := strings.TrimSpace(account.BaseURL)
	if baseURL == "" {
		baseURL = defaultOfficialCodexBaseURL
	}
	req, err := providercodex.NewAdapter(baseURL).BuildUsageRequest(ctx, token, accountID)
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
		return usagedrv.RawUsageResult{}, classifyOfficialStatusError(resp.StatusCode, raw)
	}

	result, ok := parseOfficialRawUsage(raw)
	if !ok {
		return usagedrv.RawUsageResult{}, &FetchError{
			Kind: FetchErrorKindUpstream,
			Op:   "parse_usage_payload",
			Err:  fmt.Errorf("invalid official usage payload"),
		}
	}
	return result, nil
}

func parseOfficialRawUsage(raw []byte) (usagedrv.RawUsageResult, bool) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return usagedrv.RawUsageResult{}, false
	}

	rateLimit, _ := payload["rate_limit"].(map[string]any)
	primary, _ := rateLimit["primary_window"].(map[string]any)
	secondary, _ := rateLimit["secondary_window"].(map[string]any)
	credits, _ := payload["credits"].(map[string]any)

	primaryUsed := asFloat(primary["used_percent"])
	secondaryUsed := asFloat(secondary["used_percent"])
	balance := asFloat(credits["balance"])
	allowed, _ := rateLimit["allowed"].(bool)
	limitReached, _ := rateLimit["limit_reached"].(bool)
	hasCredits, _ := credits["has_credits"].(bool)
	unlimited, _ := credits["unlimited"].(bool)

	if primaryUsed == 0 && secondaryUsed == 0 && balance == 0 && !allowed && !hasCredits && !unlimited {
		return usagedrv.RawUsageResult{}, false
	}

	primaryRemaining := maxFloat(100-primaryUsed, 0)
	secondaryRemaining := maxFloat(100-secondaryUsed, 0)
	primaryResetsAt := unixSecondsPtr(int64(asFloat(primary["reset_at"])))
	secondaryResetsAt := unixSecondsPtr(int64(asFloat(secondary["reset_at"])))

	return usagedrv.RawUsageResult{
		Source:     "remote",
		Confidence: "high",
		Payload:    payload,
		Limits: usagedrv.UsageLimits{
			Balance:              floatPtr(balance),
			RPMRemaining:         floatPtr(primaryRemaining),
			TPMRemaining:         floatPtr(secondaryRemaining),
			PrimaryUsedPercent:   floatPtr(primaryUsed),
			SecondaryUsedPercent: floatPtr(secondaryUsed),
			PrimaryResetsAt:      primaryResetsAt,
			SecondaryResetsAt:    secondaryResetsAt,
		},
		Meta: map[string]any{
			"allowed":       allowed,
			"limit_reached": limitReached,
			"has_credits":   hasCredits,
			"unlimited":     unlimited,
		},
	}, true
}

func classifyOfficialStatusError(statusCode int, raw []byte) error {
	body := strings.ToLower(strings.Join(strings.Fields(string(raw)), " "))
	switch {
	case statusCode == http.StatusTooManyRequests || strings.Contains(body, "usage limit") || strings.Contains(body, "quota"):
		return &FetchError{Kind: FetchErrorKindQuota, Op: "usage_status", Err: fmt.Errorf("http status %d: %s", statusCode, strings.TrimSpace(string(raw)))}
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return &FetchError{Kind: FetchErrorKindAuth, Op: "usage_status", Err: fmt.Errorf("http status %d: %s", statusCode, strings.TrimSpace(string(raw)))}
	default:
		return &FetchError{Kind: FetchErrorKindUpstream, Op: "usage_status", Err: fmt.Errorf("http status %d: %s", statusCode, strings.TrimSpace(string(raw)))}
	}
}

func floatPtr(v float64) *float64 {
	return &v
}

func maxFloat(a float64, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func unixSecondsPtr(seconds int64) *time.Time {
	if seconds <= 0 {
		return nil
	}
	value := time.Unix(seconds, 0).UTC()
	return &value
}

func asFloat(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case int32:
		return float64(typed)
	case int16:
		return float64(typed)
	case int8:
		return float64(typed)
	case uint:
		return float64(typed)
	case uint64:
		return float64(typed)
	case uint32:
		return float64(typed)
	case uint16:
		return float64(typed)
	case uint8:
		return float64(typed)
	case string:
		value := strings.TrimSpace(typed)
		if value == "" {
			return 0
		}
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}
