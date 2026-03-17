package builtin_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

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
		BaseURL:      server.URL,
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

func TestOpenAIOfficialDriverFetchClassifiesErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		statusCode  int
		body        string
		wantErrKind builtin.FetchErrorKind
	}{
		{name: "auth", statusCode: http.StatusUnauthorized, body: `{"error":"unauthorized"}`, wantErrKind: builtin.FetchErrorKindAuth},
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
