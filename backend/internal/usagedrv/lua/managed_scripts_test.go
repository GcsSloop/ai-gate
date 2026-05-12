package lua_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/accountdrv"
	"github.com/gcssloop/codex-router/backend/internal/accounts"
	luadrv "github.com/gcssloop/codex-router/backend/internal/usagedrv/lua"
)

func TestLuaDriverFetchesManagedScript(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk-managed" {
			t.Fatalf("Authorization = %q, want Bearer sk-managed", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"quota_remaining":2468}`))
	}))
	defer server.Close()

	root := t.TempDir()
	manager, err := luadrv.NewManagedScriptStore(root)
	if err != nil {
		t.Fatalf("NewManagedScriptStore returned error: %v", err)
	}
	if err := manager.Save("vendor_shared", `function fetch_usage(ctx)
  local response = ctx.host.http_get({ url = ctx.config.endpoint, headers = { Authorization = "Bearer " .. ctx.credential.access_token } })
  local payload = ctx.host.json_decode(response.body)
  return {
    ok = true,
    source = "remote",
    confidence = "high",
    limits = {
      quota_remaining = payload.quota_remaining
    }
  }
end
`); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	driver := luadrv.NewDriver(nil, moduleRoot(t), luadrv.WithManagedScriptRoot(root))
	result, err := driver.Fetch(context.Background(), accounts.Account{
		UsageDriver:     "lua",
		UsageConfigJSON: `{"script":"managed:vendor_shared","endpoint":"` + server.URL + `/usage"}`,
	}, accountdrv.ResolvedCredential{AccessToken: "sk-managed"})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if result.Limits.QuotaRemaining == nil || *result.Limits.QuotaRemaining != 2468 {
		t.Fatalf("QuotaRemaining = %#v, want 2468", result.Limits.QuotaRemaining)
	}
}

func TestLuaDriverUsesBuiltInManagedDSLScript(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/usage" {
			t.Fatalf("path = %q, want /v1/usage", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-nodeseek" {
			t.Fatalf("Authorization = %q, want Bearer sk-nodeseek", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"remaining":88,"unit":"USD"}`))
	}))
	defer server.Close()

	driver := luadrv.NewDriver(nil, moduleRoot(t), luadrv.WithManagedScriptRoot(t.TempDir()))
	result, err := driver.Fetch(context.Background(), accounts.Account{
		BaseURL:         server.URL,
		UsageDriver:     "lua",
		UsageConfigJSON: `{"script":"managed:ai.nodeseek.in"}`,
	}, accountdrv.ResolvedCredential{APIKey: "sk-nodeseek"})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if result.Limits.Balance == nil || *result.Limits.Balance != 88 {
		t.Fatalf("Balance = %#v, want 88", result.Limits.Balance)
	}
}

func TestManagedScriptStoreRejectsInvalidKey(t *testing.T) {
	t.Parallel()

	manager, err := luadrv.NewManagedScriptStore(filepath.Join(t.TempDir(), "usage-scripts"))
	if err != nil {
		t.Fatalf("NewManagedScriptStore returned error: %v", err)
	}
	if err := manager.Save("../escape", "function fetch_usage(ctx) return { ok = true, limits = {} } end"); err == nil {
		t.Fatal("Save returned nil error, want invalid key error")
	}
}
