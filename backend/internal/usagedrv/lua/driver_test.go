package lua_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/accountdrv"
	luadrv "github.com/gcssloop/codex-router/backend/internal/usagedrv/lua"
)

func TestLuaDriverSupportsAndName(t *testing.T) {
	t.Parallel()

	driver := luadrv.NewDriver(nil, moduleRoot(t))
	if driver.Name() != "lua" {
		t.Fatalf("Name = %q, want %q", driver.Name(), "lua")
	}
	if !driver.Supports(accounts.Account{UsageDriver: "lua"}) {
		t.Fatal("Supports = false, want true for usage_driver=lua")
	}
	if driver.Supports(accounts.Account{UsageDriver: "builtin_ppchat"}) {
		t.Fatal("Supports = true, want false for non-lua usage_driver")
	}
}

func TestLuaDriverRejectsMalformedUsageConfig(t *testing.T) {
	t.Parallel()

	driver := luadrv.NewDriver(nil, moduleRoot(t))
	tests := []string{
		"{invalid",
		`{"script":"internal/usagedrv/lua/testdata/vendor_x.lua","timeout_ms":{}}`,
		`{"script":"internal/usagedrv/lua/testdata/vendor_x.lua","timeout_ms":"abc"}`,
	}
	for _, raw := range tests {
		_, err := driver.Fetch(context.Background(), accounts.Account{
			UsageDriver:     "lua",
			UsageConfigJSON: raw,
		}, accountdrv.ResolvedCredential{})
		if err == nil {
			t.Fatalf("Fetch returned nil error for config %s, want malformed config error", raw)
		}
	}
}

func TestLuaDriverPassesContextToScript(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer at-123" {
			t.Fatalf("Authorization = %q, want %q", got, "Bearer at-123")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"quota_remaining":9876,"rpm_remaining":65,"tpm_remaining":12345}`))
	}))
	defer server.Close()

	driver := luadrv.NewDriver(nil, moduleRoot(t))
	account := accounts.Account{
		ID:            42,
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "vendor-x",
		UsageDriver:   "lua",
		UsageConfigJSON: `{
			"script":"internal/usagedrv/lua/testdata/vendor_x.lua",
			"timeout_ms":2000,
			"endpoint":"` + server.URL + `/usage"
		}`,
	}
	result, err := driver.Fetch(context.Background(), account, accountdrv.ResolvedCredential{
		Kind:        "bearer",
		AccessToken: "at-123",
	})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if result.Source != "remote" {
		t.Fatalf("Source = %q, want %q", result.Source, "remote")
	}
	if result.Confidence != "high" {
		t.Fatalf("Confidence = %q, want %q", result.Confidence, "high")
	}
	if result.Limits.QuotaRemaining == nil || *result.Limits.QuotaRemaining != 9876 {
		t.Fatalf("QuotaRemaining = %#v, want 9876", result.Limits.QuotaRemaining)
	}
	if result.Meta["account_id"] != float64(42) {
		t.Fatalf("meta.account_id = %#v, want 42", result.Meta["account_id"])
	}
}
