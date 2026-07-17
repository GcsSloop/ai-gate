package lua_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accountdrv"
	"github.com/gcssloop/codex-router/backend/internal/accounts"
	luadrv "github.com/gcssloop/codex-router/backend/internal/usagedrv/lua"
)

type roundTripFunc func(*http.Request) *http.Response

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

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
	runtime := luadrv.NewRuntime(nil, filepath.Dir(scriptPath))
	_, err := runtime.Execute(context.Background(), filepath.Base(scriptPath), accounts.Account{}, accountdrv.ResolvedCredential{}, map[string]any{})
	if err == nil {
		t.Fatal("Execute returned nil error, want missing function error")
	}
	if !strings.Contains(err.Error(), "missing fetch_usage") {
		t.Fatalf("error = %q, want missing fetch_usage", err.Error())
	}
}

func TestRuntimeExecuteSimpleUsageDSL(t *testing.T) {
	t.Parallel()

	scriptPath := writeTempScript(t, `
simple_usage({
  get = "{{base_url}}/v1/usage",
  auth = "bearer",
  remaining = pick("remaining", "quota.remaining", "balance"),
  unit = pick("unit", "quota.unit", default("USD")),
  valid = pick("is_active", "isValid", default(true))
})
`)
	runtime := luadrv.NewRuntime(&http.Client{Transport: roundTripFunc(func(req *http.Request) *http.Response {
		if req.URL.String() != "https://ai.nodeseek.in/v1/usage" {
			t.Fatalf("request url = %q, want nodeseek usage endpoint", req.URL.String())
		}
		if req.Header.Get("Authorization") != "Bearer sk-test" {
			t.Fatalf("Authorization = %q, want bearer token", req.Header.Get("Authorization"))
		}
		return jsonResponse(`{"quota":{"remaining":42.5}}`)
	})}, filepath.Dir(scriptPath))
	result, err := runtime.Execute(
		context.Background(),
		filepath.Base(scriptPath),
		accounts.Account{BaseURL: "https://ai.nodeseek.in"},
		accountdrv.ResolvedCredential{APIKey: "sk-test"},
		map[string]any{},
	)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Limits.Balance == nil || *result.Limits.Balance != 42.5 {
		t.Fatalf("Balance = %#v, want 42.5", result.Limits.Balance)
	}
	if result.Meta["unit"] != "USD" {
		t.Fatalf("unit meta = %#v, want USD", result.Meta["unit"])
	}
	if result.Meta["is_valid"] != true {
		t.Fatalf("is_valid meta = %#v, want true", result.Meta["is_valid"])
	}
}

