package lua_test

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/accountdrv"
	"github.com/gcssloop/codex-router/backend/internal/accounts"
	luadrv "github.com/gcssloop/codex-router/backend/internal/usagedrv/lua"
)

type managedScriptRoundTripFunc func(*http.Request) (*http.Response, error)

func (f managedScriptRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

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
	summary, ok := result.Display["summary"].(map[string]any)
	if !ok {
		t.Fatalf("display.summary = %#v, want object", result.Display["summary"])
	}
	if summary["label"] != "余额" || summary["value"] != "$88.00" {
		t.Fatalf("display.summary = %#v, want formatted balance", summary)
	}
}

func TestLuaDriverUsesManagedSansiStatsByAccountName(t *testing.T) {
	t.Parallel()

	client := &http.Client{Transport: managedScriptRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://lab.sansi.io/ppc-stats/api/status" {
			t.Fatalf("URL = %q, want lab.sansi.io status endpoint", r.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(`{
				"tokens": [
					{"token":"new-api-team4","label":"team4","provider":"new-api"},
					{"token":"ppchat-team4","label":"team4","provider":"ppchat"}
				],
				"token_summaries": [
					{"token":"new-api-team4","token_label":"team4","package_type":"每天15刀套餐","quota_raw_remain_quota":3457762,"quota_raw_today_used_quota":4042238,"quota_raw_today_added_quota":7500000,"quota_raw_scale":500000,"quota_raw_unit":"USD"},
					{"token":"ppchat-team4","token_label":"team4","package_type":"other","quota_raw_remain_quota":1,"quota_raw_today_used_quota":2,"quota_raw_today_added_quota":3,"quota_raw_scale":1,"quota_raw_unit":"USD"}
				]
			}`)),
		}, nil
	})}

	root := t.TempDir()
	manager, err := luadrv.NewManagedScriptStore(root)
	if err != nil {
		t.Fatalf("NewManagedScriptStore returned error: %v", err)
	}
	if err := manager.Save("sansi", `function fetch_usage(ctx)
  local response = ctx.host.http_get({ url = "https://lab.sansi.io/ppc-stats/api/status" })
  local payload = ctx.host.json_decode(response.body)
  local token
  for _, candidate in ipairs(payload.tokens or {}) do
    if candidate.label == ctx.account.account_name and candidate.provider == "new-api" then
      token = candidate
      break
    end
  end
  local summary
  for _, candidate in ipairs(payload.token_summaries or {}) do
    if token ~= nil and candidate.token == token.token then
      summary = candidate
      break
    end
  end
  local scale = summary.quota_raw_scale
  local remaining = summary.quota_raw_remain_quota / scale
  local today_used = summary.quota_raw_today_used_quota / scale
  local today_quota = summary.quota_raw_today_added_quota / scale
  return {
    ok = true,
    source = "remote",
    confidence = "high",
    limits = { balance = today_quota, quota_remaining = remaining },
    meta = { today_used = today_used },
    display = {
      summary = { label = "今日额度", value = string.format("$%.3f / $%.3f", today_used, today_quota) },
      detail_stats = {
        { label = "今日额度", value = string.format("$%.3f", today_quota) },
        { label = "今日已用", value = string.format("$%.3f", today_used) },
        { label = "今日剩余", value = string.format("$%.3f", remaining) }
      },
      usage_windows = { { label = "1D", remaining_percent = remaining / today_quota * 100 } }
    }
  }
end
`); err != nil {
		t.Fatalf("Save Sansi script returned error: %v", err)
	}

	driver := luadrv.NewDriver(client, moduleRoot(t), luadrv.WithManagedScriptRoot(root))
	result, err := driver.Fetch(context.Background(), accounts.Account{
		AccountName:     "team4",
		UsageDriver:     "lua",
		UsageConfigJSON: `{"script":"managed:sansi"}`,
	}, accountdrv.ResolvedCredential{})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if result.Limits.Balance == nil || *result.Limits.Balance != 15 {
		t.Fatalf("Balance = %#v, want 15 USD daily quota", result.Limits.Balance)
	}
	if result.Limits.QuotaRemaining == nil || *result.Limits.QuotaRemaining != 3457762.0/500000.0 {
		t.Fatalf("QuotaRemaining = %#v, want scaled daily remaining", result.Limits.QuotaRemaining)
	}
	if result.Meta["today_used"] != 4042238.0/500000.0 {
		t.Fatalf("today_used = %#v, want scaled daily usage", result.Meta["today_used"])
	}
	display := result.Display
	summary, ok := display["summary"].(map[string]any)
	if !ok || summary["label"] != "今日额度" || summary["value"] != "$8.084 / $15.000" {
		t.Fatalf("summary = %#v, want today's used and total quota", display["summary"])
	}
	stats, ok := display["detail_stats"].([]any)
	if !ok || len(stats) != 3 {
		t.Fatalf("detail_stats = %#v, want three daily quota metrics", display["detail_stats"])
	}
	windows, ok := display["usage_windows"].([]any)
	if !ok || len(windows) != 1 {
		t.Fatalf("usage_windows = %#v, want one daily progress window", display["usage_windows"])
	}
	window, ok := windows[0].(map[string]any)
	wantRemainingPercent := (3457762.0 / 7500000.0) * 100
	gotRemainingPercent, numberOK := window["remaining_percent"].(float64)
	if !ok || !numberOK || window["label"] != "1D" || math.Abs(gotRemainingPercent-wantRemainingPercent) > 1e-9 {
		t.Fatalf("usage_windows[0] = %#v, want scaled daily remaining percentage", windows[0])
	}
}

