package bootstrap_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/bootstrap"
	"github.com/gcssloop/codex-router/backend/internal/store/sqlite"
)

func TestNewApp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	app, err := bootstrap.NewApp(context.Background(), bootstrap.Config{
		ListenAddr:   "127.0.0.1:0",
		DatabasePath: t.TempDir() + "/router.sqlite",
	})
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = app.Close()
	})
	if app == nil {
		t.Fatal("NewApp returned nil app")
	}

	rootReq := httptest.NewRequest(http.MethodGet, "/", nil)
	rootRec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rootRec, rootReq)
	if rootRec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("GET / status = %d, want %d", rootRec.Code, http.StatusTemporaryRedirect)
	}
	if location := rootRec.Header().Get("Location"); location != "/ai-router/webui/" {
		t.Fatalf("GET / location = %q, want %q", location, "/ai-router/webui/")
	}

	apiReq := httptest.NewRequest(http.MethodGet, "/ai-router/api/accounts", nil)
	apiRec := httptest.NewRecorder()
	app.Handler().ServeHTTP(apiRec, apiReq)
	if apiRec.Code != http.StatusOK {
		t.Fatalf("GET /ai-router/api/accounts status = %d, want %d", apiRec.Code, http.StatusOK)
	}

	qualityReq := httptest.NewRequest(http.MethodGet, "/ai-router/api/dashboard/request-quality?range=24h", nil)
	qualityRec := httptest.NewRecorder()
	app.Handler().ServeHTTP(qualityRec, qualityReq)
	if qualityRec.Code != http.StatusOK {
		t.Fatalf("GET /ai-router/api/dashboard/request-quality status = %d, want %d; body=%s", qualityRec.Code, http.StatusOK, qualityRec.Body.String())
	}

	responsesReq := httptest.NewRequest(http.MethodPost, "/ai-router/api/responses", nil)
	responsesRec := httptest.NewRecorder()
	app.Handler().ServeHTTP(responsesRec, responsesReq)
	if responsesRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("POST /ai-router/api/responses status = %d, want %d when proxy disabled", responsesRec.Code, http.StatusServiceUnavailable)
	}

	backupCreateReq := httptest.NewRequest(http.MethodPost, "/ai-router/api/settings/database/backup", nil)
	backupCreateRec := httptest.NewRecorder()
	app.Handler().ServeHTTP(backupCreateRec, backupCreateReq)
	if backupCreateRec.Code != http.StatusCreated {
		t.Fatalf("POST /ai-router/api/settings/database/backup status = %d, want %d; body=%s", backupCreateRec.Code, http.StatusCreated, backupCreateRec.Body.String())
	}

	backupListReq := httptest.NewRequest(http.MethodGet, "/ai-router/api/settings/database/backups", nil)
	backupListRec := httptest.NewRecorder()
	app.Handler().ServeHTTP(backupListRec, backupListReq)
	if backupListRec.Code != http.StatusOK {
		t.Fatalf("GET /ai-router/api/settings/database/backups status = %d, want %d; body=%s", backupListRec.Code, http.StatusOK, backupListRec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/ai-router/api/settings/database/backups", bytes.NewBuffer(nil))
	deleteReq.URL.Path = "/ai-router/api/settings/database/backups/"
	deleteRec := httptest.NewRecorder()
	app.Handler().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code == http.StatusNotFound {
		t.Fatalf("DELETE /ai-router/api/settings/database/backups/{id} unexpectedly returned 404; body=%s", deleteRec.Body.String())
	}

	if err := os.MkdirAll(filepath.Join(home, ".aigate", "data", "tooling", "skills", "Humanizer-zh"), 0o755); err != nil {
		t.Fatalf("MkdirAll managed skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".aigate", "data", "tooling", "skills", "Humanizer-zh", "SKILL.md"), []byte("Humanizer"), 0o644); err != nil {
		t.Fatalf("WriteFile managed skill: %v", err)
	}
	toolingReq := httptest.NewRequest(http.MethodPut, "/ai-router/api/tooling/skills/Humanizer-zh", bytes.NewBufferString(`{"apps":["codex"],"enabled":false}`))
	toolingReq.Header.Set("Content-Type", "application/json")
	toolingRec := httptest.NewRecorder()
	app.Handler().ServeHTTP(toolingRec, toolingReq)
	if toolingRec.Code != http.StatusOK {
		t.Fatalf("PUT /ai-router/api/tooling/skills/Humanizer-zh status = %d, want %d; body=%s", toolingRec.Code, http.StatusOK, toolingRec.Body.String())
	}

	mcpImportReq := httptest.NewRequest(http.MethodPost, "/ai-router/api/tooling/mcp/import", bytes.NewBufferString(`{"source":"codex"}`))
	mcpImportReq.Header.Set("Content-Type", "application/json")
	mcpImportRec := httptest.NewRecorder()
	app.Handler().ServeHTTP(mcpImportRec, mcpImportReq)
	if mcpImportRec.Code == http.StatusNotFound {
		t.Fatalf("POST /ai-router/api/tooling/mcp/import unexpectedly returned 404; body=%s", mcpImportRec.Body.String())
	}
}

func TestNewAppServerModeUsesAIGatePrefixAndGateway(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	app, err := bootstrap.NewApp(context.Background(), bootstrap.Config{
		ListenAddr:             "127.0.0.1:0",
		DatabasePath:           filepath.Join(t.TempDir(), "router.sqlite"),
		ServerMode:             true,
		HTTPPrefix:             "/ai-gate",
		ProxyEnabledByDefault:  true,
		SkipCodexConfigChanges: true,
		ServerPassword:         "server-secret",
	})
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = app.Close()
	})

	rootReq := httptest.NewRequest(http.MethodGet, "/", nil)
	rootRec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rootRec, rootReq)
	if rootRec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("GET / status = %d, want %d", rootRec.Code, http.StatusTemporaryRedirect)
	}
	if location := rootRec.Header().Get("Location"); location != "/ai-gate/webui/" {
		t.Fatalf("GET / location = %q, want %q", location, "/ai-gate/webui/")
	}

	clientAPIReq := httptest.NewRequest(http.MethodGet, "/ai-router/api/accounts", nil)
	clientAPIRec := httptest.NewRecorder()
	app.Handler().ServeHTTP(clientAPIRec, clientAPIReq)
	if clientAPIRec.Code != http.StatusNotFound {
		t.Fatalf("GET /ai-router/api/accounts status = %d, want %d in server mode", clientAPIRec.Code, http.StatusNotFound)
	}

	serverAPIReq := httptest.NewRequest(http.MethodGet, "/ai-gate/api/accounts", nil)
	serverAPIRec := httptest.NewRecorder()
	app.Handler().ServeHTTP(serverAPIRec, serverAPIReq)
	if serverAPIRec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /ai-gate/api/accounts status = %d, want %d before login; body=%s", serverAPIRec.Code, http.StatusUnauthorized, serverAPIRec.Body.String())
	}

	loginReq := httptest.NewRequest(http.MethodPost, "/ai-gate/auth/login", strings.NewReader(`{"password":"server-secret"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	app.Handler().ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("POST /ai-gate/auth/login status = %d, want %d; body=%s", loginRec.Code, http.StatusOK, loginRec.Body.String())
	}
	cookies := loginRec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("POST /ai-gate/auth/login returned no cookies")
	}

	authedAPIReq := httptest.NewRequest(http.MethodGet, "/ai-gate/api/accounts", nil)
	authedAPIReq.AddCookie(cookies[0])
	authedAPIRec := httptest.NewRecorder()
	app.Handler().ServeHTTP(authedAPIRec, authedAPIReq)
	if authedAPIRec.Code != http.StatusOK {
		t.Fatalf("GET /ai-gate/api/accounts status = %d, want %d after login; body=%s", authedAPIRec.Code, http.StatusOK, authedAPIRec.Body.String())
	}

	webReq := httptest.NewRequest(http.MethodGet, "/ai-gate/webui/", nil)
	webRec := httptest.NewRecorder()
	app.Handler().ServeHTTP(webRec, webReq)
	if webRec.Code != http.StatusOK {
		t.Fatalf("GET /ai-gate/webui/ status = %d, want %d; body=%s", webRec.Code, http.StatusOK, webRec.Body.String())
	}
	if !strings.Contains(webRec.Body.String(), `<div id="root"></div>`) {
		t.Fatalf("GET /ai-gate/webui/ body = %q, want embedded web ui index", webRec.Body.String())
	}

	gatewayReq := httptest.NewRequest(http.MethodPost, "/ai-gate/v1/responses", nil)
	gatewayRec := httptest.NewRecorder()
	app.Handler().ServeHTTP(gatewayRec, gatewayReq)
	if gatewayRec.Code != http.StatusUnauthorized {
		t.Fatalf("POST /ai-gate/v1/responses status = %d, want %d without token", gatewayRec.Code, http.StatusUnauthorized)
	}

	createUserReq := httptest.NewRequest(http.MethodPost, "/ai-gate/api/server-users", strings.NewReader(`{"name":"alice"}`))
	createUserReq.Header.Set("Content-Type", "application/json")
	createUserReq.AddCookie(cookies[0])
	createUserRec := httptest.NewRecorder()
	app.Handler().ServeHTTP(createUserRec, createUserReq)
	if createUserRec.Code != http.StatusCreated {
		t.Fatalf("POST /ai-gate/api/server-users status = %d, want %d; body=%s", createUserRec.Code, http.StatusCreated, createUserRec.Body.String())
	}
	var createdUser struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(createUserRec.Body.Bytes(), &createdUser); err != nil {
		t.Fatalf("unmarshal created server user: %v", err)
	}
	if createdUser.Token == "" {
		t.Fatal("created server user returned empty token")
	}

	authedGatewayReq := httptest.NewRequest(http.MethodPost, "/ai-gate/v1/responses", nil)
	authedGatewayReq.Header.Set("Authorization", "Bearer "+createdUser.Token)
	authedGatewayRec := httptest.NewRecorder()
	app.Handler().ServeHTTP(authedGatewayRec, authedGatewayReq)
	if authedGatewayRec.Code == http.StatusUnauthorized || authedGatewayRec.Code == http.StatusServiceUnavailable {
		t.Fatalf("POST /ai-gate/v1/responses status = %d, want request past auth/proxy gates; body=%s", authedGatewayRec.Code, authedGatewayRec.Body.String())
	}
}

func TestNewAppServerModePersistsAdminPasswordEncryptedInDataDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	dbPath := filepath.Join(root, "data", "aigate.sqlite")
	passwordPath := filepath.Join(root, "data", "server-password.enc")

	app, err := bootstrap.NewApp(context.Background(), bootstrap.Config{
		ListenAddr:             "127.0.0.1:0",
		DatabasePath:           dbPath,
		ServerMode:             true,
		HTTPPrefix:             "/ai-gate",
		ProxyEnabledByDefault:  true,
		SkipCodexConfigChanges: true,
		ServerPassword:         "server-secret",
	})
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}

	loginReq := httptest.NewRequest(http.MethodPost, "/ai-gate/auth/login", strings.NewReader(`{"password":"server-secret"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	app.Handler().ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("POST /ai-gate/auth/login status = %d, want %d; body=%s", loginRec.Code, http.StatusOK, loginRec.Body.String())
	}
	cookies := loginRec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("POST /ai-gate/auth/login returned no cookies")
	}

	changeReq := httptest.NewRequest(http.MethodPut, "/ai-gate/auth/password", strings.NewReader(`{"current_password":"server-secret","new_password":"new-secret"}`))
	changeReq.Header.Set("Content-Type", "application/json")
	changeReq.AddCookie(cookies[0])
	changeRec := httptest.NewRecorder()
	app.Handler().ServeHTTP(changeRec, changeReq)
	if changeRec.Code != http.StatusOK {
		t.Fatalf("PUT /ai-gate/auth/password status = %d, want %d; body=%s", changeRec.Code, http.StatusOK, changeRec.Body.String())
	}
	if err := app.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	encrypted, err := os.ReadFile(passwordPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) returned error: %v", passwordPath, err)
	}
	if bytes.Contains(encrypted, []byte("server-secret")) || bytes.Contains(encrypted, []byte("new-secret")) {
		t.Fatalf("password store contains plaintext password: %s", string(encrypted))
	}

	restarted, err := bootstrap.NewApp(context.Background(), bootstrap.Config{
		ListenAddr:             "127.0.0.1:0",
		DatabasePath:           dbPath,
		ServerMode:             true,
		HTTPPrefix:             "/ai-gate",
		ProxyEnabledByDefault:  true,
		SkipCodexConfigChanges: true,
		ServerPassword:         "server-secret",
	})
	if err != nil {
		t.Fatalf("NewApp after restart returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = restarted.Close()
	})

	oldLoginReq := httptest.NewRequest(http.MethodPost, "/ai-gate/auth/login", strings.NewReader(`{"password":"server-secret"}`))
	oldLoginReq.Header.Set("Content-Type", "application/json")
	oldLoginRec := httptest.NewRecorder()
	restarted.Handler().ServeHTTP(oldLoginRec, oldLoginReq)
	if oldLoginRec.Code != http.StatusUnauthorized {
		t.Fatalf("old password login status = %d, want %d", oldLoginRec.Code, http.StatusUnauthorized)
	}

	newLoginReq := httptest.NewRequest(http.MethodPost, "/ai-gate/auth/login", strings.NewReader(`{"password":"new-secret"}`))
	newLoginReq.Header.Set("Content-Type", "application/json")
	newLoginRec := httptest.NewRecorder()
	restarted.Handler().ServeHTTP(newLoginRec, newLoginReq)
	if newLoginRec.Code != http.StatusOK {
		t.Fatalf("new password login status = %d, want %d; body=%s", newLoginRec.Code, http.StatusOK, newLoginRec.Body.String())
	}
}

