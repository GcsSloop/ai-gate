package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accountdrv"
	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/api"
	"github.com/gcssloop/codex-router/backend/internal/auth"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
	"github.com/gcssloop/codex-router/backend/internal/usagedrv"
	luadrv "github.com/gcssloop/codex-router/backend/internal/usagedrv/lua"
	"github.com/gcssloop/codex-router/backend/internal/usagedrv/registry"
)

func TestAccountsHandlerManagedLuaScriptCRUD(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	scriptRoot := filepath.Join(t.TempDir(), "usage-scripts")
	handler := api.NewAccountsHandler(
		repo,
		nil,
		auth.NewOAuthConnector(auth.Config{}),
		auth.NewStateStore(5*time.Minute),
		api.WithAccountsLuaScriptRoot(scriptRoot),
	)

	putReq := httptest.NewRequest(http.MethodPut, "/accounts/usage-scripts/vendor_shared", bytes.NewBufferString(`{
		"content":"function fetch_usage(ctx)\n  return { ok = true, source = \"remote\", confidence = \"high\", limits = { quota_remaining = 123 } }\nend\n"
	}`))
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	handler.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT /accounts/usage-scripts/vendor_shared status = %d, want %d body=%s", putRec.Code, http.StatusOK, putRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/accounts/usage-scripts/vendor_shared", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /accounts/usage-scripts/vendor_shared status = %d, want %d body=%s", getRec.Code, http.StatusOK, getRec.Body.String())
	}

	var payload struct {
		Key     string `json:"key"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if payload.Key != "vendor_shared" {
		t.Fatalf("key = %q, want vendor_shared", payload.Key)
	}
	if !strings.Contains(payload.Content, "fetch_usage") {
		t.Fatalf("content = %q, want fetch_usage body", payload.Content)
	}

	missingReq := httptest.NewRequest(http.MethodGet, "/accounts/usage-scripts/stellaisle", nil)
	missingRec := httptest.NewRecorder()
	handler.ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("GET unmanaged stellaisle script status = %d, want %d body=%s", missingRec.Code, http.StatusNotFound, missingRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/accounts/usage-scripts", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("GET /accounts/usage-scripts status = %d, want %d body=%s", listRec.Code, http.StatusOK, listRec.Body.String())
	}
	var listed struct {
		Items []string `json:"items"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("Unmarshal list returned error: %v", err)
	}
	if len(listed.Items) != 1 || listed.Items[0] != "vendor_shared" {
		t.Fatalf("items = %#v, want [vendor_shared]", listed.Items)
	}
}