func TestLuaDriverUsesManagedStellaisleLoginCookieForSubscription17(t *testing.T) {
	t.Parallel()

	loginAttempts := 0
	client := &http.Client{Transport: managedScriptRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/user/login":
			loginAttempts++
			if loginAttempts == 1 {
				return &http.Response{
					StatusCode: http.StatusTooManyRequests,
					Header:     http.Header{"Retry-After": []string{"0"}},
					Body:       io.NopCloser(strings.NewReader(`{"error":"rate limited"}`)),
				}, nil
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read login body: %v", err)
			}
			var credentials map[string]string
			if err := json.Unmarshal(body, &credentials); err != nil {
				t.Fatalf("decode login body: %v", err)
			}
			if credentials["username"] != "user@example.com" || credentials["password"] != "pass-placeholder" {
				t.Fatalf("login credentials = %#v, want configured credentials", credentials)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{"Set-Cookie": []string{
					"stellaisle_session=abc123; Path=/; HttpOnly",
					"stellaisle_csrf=csrf456; Path=/; HttpOnly",
				}},
				Body: io.NopCloser(strings.NewReader(`{"success":true,"data":{"id":264}}`)),
			}, nil
		case r.Method == http.MethodGet && r.URL.Path == "/api/subscription/self":
			if got := r.Header.Get("Cookie"); got != "stellaisle_session=abc123; stellaisle_csrf=csrf456" {
				t.Fatalf("Cookie = %q, want all session cookies", got)
			}
			if got := r.Header.Get("new-api-user"); got != "264" {
				t.Fatalf("new-api-user = %q, want 264", got)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(`{
					"success": true,
					"data": {
						"all_subscriptions": [
							{"subscription":{"id":18,"amount_total":1,"amount_used":1,"amount_cap":2,"amount_cap_used":2}},
							{"subscription":{"id":17,"amount_total":300000000,"amount_used":3607724,"amount_cap":9000000000,"amount_cap_used":708408003,"next_reset_time":1784304000,"status":"active","allowed_group":"套餐专用分组"}}
						]
					},
					"message":""
				}`)),
			}, nil
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.String())
			return nil, nil
		}
	})}

	root := t.TempDir()
	manager, err := luadrv.NewManagedScriptStore(root)
	if err != nil {
		t.Fatalf("NewManagedScriptStore returned error: %v", err)
	}
	if err := manager.Save("stellaisle", `function fetch_usage(ctx)
  local credentials = ctx.host.json_decode(ctx.credential.api_key or "{}")
  local base = string.gsub(ctx.account.base_url or "", "/+$", "")
  local login = ctx.host.http_post({
    url = base .. "/api/user/login?turnstile=",
    headers = { ["Accept"] = "application/json", ["Content-Type"] = "application/json" },
    body = ctx.host.json_encode({ username = credentials.username, password = credentials.password }),
    retry_on_429 = true,
    retry_count = 2,
    retry_delay_ms = 0
  })
  local login_payload = ctx.host.json_decode(login.body)
  local cookie_parts = {}
  for _, raw_cookie in ipairs(login.set_cookies or {}) do
    local cookie = string.match(tostring(raw_cookie), "^[^;]+")
    if cookie ~= nil then table.insert(cookie_parts, cookie) end
  end
  local usage = ctx.host.http_get({
    url = base .. "/api/subscription/self",
    headers = { ["Accept"] = "application/json", ["Cookie"] = table.concat(cookie_parts, "; "), ["new-api-user"] = tostring(login_payload.data.id) }
  })
  local payload = ctx.host.json_decode(usage.body)
  local selected
  for _, entry in ipairs(payload.data.all_subscriptions or {}) do
    if entry.subscription.id == 17 then selected = entry.subscription break end
  end
  local scale = 1000000
  local period_total = selected.amount_total / scale
  local period_remaining = (selected.amount_total - selected.amount_used) / scale
  local total_cap = selected.amount_cap / scale
  local total_remaining = (selected.amount_cap - selected.amount_cap_used) / scale
  return {
    ok = true,
    source = "remote",
    confidence = "high",
    limits = { balance = period_total, quota_remaining = period_remaining },
    meta = { subscription_id = 17 },
    display = {
      summary = { label = "周额度", value = string.format("$%.2f / $%.2f", period_remaining, period_total) },
      usage_windows = {
        { label = "周额度", remaining_percent = period_remaining / period_total * 100, remaining_value = string.format("$%.2f", period_remaining), total_value = string.format("$%.2f", period_total) },
        { label = "总额度", remaining_percent = total_remaining / total_cap * 100, remaining_value = string.format("$%.2f", total_remaining), total_value = string.format("$%.2f", total_cap) }
      }
    }
  }
end
`); err != nil {
		t.Fatalf("Save Stellaisle script returned error: %v", err)
	}

	driver := luadrv.NewDriver(client, moduleRoot(t), luadrv.WithManagedScriptRoot(root))
	result, err := driver.Fetch(context.Background(), accounts.Account{
		BaseURL:         "https://token.stellaisle.com",
		UsageDriver:     "lua",
		UsageConfigJSON: `{"script":"managed:stellaisle","subscription_id":17}`,
	}, accountdrv.ResolvedCredential{APIKey: `{"username":"user@example.com","password":"pass-placeholder"}`})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if result.Limits.Balance == nil || *result.Limits.Balance != 300 {
		t.Fatalf("Balance = %#v, want 300", result.Limits.Balance)
	}
	if loginAttempts != 2 {
		t.Fatalf("login attempts = %d, want one retry after 429", loginAttempts)
	}
	if result.Limits.QuotaRemaining == nil || *result.Limits.QuotaRemaining != 296.392276 {
		t.Fatalf("QuotaRemaining = %#v, want 296.392276", result.Limits.QuotaRemaining)
	}
	if result.Meta["subscription_id"] != float64(17) {
		t.Fatalf("subscription_id = %#v, want 17", result.Meta["subscription_id"])
	}
	summary, ok := result.Display["summary"].(map[string]any)
	if !ok || summary["label"] != "周额度" || summary["value"] != "$296.39 / $300.00" {
		t.Fatalf("summary = %#v, want subscription usage summary", result.Display["summary"])
	}
	windows, ok := result.Display["usage_windows"].([]any)
	if !ok || len(windows) != 2 {
		t.Fatalf("usage_windows = %#v, want weekly and total windows", result.Display["usage_windows"])
	}
	weekly, weeklyOK := windows[0].(map[string]any)
	total, totalOK := windows[1].(map[string]any)
	if !weeklyOK || weekly["label"] != "周额度" || weekly["remaining_value"] != "$296.39" || weekly["total_value"] != "$300.00" {
		t.Fatalf("weekly window = %#v, want weekly remaining/total values", windows[0])
	}
	if !totalOK || total["label"] != "总额度" || total["remaining_value"] != "$8291.59" || total["total_value"] != "$9000.00" {
		t.Fatalf("total window = %#v, want total remaining/total values", windows[1])
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
