package builtin_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accountdrv"
	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/usagedrv/builtin"
)

func TestOpenAIOfficialDriverFetchParsesUsageLimits(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/wham/usage" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"rate_limit":{"allowed":true,"limit_reached":false,"primary_window":{"used_percent":25,"reset_at":1735689600},"secondary_window":{"used_percent":40,"reset_at":1735693200}},
			"credits":{"balance":12.5,"has_credits":true,"unlimited":false}
		}`))
	}))
	defer server.Close()

	driver := builtin.NewOpenAIOfficialDriver(http.DefaultClient)
	result, err := driver.Fetch(context.Background(), accounts.Account{
		ProviderType: accounts.ProviderOpenAIOfficial,
		BaseURL:      server.URL + "/backend-api/codex",
	}, accountdrv.ResolvedCredential{
		AccessToken: "at-token",
		Metadata:    map[string]any{"account_id": "acct-1"},
	})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if result.Limits.Balance == nil || *result.Limits.Balance != 12.5 {
		t.Fatalf("Balance = %#v, want 12.5", result.Limits.Balance)
	}
	if result.Limits.PrimaryUsedPercent == nil || *result.Limits.PrimaryUsedPercent != 25 {
		t.Fatalf("PrimaryUsedPercent = %#v, want 25", result.Limits.PrimaryUsedPercent)
	}
	if result.Limits.SecondaryUsedPercent == nil || *result.Limits.SecondaryUsedPercent != 40 {
		t.Fatalf("SecondaryUsedPercent = %#v, want 40", result.Limits.SecondaryUsedPercent)
	}
	if result.Limits.RPMRemaining == nil || *result.Limits.RPMRemaining != 75 {
		t.Fatalf("RPMRemaining = %#v, want 75", result.Limits.RPMRemaining)
	}
	if result.Limits.TPMRemaining == nil || *result.Limits.TPMRemaining != 60 {
		t.Fatalf("TPMRemaining = %#v, want 60", result.Limits.TPMRemaining)
	}
	if got, ok := result.Meta["allowed"].(bool); !ok || !got {
		t.Fatalf("meta.allowed = %#v, want true", result.Meta["allowed"])
	}
	if got, ok := result.Meta["has_credits"].(bool); !ok || !got {
		t.Fatalf("meta.has_credits = %#v, want true", result.Meta["has_credits"])
	}
	if got, ok := result.Meta["unlimited"].(bool); !ok || got {
		t.Fatalf("meta.unlimited = %#v, want false", result.Meta["unlimited"])
	}
}

func TestOpenAIOfficialDriverFetchParsesCurrentCodexRateLimits(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/wham/usage" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"plan_type":"pro",
			"rate_limit":{
				"allowed":true,
				"limit_reached":false,
				"primary_window":{
					"used_percent":52,
					"limit_window_seconds":18000,
					"reset_after_seconds":3600,
					"reset_at":1767205800
				},
				"secondary_window":{
					"used_percent":51,
					"limit_window_seconds":604800,
					"reset_after_seconds":86400,
					"reset_at":1783350300
				}
			},
			"credits":{"has_credits":true,"unlimited":false,"balance":"0"},
			"additional_rate_limits":[
				{
					"limit_name":"codex_other",
					"metered_feature":"codex_other",
					"rate_limit":{
						"allowed":true,
						"limit_reached":false,
						"primary_window":{
							"used_percent":88,
							"limit_window_seconds":1800,
							"reset_after_seconds":600,
							"reset_at":1735693200
						}
					}
				}
			],
			"rate_limit_reached_type":{"type":"workspace_member_usage_limit_reached"},
			"rate_limit_reset_credits":{"available_count":1}
		}`))
	}))
	defer server.Close()

	driver := builtin.NewOpenAIOfficialDriver(http.DefaultClient)
	result, err := driver.Fetch(context.Background(), accounts.Account{
		ProviderType: accounts.ProviderOpenAIOfficial,
		BaseURL:      server.URL + "/backend-api/codex",
	}, accountdrv.ResolvedCredential{
		AccessToken: "at-token",
		Metadata:    map[string]any{"account_id": "acct-1"},
	})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if result.Limits.PrimaryUsedPercent == nil || *result.Limits.PrimaryUsedPercent != 52 {
		t.Fatalf("PrimaryUsedPercent = %#v, want 52", result.Limits.PrimaryUsedPercent)
	}
	if result.Limits.SecondaryUsedPercent == nil || *result.Limits.SecondaryUsedPercent != 51 {
		t.Fatalf("SecondaryUsedPercent = %#v, want 51", result.Limits.SecondaryUsedPercent)
	}
	if result.Limits.RPMRemaining == nil || *result.Limits.RPMRemaining != 48 {
		t.Fatalf("RPMRemaining = %#v, want 48", result.Limits.RPMRemaining)
	}
	if result.Limits.TPMRemaining == nil || *result.Limits.TPMRemaining != 49 {
		t.Fatalf("TPMRemaining = %#v, want 49", result.Limits.TPMRemaining)
	}
	wantPrimaryReset := time.Unix(1767205800, 0).UTC()
	if result.Limits.PrimaryResetsAt == nil || !result.Limits.PrimaryResetsAt.Equal(wantPrimaryReset) {
		t.Fatalf("PrimaryResetsAt = %#v, want %s", result.Limits.PrimaryResetsAt, wantPrimaryReset)
	}
	wantSecondaryReset := time.Unix(1783350300, 0).UTC()
	if result.Limits.SecondaryResetsAt == nil || !result.Limits.SecondaryResetsAt.Equal(wantSecondaryReset) {
		t.Fatalf("SecondaryResetsAt = %#v, want %s", result.Limits.SecondaryResetsAt, wantSecondaryReset)
	}
	if got, ok := result.Meta["plan_type"].(string); !ok || got != "pro" {
		t.Fatalf("meta.plan_type = %#v, want pro", result.Meta["plan_type"])
	}
	reachedType, ok := result.Meta["rate_limit_reached_type"].(map[string]any)
	if !ok || reachedType["type"] != "workspace_member_usage_limit_reached" {
		t.Fatalf("meta.rate_limit_reached_type = %#v, want workspace_member_usage_limit_reached", result.Meta["rate_limit_reached_type"])
	}
}