func TestNewAppServerModeRequiresPassword(t *testing.T) {
	_, err := bootstrap.NewApp(context.Background(), bootstrap.Config{
		ListenAddr:   "127.0.0.1:0",
		DatabasePath: filepath.Join(t.TempDir(), "router.sqlite"),
		ServerMode:   true,
		HTTPPrefix:   "/ai-gate",
	})
	if err == nil {
		t.Fatal("NewApp returned nil error, want missing password error")
	}
	if !strings.Contains(err.Error(), "server password") {
		t.Fatalf("NewApp error = %q, want server password", err.Error())
	}
}

func TestNewAppSchedulesAutomaticDatabaseBackups(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	root := t.TempDir()
	dbPath := filepath.Join(root, "router.sqlite")
	app, err := bootstrap.NewApp(context.Background(), bootstrap.Config{
		ListenAddr:        "127.0.0.1:0",
		DatabasePath:      dbPath,
		SchedulerInterval: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = app.Close()
	})

	backupPath := filepath.Join(root, "backups", "db")
	deadline := time.Now().Add(1 * time.Second)
	for {
		entries, readErr := os.ReadDir(backupPath)
		if readErr == nil && len(entries) > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected automatic backup files in %s", backupPath)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestNewAppSchedulesUsageRefreshOrchestrator(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	usageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk-refresh" {
			t.Fatalf("Authorization = %q, want Bearer sk-refresh", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"quota_remaining":321}`))
	}))
	defer usageServer.Close()

	dbPath := filepath.Join(t.TempDir(), "router.sqlite")
	seedUsageAccount(t, dbPath, usageServer.URL, "lua-refresh", "sk-refresh")
	app, err := bootstrap.NewApp(context.Background(), bootstrap.Config{
		ListenAddr:        "127.0.0.1:0",
		DatabasePath:      dbPath,
		SchedulerInterval: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = app.Close()
	})

	deadline := time.Now().Add(1 * time.Second)
	var lastStatus int
	var lastBody string
	for {
		req := httptest.NewRequest(http.MethodGet, "/ai-router/api/accounts/usage", nil)
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, req)
		lastStatus = rec.Code
		lastBody = rec.Body.String()
		if rec.Code == http.StatusOK {
			var items []map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
				t.Fatalf("json.Unmarshal returned error: %v", err)
			}
			if len(items) == 1 && items[0]["quota_remaining"] == float64(321) {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("usage refresh did not populate snapshot before deadline: status=%d body=%s", lastStatus, lastBody)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestNewAppRefreshesUsageImmediatelyOnStartup(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	usageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk-immediate" {
			t.Fatalf("Authorization = %q, want Bearer sk-immediate", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"quota_remaining":654}`))
	}))
	defer usageServer.Close()

	dbPath := filepath.Join(t.TempDir(), "router.sqlite")
	seedUsageAccount(t, dbPath, usageServer.URL, "lua-immediate", "sk-immediate")

	app, err := bootstrap.NewApp(context.Background(), bootstrap.Config{
		ListenAddr:        "127.0.0.1:0",
		DatabasePath:      dbPath,
		SchedulerInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("restart NewApp returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = app.Close()
	})

	deadline := time.Now().Add(250 * time.Millisecond)
	for {
		req := httptest.NewRequest(http.MethodGet, "/ai-router/api/accounts/usage", nil)
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, req)
		if rec.Code == http.StatusOK {
			var items []map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
				t.Fatalf("json.Unmarshal returned error: %v", err)
			}
			if len(items) == 1 && items[0]["quota_remaining"] == float64(654) {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("usage refresh did not run immediately on startup: status=%d body=%s", rec.Code, rec.Body.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func seedUsageAccount(t *testing.T, dbPath, usageServerURL, accountName, credential string) {
	t.Helper()

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("sqlite.Open returned error: %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	repo := accounts.NewSQLiteRepository(store.DB())
	err = repo.Create(accounts.Account{
		ProviderType:    accounts.ProviderOpenAICompatible,
		AccountName:     accountName,
		AuthMode:        accounts.AuthModeAPIKey,
		CredentialRef:   credential,
		UsageDriver:     "lua",
		UsageConfigJSON: "{\"script\":\"internal/usagedrv/lua/testdata/vendor_x.lua\",\"endpoint\":\"" + usageServerURL + "/usage\"}",
		BaseURL:         "https://mirror.example.test/v1",
		Status:          accounts.StatusActive,
		Priority:        1,
	})
	if err != nil {
		t.Fatalf("seed usage account returned error: %v", err)
	}
}
