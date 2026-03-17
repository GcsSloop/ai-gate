package lua_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accountdrv"
	"github.com/gcssloop/codex-router/backend/internal/accounts"
	luadrv "github.com/gcssloop/codex-router/backend/internal/usagedrv/lua"
)

func TestRuntimeExecuteValidScript(t *testing.T) {
	t.Parallel()

	runtime := luadrv.NewRuntime(nil, moduleRoot(t))
	result, err := runtime.Execute(
		context.Background(),
		"internal/usagedrv/lua/testdata/minimal_ok.lua",
		accounts.Account{ID: 1, ProviderType: accounts.ProviderOpenAICompatible},
		accountdrv.ResolvedCredential{AccessToken: "token"},
		map[string]any{},
	)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Source != "remote" {
		t.Fatalf("Source = %q, want %q", result.Source, "remote")
	}
	if result.Confidence != "high" {
		t.Fatalf("Confidence = %q, want %q", result.Confidence, "high")
	}
	if result.Limits.QuotaRemaining == nil || *result.Limits.QuotaRemaining != 120000 {
		t.Fatalf("QuotaRemaining = %#v, want 120000", result.Limits.QuotaRemaining)
	}
}

func TestRuntimeExecuteRejectsInvalidSchema(t *testing.T) {
	t.Parallel()

	runtime := luadrv.NewRuntime(nil, moduleRoot(t))
	_, err := runtime.Execute(
		context.Background(),
		"internal/usagedrv/lua/testdata/invalid_shape.lua",
		accounts.Account{},
		accountdrv.ResolvedCredential{},
		map[string]any{},
	)
	if err == nil {
		t.Fatal("Execute returned nil error, want schema error")
	}
	if !strings.Contains(err.Error(), "unknown top-level key") {
		t.Fatalf("error = %q, want unknown top-level key", err.Error())
	}
}

func TestRuntimeExecuteRequiresFetchUsageFunction(t *testing.T) {
	t.Parallel()

	scriptPath := writeTempScript(t, `x = 1`)
	runtime := luadrv.NewRuntime(nil, "")
	_, err := runtime.Execute(context.Background(), scriptPath, accounts.Account{}, accountdrv.ResolvedCredential{}, map[string]any{})
	if err == nil {
		t.Fatal("Execute returned nil error, want missing function error")
	}
	if !strings.Contains(err.Error(), "missing fetch_usage") {
		t.Fatalf("error = %q, want missing fetch_usage", err.Error())
	}
}

func TestRuntimeExecuteTimeout(t *testing.T) {
	t.Parallel()

	scriptPath := writeTempScript(t, `
function fetch_usage(ctx)
  ctx.host.sleep_ms(200)
  return {
    ok = true,
    source = "remote",
    confidence = "high",
    limits = {}
  }
end
`)
	runtime := luadrv.NewRuntime(nil, "")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := runtime.Execute(ctx, scriptPath, accounts.Account{}, accountdrv.ResolvedCredential{}, map[string]any{})
	if err == nil {
		t.Fatal("Execute returned nil error, want timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("error = %q, want timeout", err.Error())
	}
}

func TestRuntimeExecuteOnlyHostAPIExposed(t *testing.T) {
	t.Parallel()

	scriptPath := writeTempScript(t, `
function fetch_usage(ctx)
  local _ = os.execute("echo hi")
  return {
    ok = true,
    source = "remote",
    confidence = "high",
    limits = {}
  }
end
`)
	runtime := luadrv.NewRuntime(nil, "")
	_, err := runtime.Execute(context.Background(), scriptPath, accounts.Account{}, accountdrv.ResolvedCredential{}, map[string]any{})
	if err == nil {
		t.Fatal("Execute returned nil error, want forbidden global error")
	}
}

func writeTempScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "script.lua")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	return path
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd returned error: %v", err)
	}
	dir := wd
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found from %s", wd)
		}
		dir = parent
	}
}