func TestOpenAIOfficialDriverFetchClassifiesErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		statusCode  int
		body        string
		wantErrKind builtin.FetchErrorKind
	}{
		{name: "auth", statusCode: http.StatusUnauthorized, body: `{"error":"unauthorized"}`, wantErrKind: builtin.FetchErrorKindAuth},
		{name: "quota-like 403", statusCode: http.StatusForbidden, body: `{"error":"usage limit reached"}`, wantErrKind: builtin.FetchErrorKindQuota},
		{name: "quota", statusCode: http.StatusTooManyRequests, body: `{"error":"usage limit reached"}`, wantErrKind: builtin.FetchErrorKindQuota},
		{name: "upstream", statusCode: http.StatusBadGateway, body: `{"error":"bad gateway"}`, wantErrKind: builtin.FetchErrorKindUpstream},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			driver := builtin.NewOpenAIOfficialDriver(http.DefaultClient)
			_, err := driver.Fetch(context.Background(), accounts.Account{
				ProviderType: accounts.ProviderOpenAIOfficial,
				BaseURL:      server.URL,
			}, accountdrv.ResolvedCredential{AccessToken: "at-token", Metadata: map[string]any{"account_id": "acct-1"}})
			if err == nil {
				t.Fatal("Fetch returned nil error, want error")
			}
			var fetchErr *builtin.FetchError
			if !errors.As(err, &fetchErr) {
				t.Fatalf("error type = %T, want *builtin.FetchError", err)
			}
			if fetchErr.Kind != tc.wantErrKind {
				t.Fatalf("error kind = %q, want %q", fetchErr.Kind, tc.wantErrKind)
			}
		})
	}
}

func TestPPChatDriverFetchParsesUsageLimits(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success":true,
			"data":{"token_info":{"remain_quota_display":321.9}}
		}`))
	}))
	defer server.Close()

	driver := builtin.NewPPChatDriver(http.DefaultClient)
	driver.SetTokenLogsBaseURLForTest(server.URL)
	result, err := driver.Fetch(context.Background(), accounts.Account{
		ProviderType: accounts.ProviderOpenAICompatible,
		BaseURL:      server.URL,
	}, accountdrv.ResolvedCredential{AccessToken: "ppchat-token"})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if result.Limits.QuotaRemaining == nil || *result.Limits.QuotaRemaining != 321.9 {
		t.Fatalf("QuotaRemaining = %#v, want 321.9", result.Limits.QuotaRemaining)
	}
}

func TestPPChatDriverFetchAlwaysUsesFixedTokenLogHost(t *testing.T) {
	t.Parallel()

	capturedHost := ""
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			capturedHost = req.URL.Host
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"success":true,"data":{"token_info":{"remain_quota_display":1}}}`)),
			}, nil
		}),
	}

	driver := builtin.NewPPChatDriver(client)
	_, err := driver.Fetch(context.Background(), accounts.Account{
		ProviderType: accounts.ProviderOpenAICompatible,
		BaseURL:      "https://code.ppchat.vip/v1",
	}, accountdrv.ResolvedCredential{AccessToken: "ppchat-token"})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if capturedHost != "his.ppchat.vip" {
		t.Fatalf("host = %q, want %q", capturedHost, "his.ppchat.vip")
	}
}

func TestPPChatDriverFetchClassifiesErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		statusCode  int
		body        string
		wantErrKind builtin.FetchErrorKind
	}{
		{name: "auth", statusCode: http.StatusUnauthorized, body: `{"error":"invalid token"}`, wantErrKind: builtin.FetchErrorKindAuth},
		{name: "quota", statusCode: http.StatusForbidden, body: `{"error":"额度不足"}`, wantErrKind: builtin.FetchErrorKindQuota},
		{name: "upstream", statusCode: http.StatusBadGateway, body: `{"error":"upstream failed"}`, wantErrKind: builtin.FetchErrorKindUpstream},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			driver := builtin.NewPPChatDriver(http.DefaultClient)
			driver.SetTokenLogsBaseURLForTest(server.URL)
			_, err := driver.Fetch(context.Background(), accounts.Account{
				ProviderType: accounts.ProviderOpenAICompatible,
				BaseURL:      server.URL,
			}, accountdrv.ResolvedCredential{AccessToken: "ppchat-token"})
			if err == nil {
				t.Fatal("Fetch returned nil error, want error")
			}
			var fetchErr *builtin.FetchError
			if !errors.As(err, &fetchErr) {
				t.Fatalf("error type = %T, want *builtin.FetchError", err)
			}
			if fetchErr.Kind != tc.wantErrKind {
				t.Fatalf("error kind = %q, want %q", fetchErr.Kind, tc.wantErrKind)
			}
		})
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