func TestRuntimeHTTPPostRetriesOptedInRateLimit(t *testing.T) {
	t.Parallel()

	scriptPath := writeTempScript(t, `
function fetch_usage(ctx)
  local response = ctx.host.http_post({
    url = "https://token.stellaisle.com/api/user/login?turnstile=",
    headers = { ["Content-Type"] = "application/json" },
    body = "{}",
    retry_on_429 = true,
    retry_count = 2,
    retry_delay_ms = 0
  })
  if response.status ~= 200 then
    error("unexpected status " .. tostring(response.status))
  end
  return { ok = true, source = "remote", confidence = "high", limits = {} }
end
`)
	attempts := 0
	runtime := luadrv.NewRuntime(&http.Client{Transport: roundTripFunc(func(req *http.Request) *http.Response {
		attempts++
		if attempts == 1 {
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     http.Header{"Retry-After": []string{"0"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":"rate limited"}`)),
			}
		}
		return jsonResponse(`{"ok":true}`)
	})}, filepath.Dir(scriptPath))

	_, err := runtime.Execute(context.Background(), filepath.Base(scriptPath), accounts.Account{}, accountdrv.ResolvedCredential{}, map[string]any{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("POST attempts = %d, want 2", attempts)
	}
}

func TestRuntimeExecuteSimpleUsageDSLReturnsDisplayHints(t *testing.T) {
	t.Parallel()

	scriptPath := writeTempScript(t, `
simple_usage({
  get = "/v1/usage",
  auth = "bearer",
  remaining = pick("remaining"),
  display = {
    summary = {
      label = "余额",
      value = function(payload)
        return "$" .. string.format("%.2f", payload.remaining)
      end
    },
    detail_stats = {
      { label = "余额", value = function(payload) return "$" .. string.format("%.2f", payload.remaining) end },
      { label = "状态", value = "可用" }
    },
    detail_items = {
      { label = "计费单位", value = "美元" }
    }
  }
})
`)
	runtime := luadrv.NewRuntime(&http.Client{Transport: roundTripFunc(func(req *http.Request) *http.Response {
		return jsonResponse(`{"remaining":61.96}`)
	})}, filepath.Dir(scriptPath))
	result, err := runtime.Execute(
		context.Background(),
		filepath.Base(scriptPath),
		accounts.Account{BaseURL: "https://ai.nodeseek.in"},
		accountdrv.ResolvedCredential{APIKey: "sk-test"},
		map[string]any{},
	)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	summary, ok := result.Display["summary"].(map[string]any)
	if !ok {
		t.Fatalf("display.summary = %#v, want object", result.Display["summary"])
	}
	if summary["label"] != "余额" || summary["value"] != "$61.96" {
		t.Fatalf("display.summary = %#v, want balance label/value", summary)
	}
	stats, ok := result.Display["detail_stats"].([]any)
	if !ok || len(stats) != 2 {
		t.Fatalf("display.detail_stats = %#v, want two items", result.Display["detail_stats"])
	}
}

func TestRuntimeExecuteSimpleUsageDSLDeduplicatesVersionedBaseURL(t *testing.T) {
	t.Parallel()

	scriptPath := writeTempScript(t, `
simple_usage({
  get = "/v1/usage",
  auth = "bearer",
  remaining = pick("remaining")
})
`)
	runtime := luadrv.NewRuntime(&http.Client{Transport: roundTripFunc(func(req *http.Request) *http.Response {
		if req.URL.String() != "https://ai.nodeseek.in/v1/usage" {
			t.Fatalf("request url = %q, want single /v1 usage endpoint", req.URL.String())
		}
		return jsonResponse(`{"remaining":1}`)
	})}, filepath.Dir(scriptPath))
	_, err := runtime.Execute(
		context.Background(),
		filepath.Base(scriptPath),
		accounts.Account{BaseURL: "https://ai.nodeseek.in/v1"},
		accountdrv.ResolvedCredential{APIKey: "sk-test"},
		map[string]any{},
	)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
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
	runtime = luadrv.NewRuntime(nil, filepath.Dir(scriptPath))
	_, err := runtime.Execute(ctx, filepath.Base(scriptPath), accounts.Account{}, accountdrv.ResolvedCredential{}, map[string]any{})
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

func TestRuntimeExecuteRejectsAbsoluteScriptOutsideBaseDir(t *testing.T) {
	t.Parallel()

	scriptPath := writeTempScript(t, `
function fetch_usage(ctx)
  return {
    ok = true,
    source = "remote",
    confidence = "high",
    limits = {}
  }
end
`)
	runtime := luadrv.NewRuntime(nil, moduleRoot(t))
	_, err := runtime.Execute(context.Background(), scriptPath, accounts.Account{}, accountdrv.ResolvedCredential{}, map[string]any{})
	if err == nil {
		t.Fatal("Execute returned nil error, want script path restriction error")
	}
	if !strings.Contains(err.Error(), "outside adapter root") {
		t.Fatalf("error = %q, want outside adapter root", err.Error())
	}
}

func TestRuntimeExecuteRejectsSymlinkScriptOutsideBaseDir(t *testing.T) {
	t.Parallel()

	outside := writeTempScript(t, `
function fetch_usage(ctx)
  return {
    ok = true,
    source = "remote",
    confidence = "high",
    limits = {}
  }
end
`)
	baseDir := t.TempDir()
	linkPath := filepath.Join(baseDir, "linked.lua")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Fatalf("Symlink returned error: %v", err)
	}

	runtime := luadrv.NewRuntime(nil, baseDir)
	_, err := runtime.Execute(context.Background(), "linked.lua", accounts.Account{}, accountdrv.ResolvedCredential{}, map[string]any{})
	if err == nil {
		t.Fatal("Execute returned nil error, want symlink restriction error")
	}
	if !strings.Contains(err.Error(), "outside adapter root") {
		t.Fatalf("error = %q, want outside adapter root", err.Error())
	}
}

func TestRuntimeExecuteRejectsCyclicResult(t *testing.T) {
	t.Parallel()

	if os.Getenv("LUA_CYCLE_HELPER") == "1" {
		scriptPath := writeTempScript(t, `
function fetch_usage(ctx)
  local payload = {}
  payload.self = payload
  return {
    ok = true,
    source = "remote",
    confidence = "high",
    limits = {},
    payload = payload
  }
end
`)
		runtime := luadrv.NewRuntime(nil, filepath.Dir(scriptPath))
		_, err := runtime.Execute(context.Background(), filepath.Base(scriptPath), accounts.Account{}, accountdrv.ResolvedCredential{}, map[string]any{})
		if err != nil {
			fmt.Fprintln(os.Stdout, err.Error())
			os.Exit(0)
		}
		fmt.Fprintln(os.Stdout, "missing cycle rejection")
		os.Exit(2)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestRuntimeExecuteRejectsCyclicResult")
	cmd.Env = append(os.Environ(), "LUA_CYCLE_HELPER=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("cycle helper exited with error: %v, output=%s", err, output)
	}
	if !strings.Contains(string(output), "cycle") {
		t.Fatalf("helper output = %q, want cycle error", string(output))
	}
}

func TestRuntimeExecuteRejectsCyclicJSONEncode(t *testing.T) {
	t.Parallel()

	scriptPath := writeTempScript(t, `
function fetch_usage(ctx)
  local payload = {}
  payload.self = payload
  local _ = ctx.host.json_encode(payload)
  return {
    ok = true,
    source = "remote",
    confidence = "high",
    limits = {}
  }
end
`)
	runtime := luadrv.NewRuntime(nil, filepath.Dir(scriptPath))
	_, err := runtime.Execute(context.Background(), filepath.Base(scriptPath), accounts.Account{}, accountdrv.ResolvedCredential{}, map[string]any{})
	if err == nil {
		t.Fatal("Execute returned nil error, want json_encode cycle error")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("error = %q, want cycle error", err.Error())
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