func TestAccountsHandlerLuaUsageTestExecutesWithRealCredential(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk-live" {
			t.Fatalf("Authorization = %q, want Bearer sk-live", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"quota_remaining":4321,"rpm_remaining":21}`))
	}))
	defer upstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	if err := repo.Create(accounts.Account{
		ProviderType:    accounts.ProviderOpenAICompatible,
		AccountName:     "vendor-lua",
		AuthMode:        accounts.AuthModeAPIKey,
		BaseURL:         "https://mirror.example.test/v1",
		CredentialRef:   "sk-live",
		AccountDriver:   "builtin_api_key",
		UsageDriver:     "lua",
		UsageConfigJSON: `{"script":"managed:vendor_shared","endpoint":"` + upstream.URL + `/usage"}`,
		Status:          accounts.StatusActive,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	scriptRoot := filepath.Join(t.TempDir(), "usage-scripts")
	luaDriver := luadrv.NewDriver(http.DefaultClient, "", luadrv.WithManagedScriptRoot(scriptRoot))
	driverRegistry, err := registry.New(
		[]accountdrv.AccountDriver{
			accountdrv.NewOfficialDriver(http.DefaultClient, repo),
			accountdrv.NewAPIKeyDriver(),
		},
		[]usagedrv.UsageDriver{luaDriver},
	)
	if err != nil {
		t.Fatalf("registry.New returned error: %v", err)
	}

	handler := api.NewAccountsHandler(
		repo,
		nil,
		auth.NewOAuthConnector(auth.Config{}),
		auth.NewStateStore(5*time.Minute),
		api.WithAccountsLuaScriptRoot(scriptRoot),
		api.WithAccountsDriverRegistry(driverRegistry),
	)

	seedReq := httptest.NewRequest(http.MethodPut, "/accounts/usage-scripts/vendor_shared", bytes.NewBufferString(`{
		"content":"function fetch_usage(ctx)\n  local response = ctx.host.http_get({ url = ctx.config.endpoint, headers = { Authorization = 'Bearer ' .. ctx.credential.access_token } })\n  local payload = ctx.host.json_decode(response.body)\n  return { ok = true, source = 'remote', confidence = 'high', limits = { quota_remaining = payload.quota_remaining, rpm_remaining = payload.rpm_remaining }, meta = { account_name = ctx.account.account_name } }\nend\n"
	}`))
	seedReq.Header.Set("Content-Type", "application/json")
	seedRec := httptest.NewRecorder()
	handler.ServeHTTP(seedRec, seedReq)
	if seedRec.Code != http.StatusOK {
		t.Fatalf("seed PUT status = %d, want %d body=%s", seedRec.Code, http.StatusOK, seedRec.Body.String())
	}

	testReq := httptest.NewRequest(http.MethodPost, "/accounts/1/usage-lua-test", bytes.NewBufferString(`{
		"usage_config_json":"{\"script\":\"managed:vendor_shared\",\"endpoint\":\"`+upstream.URL+`/usage\"}",
		"script_content":"function fetch_usage(ctx)\n  local response = ctx.host.http_get({ url = ctx.config.endpoint, headers = { Authorization = 'Bearer ' .. ctx.credential.access_token } })\n  local payload = ctx.host.json_decode(response.body)\n  return { ok = true, source = 'remote', confidence = 'high', limits = { quota_remaining = payload.quota_remaining, rpm_remaining = payload.rpm_remaining }, meta = { account_name = ctx.account.account_name } }\nend\n"
	}`))
	testReq.Header.Set("Content-Type", "application/json")
	testRec := httptest.NewRecorder()
	handler.ServeHTTP(testRec, testReq)
	if testRec.Code != http.StatusOK {
		t.Fatalf("POST /accounts/1/usage-lua-test status = %d, want %d body=%s", testRec.Code, http.StatusOK, testRec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(testRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if payload["ok"] != true {
		t.Fatalf("ok = %v, want true", payload["ok"])
	}
	content, _ := payload["content"].(string)
	if !strings.Contains(content, `"quota_remaining": 4321`) {
		t.Fatalf("content = %q, want quota_remaining output", content)
	}
	if !strings.Contains(content, `"account_name": "vendor-lua"`) {
		t.Fatalf("content = %q, want account_name in meta", content)
	}
}

func TestAccountsHandlerLuaUsageTestRejectsMalformedConfig(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	if err := repo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "vendor-lua",
		AuthMode:      accounts.AuthModeAPIKey,
		CredentialRef: "sk-live",
		UsageDriver:   "lua",
		Status:        accounts.StatusActive,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	driverRegistry, err := registry.New(
		[]accountdrv.AccountDriver{accountdrv.NewAPIKeyDriver()},
		[]usagedrv.UsageDriver{luadrv.NewDriver(http.DefaultClient, "")},
	)
	if err != nil {
		t.Fatalf("registry.New returned error: %v", err)
	}

	handler := api.NewAccountsHandler(
		repo,
		nil,
		auth.NewOAuthConnector(auth.Config{}),
		auth.NewStateStore(5*time.Minute),
		api.WithAccountsDriverRegistry(driverRegistry),
	)

	req := httptest.NewRequest(http.MethodPost, "/accounts/1/usage-lua-test", bytes.NewBufferString(`{"usage_config_json":"{"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /accounts/1/usage-lua-test status = %d, want %d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}
