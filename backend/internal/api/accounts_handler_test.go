package api_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accountdrv"
	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/api"
	"github.com/gcssloop/codex-router/backend/internal/auth"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
	"github.com/gcssloop/codex-router/backend/internal/usage"
	"github.com/gcssloop/codex-router/backend/internal/usage/refresh"
	"github.com/gcssloop/codex-router/backend/internal/usagedrv"
	"github.com/gcssloop/codex-router/backend/internal/usagedrv/builtin"
	"github.com/gcssloop/codex-router/backend/internal/usagedrv/registry"
)

func TestAccountsHandlerAuthorizeReturnsDeviceCode(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	deviceAuthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("device auth method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/device-auth" {
			t.Fatalf("device auth path = %s, want /device-auth", r.URL.Path)
		}
		writeJSON := func(value any) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(value)
		}
		writeJSON(map[string]any{
			"device_auth_id": "dev-auth-1",
			"user_code":      "ABCD-EFGH",
			"expires_in":     900,
			"interval":       5,
		})
	}))
	t.Cleanup(deviceAuthServer.Close)

	connector := auth.NewOAuthConnector(auth.Config{
		ClientID:              "client-id",
		AuthorizeURL:          "https://auth.example.test/oauth/authorize",
		TokenURL:              "https://auth.example.test/oauth/token",
		RedirectURL:           "http://localhost:8080/callback",
		Scopes:                []string{"model.read"},
		DeviceAuthUserCodeURL: deviceAuthServer.URL + "/device-auth",
		DeviceVerificationURL: "https://auth.openai.com/codex/device",
	})
	handler := api.NewAccountsHandler(
		repo,
		nil,
		connector,
		auth.NewStateStore(5*time.Minute),
		api.WithAccountsHTTPClient(deviceAuthServer.Client()),
	)

	req := httptest.NewRequest(http.MethodPost, "/accounts/auth/authorize", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /accounts/auth/authorize status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if payload["user_code"] != "ABCD-EFGH" {
		t.Fatalf("user_code = %v, want ABCD-EFGH", payload["user_code"])
	}
	if payload["device_code"] != "dev-auth-1" {
		t.Fatalf("device_code = %v, want dev-auth-1", payload["device_code"])
	}
	if payload["verification_uri"] != "https://auth.openai.com/codex/device" {
		t.Fatalf("verification_uri = %v, want https://auth.openai.com/codex/device", payload["verification_uri"])
	}
	if _, ok := payload["authorization_url"]; !ok {
		t.Fatalf("authorization_url missing from response: %#v", payload)
	}
}

func TestAccountsHandlerAuthorizeReturnsErrorWhenDeviceAuthUnavailable(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	deviceAuthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "temporary upstream failure", http.StatusBadGateway)
	}))
	t.Cleanup(deviceAuthServer.Close)

	connector := auth.NewOAuthConnector(auth.Config{
		ClientID:              "client-id",
		AuthorizeURL:          "https://auth.example.test/oauth/authorize",
		TokenURL:              "https://auth.example.test/oauth/token",
		RedirectURL:           "http://localhost:8080/callback",
		Scopes:                []string{"model.read"},
		DeviceAuthUserCodeURL: deviceAuthServer.URL + "/device-auth",
	})
	handler := api.NewAccountsHandler(
		repo,
		nil,
		connector,
		auth.NewStateStore(5*time.Minute),
		api.WithAccountsHTTPClient(deviceAuthServer.Client()),
	)

	req := httptest.NewRequest(http.MethodPost, "/accounts/auth/authorize", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("POST /accounts/auth/authorize status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	if !strings.Contains(rec.Body.String(), "start device auth:") {
		t.Fatalf("POST /accounts/auth/authorize body = %q, want prefixed error", rec.Body.String())
	}
}

func TestAccountsHandlerCompleteDeviceAuthUsesInferredName(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	oauthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON := func(value any) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(value)
		}
		switch r.URL.Path {
		case "/device-token":
			writeJSON(map[string]any{
				"authorization_code": "auth-code-1",
				"code_verifier":      "verifier-1",
			})
			return
		case "/oauth-token":
			writeJSON(map[string]any{
				"access_token": testJWT(t, map[string]any{
					"https://api.openai.com/auth": map[string]any{
						"chatgpt_account_id": "acct-1",
					},
				}),
				"id_token": testJWT(t, map[string]any{
					"name": "OpenAI Test User",
				}),
				"refresh_token": "refresh-1",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	t.Cleanup(oauthServer.Close)

	connector := auth.NewOAuthConnector(auth.Config{
		ClientID:              "client-id",
		TokenURL:              oauthServer.URL + "/oauth-token",
		DeviceAuthTokenURL:    oauthServer.URL + "/device-token",
		DeviceRedirectURL:     "https://auth.openai.com/deviceauth/callback",
		DeviceVerificationURL: "https://auth.openai.com/codex/device",
	})
	handler := api.NewAccountsHandler(
		repo,
		nil,
		connector,
		auth.NewStateStore(5*time.Minute),
		api.WithAccountsHTTPClient(oauthServer.Client()),
	)

	req := httptest.NewRequest(http.MethodPost, "/accounts/auth/device/complete", bytes.NewBufferString(`{
		"device_code":"dev-1",
		"user_code":"ABCD-EFGH"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /accounts/auth/device/complete status = %d, want %d", rec.Code, http.StatusCreated)
	}

	listed, err := repo.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("List returned %d accounts, want 1", len(listed))
	}
	if listed[0].AccountName != "OpenAI Test User" {
		t.Fatalf("AccountName = %q, want %q", listed[0].AccountName, "OpenAI Test User")
	}
}

func TestAccountsHandler(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	usageRepo := usage.NewSQLiteRepository(store.DB())
	deviceAuthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON := func(value any) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(value)
		}
		switch r.URL.Path {
		case "/device-auth":
			writeJSON(map[string]any{
				"device_auth_id": "dev-auth-generic",
				"user_code":      "WXYZ-1234",
				"expires_in":     900,
				"interval":       5,
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	t.Cleanup(deviceAuthServer.Close)

	connector := auth.NewOAuthConnector(auth.Config{
		ClientID:              "client-id",
		AuthorizeURL:          "https://auth.example.test/oauth/authorize",
		TokenURL:              "https://auth.example.test/oauth/token",
		RedirectURL:           "http://localhost:8080/callback",
		Scopes:                []string{"model.read"},
		DeviceAuthUserCodeURL: deviceAuthServer.URL + "/device-auth",
	})
	handler := api.NewAccountsHandler(
		repo,
		usageRepo,
		connector,
		auth.NewStateStore(5*time.Minute),
		api.WithAccountsHTTPClient(deviceAuthServer.Client()),
	)

	createBody := bytes.NewBufferString(`{
		"provider_type":"openai-compatible",
		"account_name":"mirror-east",
		"auth_mode":"api_key",
		"base_url":"https://mirror.example.test/v1",
		"credential_ref":"cred-api-key",
		"account_driver":"custom-account-driver",
		"usage_driver":"lua",
		"usage_config_json":"{\"script\":\"adapters/vendor.lua\"}"
	}`)
	createReq := httptest.NewRequest(http.MethodPost, "/accounts", createBody)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("POST /accounts status = %d, want %d", createRec.Code, http.StatusCreated)
	}

	authReq := httptest.NewRequest(http.MethodPost, "/accounts/auth/authorize", bytes.NewBufferString(`{}`))
	authReq.Header.Set("Content-Type", "application/json")
	authRec := httptest.NewRecorder()
	handler.ServeHTTP(authRec, authReq)
	if authRec.Code != http.StatusOK {
		t.Fatalf("POST /accounts/auth/authorize status = %d, want %d", authRec.Code, http.StatusOK)
	}

	cooldownUntil := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	if err := repo.Create(accounts.Account{
		ProviderType:   accounts.ProviderOpenAIOfficial,
		AccountName:    "official-secondary",
		AuthMode:       accounts.AuthModeOAuth,
		CredentialRef:  "cred-oauth",
		Status:         accounts.StatusActive,
		CooldownUntil:  &cooldownUntil,
		CooldownReason: "rate_limited",
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if err := usageRepo.Save(usage.Snapshot{
		AccountID:       2,
		Balance:         88.5,
		QuotaRemaining:  12000,
		RPMRemaining:    42,
		TPMRemaining:    18000,
		HealthScore:     0.91,
		RecentErrorRate: 0.01,
	}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/accounts", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("GET /accounts status = %d, want %d", listRec.Code, http.StatusOK)
	}

	var listed []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("GET /accounts returned %d items, want 2", len(listed))
	}
	if _, ok := listed[0]["allow_chat_fallback"]; ok {
		t.Fatalf("GET /accounts item = %+v, want no allow_chat_fallback field", listed[0])
	}
	if listed[0]["source_icon"] != "openai" {
		t.Fatalf("source_icon = %v, want openai", listed[0]["source_icon"])
	}
	if listed[0]["account_driver"] != "custom-account-driver" {
		t.Fatalf("account_driver = %v, want custom-account-driver", listed[0]["account_driver"])
	}
	if listed[0]["usage_driver"] != "lua" {
		t.Fatalf("usage_driver = %v, want lua", listed[0]["usage_driver"])
	}
	if listed[0]["usage_config_json"] != "{\"script\":\"adapters/vendor.lua\"}" {
		t.Fatalf("usage_config_json = %v, want serialized config", listed[0]["usage_config_json"])
	}
	if listed[1]["status"] != string(accounts.StatusActive) {
		t.Fatalf("status = %v, want active", listed[1]["status"])
	}
	if listed[1]["routing_cooldown_remaining_seconds"] == nil {
		t.Fatal("routing_cooldown_remaining_seconds missing from routed cooldown account")
	}
	if listed[1]["balance"].(float64) != 0 {
		t.Fatalf("balance = %v, want 0", listed[1]["balance"])
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/accounts/usage", nil)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("GET /accounts/usage status = %d, want %d", usageRec.Code, http.StatusOK)
	}

	var usageItems []map[string]any
	if err := json.Unmarshal(usageRec.Body.Bytes(), &usageItems); err != nil {
		t.Fatalf("json.Unmarshal usage returned error: %v", err)
	}
	if usageItems[1]["balance"].(float64) != 88.5 {
		t.Fatalf("usage balance = %v, want 88.5", usageItems[1]["balance"])
	}

	disableReq := httptest.NewRequest(http.MethodPost, "/accounts/1/disable", nil)
	disableRec := httptest.NewRecorder()
	handler.ServeHTTP(disableRec, disableReq)
	if disableRec.Code != http.StatusNoContent {
		t.Fatalf("POST /accounts/1/disable status = %d, want %d", disableRec.Code, http.StatusNoContent)
	}
}

func testJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	headerRaw, err := json.Marshal(map[string]any{"alg": "none", "typ": "JWT"})
	if err != nil {
		t.Fatalf("Marshal header returned error: %v", err)
	}
	claimsRaw, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("Marshal claims returned error: %v", err)
	}
	return encodeJWTPart(headerRaw) + "." + encodeJWTPart(claimsRaw) + "."
}

func encodeJWTPart(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}

func TestAccountsHandlerImportLocalCodexAuth(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	connector := auth.NewOAuthConnector(auth.Config{})
	handler := api.NewAccountsHandler(repo, nil, connector, auth.NewStateStore(5*time.Minute))

	authPath := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(authPath, []byte(`{
		"auth_mode":"chatgpt",
		"tokens":{"access_token":"token-1","account_id":"acct-1"}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/accounts/import-local", bytes.NewBufferString(`{
		"path":"`+authPath+`",
		"account_name":"local-codex"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /accounts/import-local status = %d, want %d", rec.Code, http.StatusCreated)
	}

	listed, err := repo.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("List returned %d accounts, want 1", len(listed))
	}
	if listed[0].AuthMode != accounts.AuthModeLocalImport {
		t.Fatalf("AuthMode = %q, want %q", listed[0].AuthMode, accounts.AuthModeLocalImport)
	}
	if listed[0].BaseURL != "https://chatgpt.com/backend-api/codex" {
		t.Fatalf("BaseURL = %q, want https://chatgpt.com/backend-api/codex", listed[0].BaseURL)
	}
	if listed[0].AccountDriver != "builtin_openai_official_session" {
		t.Fatalf("AccountDriver = %q, want builtin_openai_official_session", listed[0].AccountDriver)
	}
	if listed[0].UsageDriver != "builtin_openai_official" {
		t.Fatalf("UsageDriver = %q, want builtin_openai_official", listed[0].UsageDriver)
	}
}

func TestAccountsHandlerCreateThirdPartyDefaultsResponsesSupport(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	handler := api.NewAccountsHandler(repo, nil, auth.NewOAuthConnector(auth.Config{}), auth.NewStateStore(5*time.Minute))

	req := httptest.NewRequest(http.MethodPost, "/accounts", bytes.NewBufferString(`{
		"provider_type":"openai-compatible",
		"account_name":"team3",
		"auth_mode":"api_key",
		"base_url":"https://code.ppchat.vip/v1",
		"credential_ref":"sk-test"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /accounts status = %d, want %d", rec.Code, http.StatusCreated)
	}

	listed, err := repo.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("List returned %d accounts, want 1", len(listed))
	}
	if !listed[0].SupportsResponses {
		t.Fatal("SupportsResponses = false, want true by default for third-party accounts")
	}
}

func TestAccountsHandlerCreateThirdPartyRespectsExplicitResponsesOptOut(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	handler := api.NewAccountsHandler(repo, nil, auth.NewOAuthConnector(auth.Config{}), auth.NewStateStore(5*time.Minute))

	req := httptest.NewRequest(http.MethodPost, "/accounts", bytes.NewBufferString(`{
		"provider_type":"openai-compatible",
		"account_name":"team3",
		"auth_mode":"api_key",
		"base_url":"https://code.ppchat.vip/v1",
		"credential_ref":"sk-test",
		"supports_responses": false
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /accounts status = %d, want %d", rec.Code, http.StatusCreated)
	}

	listed, err := repo.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("List returned %d accounts, want 1", len(listed))
	}
	if listed[0].SupportsResponses {
		t.Fatal("SupportsResponses = true, want explicit false to be preserved")
	}
}

func TestAccountsHandlerUpdateAndTestAccount(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-updated" {
			t.Fatalf("authorization = %q, want Bearer sk-updated", got)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode returned error: %v", err)
		}
		if payload["model"] != "gpt-5.2-codex" {
			t.Fatalf("model = %v, want gpt-5.2-codex", payload["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"pong"}}]}`)
	}))
	defer upstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	usageRepo := usage.NewSQLiteRepository(store.DB())
	handler := api.NewAccountsHandler(repo, usageRepo, auth.NewOAuthConnector(auth.Config{}), auth.NewStateStore(5*time.Minute))

	if err := repo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "editable",
		AuthMode:      accounts.AuthModeAPIKey,
		BaseURL:       "https://old.example.test/v1",
		CredentialRef: "sk-old",
		Status:        accounts.StatusActive,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	updateReq := httptest.NewRequest(http.MethodPut, "/accounts/1", bytes.NewBufferString(`{
		"account_name":"edited-name",
		"source_icon":"claude_code",
		"base_url":"`+upstream.URL+`/v1",
		"credential_ref":"sk-updated",
		"account_driver":"builtin_api_key",
		"usage_driver":"lua",
		"usage_config_json":"{\"script\":\"adapters/vendor.lua\",\"timeout_ms\":5000}",
		"status":"cooldown",
		"priority":7,
		"is_active":true,
		"supports_responses":true
	}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("PUT /accounts/1 status = %d, want %d", updateRec.Code, http.StatusOK)
	}

	listed, err := repo.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if listed[0].AccountName != "edited-name" {
		t.Fatalf("AccountName = %q, want edited-name", listed[0].AccountName)
	}
	if listed[0].CredentialRef != "sk-updated" {
		t.Fatalf("CredentialRef = %q, want sk-updated", listed[0].CredentialRef)
	}
	if listed[0].Priority != 7 {
		t.Fatalf("Priority = %d, want 7", listed[0].Priority)
	}
	if !listed[0].IsActive {
		t.Fatal("IsActive = false, want true")
	}
	if !listed[0].SupportsResponses {
		t.Fatal("SupportsResponses = false, want true")
	}
	if listed[0].SourceIcon != "claude_code" {
		t.Fatalf("SourceIcon = %q, want claude_code", listed[0].SourceIcon)
	}
	if listed[0].AccountDriver != "builtin_api_key" {
		t.Fatalf("AccountDriver = %q, want builtin_api_key", listed[0].AccountDriver)
	}
	if listed[0].UsageDriver != "lua" {
		t.Fatalf("UsageDriver = %q, want lua", listed[0].UsageDriver)
	}
	if listed[0].UsageConfigJSON != "{\"script\":\"adapters/vendor.lua\",\"timeout_ms\":5000}" {
		t.Fatalf("UsageConfigJSON = %q, want serialized config", listed[0].UsageConfigJSON)
	}

	testReq := httptest.NewRequest(http.MethodPost, "/accounts/1/test", bytes.NewBufferString(`{
		"model":"gpt-5.2-codex",
		"input":"ping"
	}`))
	testReq.Header.Set("Content-Type", "application/json")
	testRec := httptest.NewRecorder()
	handler.ServeHTTP(testRec, testReq)
	if testRec.Code != http.StatusOK {
		t.Fatalf("POST /accounts/1/test status = %d, want %d", testRec.Code, http.StatusOK)
	}

	var payload map[string]any
	if err := json.Unmarshal(testRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if payload["ok"] != true {
		t.Fatalf("ok = %v, want true", payload["ok"])
	}
	if payload["message"] != "远端连通性测试成功" {
		t.Fatalf("message = %v, want remote success message", payload["message"])
	}
	if payload["content"] != "pong" {
		t.Fatalf("content = %v, want pong", payload["content"])
	}
}

func TestAccountsHandlerListAccountsDefaultsBuiltInDrivers(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	handler := api.NewAccountsHandler(repo, nil, auth.NewOAuthConnector(auth.Config{}), auth.NewStateStore(5*time.Minute))

	if err := repo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAIOfficial,
		AccountName:   "official",
		AuthMode:      accounts.AuthModeLocalImport,
		CredentialRef: "raw-auth",
		BaseURL:       "https://chatgpt.com/backend-api/codex",
		Status:        accounts.StatusActive,
	}); err != nil {
		t.Fatalf("Create official returned error: %v", err)
	}
	if err := repo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "ppchat",
		AuthMode:      accounts.AuthModeAPIKey,
		CredentialRef: "sk-test",
		BaseURL:       "https://code.ppchat.vip/v1",
		Status:        accounts.StatusActive,
	}); err != nil {
		t.Fatalf("Create ppchat returned error: %v", err)
	}
	if err := repo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "nodeseek",
		AuthMode:      accounts.AuthModeAPIKey,
		CredentialRef: "sk-test",
		BaseURL:       "https://ai.nodeseek.in",
		Status:        accounts.StatusActive,
	}); err != nil {
		t.Fatalf("Create nodeseek returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/accounts", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /accounts status = %d, want %d", rec.Code, http.StatusOK)
	}

	var listed []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if listed[0]["account_driver"] != "builtin_openai_official_session" {
		t.Fatalf("official account_driver = %v, want builtin_openai_official_session", listed[0]["account_driver"])
	}
	if listed[0]["usage_driver"] != "builtin_openai_official" {
		t.Fatalf("official usage_driver = %v, want builtin_openai_official", listed[0]["usage_driver"])
	}
	if listed[1]["account_driver"] != "builtin_api_key" {
		t.Fatalf("ppchat account_driver = %v, want builtin_api_key", listed[1]["account_driver"])
	}
	if listed[1]["usage_driver"] != "builtin_ppchat" {
		t.Fatalf("ppchat usage_driver = %v, want builtin_ppchat", listed[1]["usage_driver"])
	}
	if listed[2]["account_driver"] != "builtin_api_key" {
		t.Fatalf("nodeseek account_driver = %v, want builtin_api_key", listed[2]["account_driver"])
	}
	if listed[2]["usage_driver"] != "lua" {
		t.Fatalf("nodeseek usage_driver = %v, want lua", listed[2]["usage_driver"])
	}
	if listed[2]["usage_config_json"] != `{"script":"managed:ai.nodeseek.in"}` {
		t.Fatalf("nodeseek usage_config_json = %v, want managed script config", listed[2]["usage_config_json"])
	}
}

func TestAccountsHandlerDeleteAccount(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	usageRepo := usage.NewSQLiteRepository(store.DB())
	handler := api.NewAccountsHandler(repo, usageRepo, auth.NewOAuthConnector(auth.Config{}), auth.NewStateStore(5*time.Minute))

	if err := repo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "delete-me",
		AuthMode:      accounts.AuthModeAPIKey,
		BaseURL:       "https://delete.example.test/v1",
		CredentialRef: "sk-delete",
		Status:        accounts.StatusActive,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/accounts/1", nil)
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /accounts/1 status = %d, want %d", deleteRec.Code, http.StatusNoContent)
	}

	listed, err := repo.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("List returned %d accounts, want 0", len(listed))
	}
}

func TestAccountsHandlerDuplicateAccountCreatesInactiveCopyWithIncrementedName(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	handler := api.NewAccountsHandler(repo, nil, auth.NewOAuthConnector(auth.Config{}), auth.NewStateStore(5*time.Minute))

	if err := repo.Create(accounts.Account{
		ProviderType:      accounts.ProviderOpenAICompatible,
		AccountName:       "mirror-east",
		SourceIcon:        "ppchat",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           "https://code.ppchat.vip/v1",
		CredentialRef:     "sk-origin",
		Status:            accounts.StatusActive,
		Priority:          3,
		IsActive:          true,
		SupportsResponses: true,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if err := repo.Create(accounts.Account{
		ProviderType:      accounts.ProviderOpenAICompatible,
		AccountName:       "mirror-east 1",
		SourceIcon:        "ppchat",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           "https://code.ppchat.vip/v1",
		CredentialRef:     "sk-existing",
		Status:            accounts.StatusActive,
		Priority:          2,
		IsActive:          false,
		SupportsResponses: true,
	}); err != nil {
		t.Fatalf("Create second account returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/accounts/1/duplicate", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /accounts/1/duplicate status = %d, want %d", rec.Code, http.StatusCreated)
	}

	listed, err := repo.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(listed) != 3 {
		t.Fatalf("List returned %d accounts, want 3", len(listed))
	}

	var duplicate accounts.Account
	found := false
	for _, item := range listed {
		if item.AccountName == "mirror-east 2" {
			duplicate = item
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("List did not contain duplicate account: %+v", listed)
	}
	if duplicate.AccountName != "mirror-east 2" {
		t.Fatalf("duplicate name = %q, want %q", duplicate.AccountName, "mirror-east 2")
	}
	if duplicate.CredentialRef != "sk-origin" {
		t.Fatalf("duplicate credential = %q, want %q", duplicate.CredentialRef, "sk-origin")
	}
	if duplicate.IsActive {
		t.Fatal("duplicate IsActive = true, want false")
	}
	if duplicate.ProviderType != accounts.ProviderOpenAICompatible {
		t.Fatalf("duplicate provider_type = %q, want %q", duplicate.ProviderType, accounts.ProviderOpenAICompatible)
	}
	if duplicate.SourceIcon != "ppchat" {
		t.Fatalf("duplicate source_icon = %q, want ppchat", duplicate.SourceIcon)
	}
}

func TestAccountsHandlerShareExportsPortablePayload(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	handler := api.NewAccountsHandler(repo, nil, auth.NewOAuthConnector(auth.Config{}), auth.NewStateStore(5*time.Minute))

	if err := repo.Create(accounts.Account{
		ProviderType:      accounts.ProviderOpenAICompatible,
		AccountName:       "mirror-east",
		SourceIcon:        "ppchat",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           "https://code.ppchat.vip/v1",
		CredentialRef:     "sk-share-me",
		AccountDriver:     "builtin_api_key",
		UsageDriver:       "lua",
		UsageConfigJSON:   `{"script":"adapters/vendor.lua"}`,
		Status:            accounts.StatusActive,
		CooldownReason:    "capacity_failed",
		Priority:          9,
		IsActive:          true,
		SupportsResponses: true,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/accounts/1/share", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /accounts/1/share status = %d, want %d", rec.Code, http.StatusOK)
	}

	var response struct {
		Payload string `json:"payload"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if strings.TrimSpace(response.Payload) == "" {
		t.Fatal("payload is empty")
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(response.Payload), &payload); err != nil {
		t.Fatalf("Unmarshal payload returned error: %v", err)
	}
	if payload["kind"] != "aigate-account-share" {
		t.Fatalf("kind = %v, want aigate-account-share", payload["kind"])
	}
	if payload["schema_version"].(float64) != 1 {
		t.Fatalf("schema_version = %v, want 1", payload["schema_version"])
	}
	if strings.TrimSpace(payload["exported_at"].(string)) == "" {
		t.Fatal("exported_at is empty")
	}

	accountPayload, ok := payload["account"].(map[string]any)
	if !ok {
		t.Fatalf("account = %#v, want object", payload["account"])
	}
	if accountPayload["provider_type"] != "openai-compatible" {
		t.Fatalf("provider_type = %v, want openai-compatible", accountPayload["provider_type"])
	}
	if accountPayload["account_name"] != "mirror-east" {
		t.Fatalf("account_name = %v, want mirror-east", accountPayload["account_name"])
	}
	if accountPayload["credential_ref"] != "sk-share-me" {
		t.Fatalf("credential_ref = %v, want sk-share-me", accountPayload["credential_ref"])
	}
	if accountPayload["usage_driver"] != "lua" {
		t.Fatalf("usage_driver = %v, want lua", accountPayload["usage_driver"])
	}
	if accountPayload["usage_config_json"] != `{"script":"adapters/vendor.lua"}` {
		t.Fatalf("usage_config_json = %v, want serialized config", accountPayload["usage_config_json"])
	}
	if _, exists := accountPayload["priority"]; exists {
		t.Fatalf("portable payload unexpectedly contains priority: %#v", accountPayload)
	}
	if _, exists := accountPayload["is_active"]; exists {
		t.Fatalf("portable payload unexpectedly contains is_active: %#v", accountPayload)
	}
	if _, exists := accountPayload["status"]; exists {
		t.Fatalf("portable payload unexpectedly contains status: %#v", accountPayload)
	}
}

func TestAccountsHandlerImportSharedCreatesFreshAccount(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	handler := api.NewAccountsHandler(repo, nil, auth.NewOAuthConnector(auth.Config{}), auth.NewStateStore(5*time.Minute))

	req := httptest.NewRequest(http.MethodPost, "/accounts/import-shared", bytes.NewBufferString(`{
		"payload":"{\"kind\":\"aigate-account-share\",\"schema_version\":1,\"exported_at\":\"2026-03-18T15:20:00Z\",\"account\":{\"provider_type\":\"openai-compatible\",\"account_name\":\"shared-mirror\",\"source_icon\":\"ppchat\",\"auth_mode\":\"api_key\",\"base_url\":\"https://code.ppchat.vip/v1\",\"credential_ref\":\"sk-imported\",\"account_driver\":\"builtin_api_key\",\"usage_driver\":\"lua\",\"usage_config_json\":\"{\\\"script\\\":\\\"adapters/vendor.lua\\\"}\",\"supports_responses\":true}}"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /accounts/import-shared status = %d, want %d", rec.Code, http.StatusCreated)
	}

	listed, err := repo.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("List returned %d accounts, want 1", len(listed))
	}
	if listed[0].AccountName != "shared-mirror" {
		t.Fatalf("AccountName = %q, want shared-mirror", listed[0].AccountName)
	}
	if listed[0].CredentialRef != "sk-imported" {
		t.Fatalf("CredentialRef = %q, want sk-imported", listed[0].CredentialRef)
	}
	if listed[0].Priority != 0 {
		t.Fatalf("Priority = %d, want 0", listed[0].Priority)
	}
	if listed[0].IsActive {
		t.Fatal("IsActive = true, want false")
	}
	if listed[0].Status != accounts.StatusActive {
		t.Fatalf("Status = %q, want active", listed[0].Status)
	}
	if listed[0].UsageDriver != "lua" {
		t.Fatalf("UsageDriver = %q, want lua", listed[0].UsageDriver)
	}
}

func TestAccountsHandlerImportSharedRejectsInvalidPayload(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		payload string
	}{
		{
			name:    "invalid kind",
			payload: `{"kind":"wrong-kind","schema_version":1,"exported_at":"2026-03-18T15:20:00Z","account":{"provider_type":"openai-compatible","account_name":"shared-mirror","source_icon":"ppchat","auth_mode":"api_key","base_url":"https://code.ppchat.vip/v1","credential_ref":"sk-imported","usage_config_json":"{}"}}`,
		},
		{
			name:    "invalid schema version",
			payload: `{"kind":"aigate-account-share","schema_version":2,"exported_at":"2026-03-18T15:20:00Z","account":{"provider_type":"openai-compatible","account_name":"shared-mirror","source_icon":"ppchat","auth_mode":"api_key","base_url":"https://code.ppchat.vip/v1","credential_ref":"sk-imported","usage_config_json":"{}"}}`,
		},
		{
			name:    "invalid base url",
			payload: `{"kind":"aigate-account-share","schema_version":1,"exported_at":"2026-03-18T15:20:00Z","account":{"provider_type":"openai-compatible","account_name":"shared-mirror","source_icon":"ppchat","auth_mode":"api_key","base_url":"not-a-url","credential_ref":"sk-imported","usage_config_json":"{}"}}`,
		},
		{
			name:    "invalid usage config json",
			payload: `{"kind":"aigate-account-share","schema_version":1,"exported_at":"2026-03-18T15:20:00Z","account":{"provider_type":"openai-compatible","account_name":"shared-mirror","source_icon":"ppchat","auth_mode":"api_key","base_url":"https://code.ppchat.vip/v1","credential_ref":"sk-imported","usage_config_json":"{"}}`,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
			if err != nil {
				t.Fatalf("Open returned error: %v", err)
			}
			t.Cleanup(func() { _ = store.Close() })

			repo := accounts.NewSQLiteRepository(store.DB())
			handler := api.NewAccountsHandler(repo, nil, auth.NewOAuthConnector(auth.Config{}), auth.NewStateStore(5*time.Minute))

			req := httptest.NewRequest(http.MethodPost, "/accounts/import-shared", bytes.NewBufferString(`{"payload":`+strconv.Quote(tc.payload)+`}`))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("POST /accounts/import-shared status = %d, want %d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}

			listed, err := repo.List()
			if err != nil {
				t.Fatalf("List returned error: %v", err)
			}
			if len(listed) != 0 {
				t.Fatalf("List returned %d accounts, want 0", len(listed))
			}
		})
	}
}

func TestAccountsHandlerListUsageReadsCachedSnapshotsOnly(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected outbound usage refresh: %s", r.URL.Path)
	}))
	defer upstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	usageRepo := usage.NewSQLiteRepository(store.DB())
	handler := api.NewAccountsHandler(repo, usageRepo, auth.NewOAuthConnector(auth.Config{}), auth.NewStateStore(5*time.Minute))

	if err := repo.Create(accounts.Account{
		ProviderType: accounts.ProviderOpenAIOfficial,
		AccountName:  "local-codex",
		AuthMode:     accounts.AuthModeLocalImport,
		BaseURL:      upstream.URL + "/backend-api/codex",
		CredentialRef: `{
			"auth_mode":"chatgpt",
			"tokens":{"access_token":"token-1","account_id":"acct-1"}
		}`,
		Status: accounts.StatusActive,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if err := usageRepo.Save(usage.Snapshot{
		AccountID:            1,
		Balance:              5.39,
		PrimaryUsedPercent:   34,
		SecondaryUsedPercent: 58,
		RPMRemaining:         66,
		TPMRemaining:         42,
		CheckedAt:            time.Now().UTC(),
		Source:               "remote",
		Confidence:           "high",
		ProviderSnapshotJSON: `{"cached":true}`,
	}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/accounts/usage", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("GET /accounts/usage status = %d, want %d", listRec.Code, http.StatusOK)
	}

	var listed []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if listed[0]["balance"].(float64) != 5.39 {
		t.Fatalf("balance = %v, want 5.39", listed[0]["balance"])
	}
	if listed[0]["primary_used_percent"].(float64) != 34 {
		t.Fatalf("primary_used_percent = %v, want 34", listed[0]["primary_used_percent"])
	}
	if listed[0]["secondary_used_percent"].(float64) != 58 {
		t.Fatalf("secondary_used_percent = %v, want 58", listed[0]["secondary_used_percent"])
	}
	if listed[0]["rpm_remaining"].(float64) != 66 {
		t.Fatalf("rpm_remaining = %v, want 66", listed[0]["rpm_remaining"])
	}
	if listed[0]["tpm_remaining"].(float64) != 42 {
		t.Fatalf("tpm_remaining = %v, want 42", listed[0]["tpm_remaining"])
	}
}

func TestAccountsHandlerListUsageIncludesPPChatDailySummaryFromCachedSnapshot(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	usageRepo := usage.NewSQLiteRepository(store.DB())
	handler := api.NewAccountsHandler(repo, usageRepo, auth.NewOAuthConnector(auth.Config{}), auth.NewStateStore(5*time.Minute))

	if err := repo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "ppchat-main",
		SourceIcon:    "ppchat",
		AuthMode:      accounts.AuthModeAPIKey,
		BaseURL:       "https://code.ppchat.vip/v1",
		CredentialRef: "sk-ppchat",
		Status:        accounts.StatusActive,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if err := usageRepo.Save(usage.Snapshot{
		AccountID:      1,
		QuotaRemaining: 13931,
		CheckedAt:      time.Now().UTC(),
		Source:         "remote",
		Confidence:     "high",
		ProviderSnapshotJSON: `{
			"capacity_model":"quota_rate",
			"payload":{
				"success":true,
				"data":{
					"token_info":{
						"today_used_quota":1068,
						"remain_quota_display":13931,
						"today_added_quota":14999
					}
				}
			}
		}`,
	}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/accounts/usage", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /accounts/usage status = %d, want %d", rec.Code, http.StatusOK)
	}

	var listed []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if listed[0]["ppchat_today_used_quota"].(float64) != 1068 {
		t.Fatalf("ppchat_today_used_quota = %v, want 1068", listed[0]["ppchat_today_used_quota"])
	}
	if listed[0]["ppchat_today_remaining_quota"].(float64) != 13931 {
		t.Fatalf("ppchat_today_remaining_quota = %v, want 13931", listed[0]["ppchat_today_remaining_quota"])
	}
	if listed[0]["ppchat_today_added_quota"].(float64) != 14999 {
		t.Fatalf("ppchat_today_added_quota = %v, want 14999", listed[0]["ppchat_today_added_quota"])
	}
}

func TestAccountsHandlerListUsageIncludesCustomDisplayHints(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	usageRepo := usage.NewSQLiteRepository(store.DB())
	handler := api.NewAccountsHandler(repo, usageRepo, auth.NewOAuthConnector(auth.Config{}), auth.NewStateStore(5*time.Minute))

	if err := repo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "nodeseek",
		AuthMode:      accounts.AuthModeAPIKey,
		BaseURL:       "https://ai.nodeseek.in",
		CredentialRef: "sk-nodeseek",
		Status:        accounts.StatusActive,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if err := usageRepo.Save(usage.Snapshot{
		AccountID:   1,
		Balance:     61.96,
		HealthScore: 1,
		CheckedAt:   time.Now().UTC(),
		Source:      "remote",
		Confidence:  "high",
		ProviderSnapshotJSON: `{
			"capacity_model":"balance_only",
			"display":{
				"summary":{"label":"余额","value":"$61.96"},
				"detail_stats":[{"label":"余额","value":"$61.96"}],
				"detail_items":[{"label":"计费单位","value":"美元"}]
			}
		}`,
	}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/accounts/usage", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /accounts/usage status = %d, want %d", rec.Code, http.StatusOK)
	}

	var listed []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	display, ok := listed[0]["usage_display"].(map[string]any)
	if !ok {
		t.Fatalf("usage_display = %#v, want object", listed[0]["usage_display"])
	}
	summary := display["summary"].(map[string]any)
	if summary["label"] != "余额" || summary["value"] != "$61.96" {
		t.Fatalf("usage_display.summary = %#v, want balance label/value", summary)
	}
}

func TestAccountsHandlerListUsageDerivesPPChatAddedQuotaWhenMissing(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	usageRepo := usage.NewSQLiteRepository(store.DB())
	handler := api.NewAccountsHandler(repo, usageRepo, auth.NewOAuthConnector(auth.Config{}), auth.NewStateStore(5*time.Minute))

	if err := repo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "ppchat-main",
		SourceIcon:    "ppchat",
		AuthMode:      accounts.AuthModeAPIKey,
		BaseURL:       "https://code.ppchat.vip/v1",
		CredentialRef: "sk-ppchat",
		Status:        accounts.StatusActive,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if err := usageRepo.Save(usage.Snapshot{
		AccountID:      1,
		QuotaRemaining: 13931,
		CheckedAt:      time.Now().UTC(),
		Source:         "remote",
		Confidence:     "high",
		ProviderSnapshotJSON: `{
			"capacity_model":"quota_rate",
			"payload":{
				"success":true,
				"data":{
					"token_info":{
						"today_used_quota":1068,
						"remain_quota_display":13931
					}
				}
			}
		}`,
	}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/accounts/usage", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /accounts/usage status = %d, want %d", rec.Code, http.StatusOK)
	}

	var listed []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if listed[0]["ppchat_today_added_quota"].(float64) != 14999 {
		t.Fatalf("ppchat_today_added_quota = %v, want 14999", listed[0]["ppchat_today_added_quota"])
	}
}

func TestAccountsHandlerRefreshUsage(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/wham/usage" {
			t.Fatalf("path = %q, want /backend-api/wham/usage", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"rate_limit":{
				"allowed":true,
				"limit_reached":false,
				"primary_window":{"used_percent":0,"reset_at":0},
				"secondary_window":{"used_percent":0,"reset_at":0}
			},
			"credits":{"has_credits":true,"unlimited":false,"balance":"0"}
		}`)
	}))
	defer upstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	usageRepo := usage.NewSQLiteRepository(store.DB())
	client := upstream.Client()
	driverRegistry, err := registry.New(
		[]accountdrv.AccountDriver{
			accountdrv.NewOfficialDriver(client, repo),
			accountdrv.NewAPIKeyDriver(),
		},
		[]usagedrv.UsageDriver{
			builtin.NewOpenAIOfficialDriver(client),
		},
	)
	if err != nil {
		t.Fatalf("registry.New returned error: %v", err)
	}
	refresher := refresh.NewOrchestrator(repo, usageRepo, driverRegistry)
	handler := api.NewAccountsHandler(
		repo,
		usageRepo,
		auth.NewOAuthConnector(auth.Config{}),
		auth.NewStateStore(5*time.Minute),
		api.WithAccountsUsageRefresher(refresher),
	)

	if err := repo.Create(accounts.Account{
		ProviderType: accounts.ProviderOpenAIOfficial,
		AccountName:  "local-codex",
		AuthMode:     accounts.AuthModeLocalImport,
		BaseURL:      upstream.URL + "/backend-api/codex",
		CredentialRef: `{
			"auth_mode":"chatgpt",
			"tokens":{"access_token":"token-1","account_id":"acct-1"}
		}`,
		Status: accounts.StatusActive,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	refreshReq := httptest.NewRequest(http.MethodPost, "/accounts/usage/refresh", nil)
	refreshReq = refreshReq.WithContext(context.Background())
	refreshRec := httptest.NewRecorder()
	handler.ServeHTTP(refreshRec, refreshReq)
	if refreshRec.Code != http.StatusNoContent {
		t.Fatalf("POST /accounts/usage/refresh status = %d, want %d", refreshRec.Code, http.StatusNoContent)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/accounts/usage", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("GET /accounts/usage status = %d, want %d", listRec.Code, http.StatusOK)
	}

	var listed []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if listed[0]["balance"].(float64) != 0 {
		t.Fatalf("balance = %v, want 0", listed[0]["balance"])
	}
	if listed[0]["rpm_remaining"].(float64) != 100 {
		t.Fatalf("rpm_remaining = %v, want 100", listed[0]["rpm_remaining"])
	}
	if listed[0]["tpm_remaining"].(float64) != 100 {
		t.Fatalf("tpm_remaining = %v, want 100", listed[0]["tpm_remaining"])
	}
}

func TestAccountsHandlerTestLocalImportedAccount(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %q, want /v1/responses", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token-1" {
			t.Fatalf("authorization = %q, want Bearer token-1", got)
		}
		if got := r.Header.Get("ChatGPT-Account-Id"); got != "acct-1" {
			t.Fatalf("ChatGPT-Account-Id = %q, want acct-1", got)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode returned error: %v", err)
		}
		if stream, ok := payload["stream"].(bool); !ok || !stream {
			t.Fatalf("stream = %#v, want true", payload["stream"])
		}
		instructions, _ := payload["instructions"].(string)
		if strings.TrimSpace(instructions) == "" {
			t.Fatal("instructions is empty, want default codex instructions")
		}
		inputItems, ok := payload["input"].([]any)
		if !ok || len(inputItems) != 1 {
			t.Fatalf("input = %#v, want single list item", payload["input"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"official-pong\"}\n\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":54,\"input_tokens_details\":{\"cached_tokens\":0},\"output_tokens\":5,\"total_tokens\":59},\"store\":false}}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer upstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	usageRepo := usage.NewSQLiteRepository(store.DB())
	handler := api.NewAccountsHandler(repo, usageRepo, auth.NewOAuthConnector(auth.Config{}), auth.NewStateStore(5*time.Minute))

	if err := repo.Create(accounts.Account{
		ProviderType: accounts.ProviderOpenAIOfficial,
		AccountName:  "local-codex",
		AuthMode:     accounts.AuthModeLocalImport,
		BaseURL:      upstream.URL + "/v1",
		CredentialRef: `{
			"auth_mode":"chatgpt",
			"tokens":{"access_token":"token-1","account_id":"acct-1"}
		}`,
		Status: accounts.StatusActive,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	testReq := httptest.NewRequest(http.MethodPost, "/accounts/1/test", bytes.NewBufferString(`{
		"model":"gpt-5.2-codex",
		"input":"ping"
	}`))
	testReq.Header.Set("Content-Type", "application/json")
	testRec := httptest.NewRecorder()
	handler.ServeHTTP(testRec, testReq)
	if testRec.Code != http.StatusOK {
		t.Fatalf("POST /accounts/1/test status = %d, want %d", testRec.Code, http.StatusOK)
	}

	var payload map[string]any
	if err := json.Unmarshal(testRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if payload["ok"] != true {
		t.Fatalf("ok = %v, want true", payload["ok"])
	}
	if payload["message"] != "OpenAI responses 测试成功" {
		t.Fatalf("message = %v, want responses success message", payload["message"])
	}
	if payload["content"] != "official-pong" {
		t.Fatalf("content = %v, want official-pong", payload["content"])
	}

	snapshot, err := usageRepo.GetLatest(1)
	if err != nil {
		t.Fatalf("GetLatest returned error: %v", err)
	}
	if snapshot.LastTotalTokens != 59 {
		t.Fatalf("LastTotalTokens = %v, want 59", snapshot.LastTotalTokens)
	}
	if snapshot.LastInputTokens != 54 {
		t.Fatalf("LastInputTokens = %v, want 54", snapshot.LastInputTokens)
	}
	if snapshot.LastOutputTokens != 5 {
		t.Fatalf("LastOutputTokens = %v, want 5", snapshot.LastOutputTokens)
	}
	if snapshot.HealthScore != 1 {
		t.Fatalf("HealthScore = %v, want 1", snapshot.HealthScore)
	}
}

func TestAccountsHandlerTestAccountReturnsUpstreamErrorDetails(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":{"message":"model not allowed"}}`)
	}))
	defer upstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	handler := api.NewAccountsHandler(repo, nil, auth.NewOAuthConnector(auth.Config{}), auth.NewStateStore(5*time.Minute))

	if err := repo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "ppchat",
		AuthMode:      accounts.AuthModeAPIKey,
		BaseURL:       upstream.URL + "/v1",
		CredentialRef: "sk-test",
		Status:        accounts.StatusActive,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	testReq := httptest.NewRequest(http.MethodPost, "/accounts/1/test", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"ping"
	}`))
	testReq.Header.Set("Content-Type", "application/json")
	testRec := httptest.NewRecorder()
	handler.ServeHTTP(testRec, testReq)
	if testRec.Code != http.StatusOK {
		t.Fatalf("POST /accounts/1/test status = %d, want %d", testRec.Code, http.StatusOK)
	}

	var payload map[string]any
	if err := json.Unmarshal(testRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if payload["ok"] != false {
		t.Fatalf("ok = %v, want false", payload["ok"])
	}
	if !strings.Contains(payload["details"].(string), "403 Forbidden") {
		t.Fatalf("details = %q, want 403 status", payload["details"])
	}
	if !strings.Contains(payload["content"].(string), "model not allowed") {
		t.Fatalf("content = %q, want upstream body", payload["content"])
	}
}

func TestAccountsHandlerImportLocalCodexAuthUpload(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	handler := api.NewAccountsHandler(repo, nil, auth.NewOAuthConnector(auth.Config{}), auth.NewStateStore(5*time.Minute))

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("account_name", "uploaded-codex"); err != nil {
		t.Fatalf("WriteField returned error: %v", err)
	}
	part, err := writer.CreateFormFile("auth_file", "auth.json")
	if err != nil {
		t.Fatalf("CreateFormFile returned error: %v", err)
	}
	if _, err := io.WriteString(part, `{"auth_mode":"chatgpt","tokens":{"access_token":"token-upload","account_id":"acct-upload"}}`); err != nil {
		t.Fatalf("WriteString returned error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/accounts/import-local", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /accounts/import-local upload status = %d, want %d", rec.Code, http.StatusCreated)
	}

	listed, err := repo.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("List returned %d accounts, want 1", len(listed))
	}
	if listed[0].AccountName != "uploaded-codex" {
		t.Fatalf("AccountName = %q, want uploaded-codex", listed[0].AccountName)
	}
	if listed[0].BaseURL != "https://chatgpt.com/backend-api/codex" {
		t.Fatalf("BaseURL = %q, want https://chatgpt.com/backend-api/codex", listed[0].BaseURL)
	}
}

func TestAccountsHandlerImportCurrentCodexAuth(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "auth.json"), []byte(`{
		"auth_mode":"chatgpt",
		"tokens":{"access_token":"token-1","account_id":"acct-1"}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	handler := api.NewAccountsHandler(repo, nil, auth.NewOAuthConnector(auth.Config{}), auth.NewStateStore(5*time.Minute))

	req := httptest.NewRequest(http.MethodPost, "/accounts/import-current", bytes.NewBufferString(`{"account_name":"current-codex"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /accounts/import-current status = %d, want %d", rec.Code, http.StatusCreated)
	}

	listed, err := repo.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("List returned %d accounts, want 1", len(listed))
	}
	if listed[0].AccountName != "current-codex" {
		t.Fatalf("AccountName = %q, want current-codex", listed[0].AccountName)
	}
}

func TestAccountsHandlerImportCurrentCodexAuthWaitsForFileReady(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	authPath := filepath.Join(codexDir, "auth.json")

	go func() {
		time.Sleep(80 * time.Millisecond)
		_ = os.WriteFile(authPath, []byte(`{
			"auth_mode":"chatgpt",
			"tokens":{"access_token":"token-1","account_id":"acct-1"}
		}`), 0o600)
	}()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	handler := api.NewAccountsHandler(repo, nil, auth.NewOAuthConnector(auth.Config{}), auth.NewStateStore(5*time.Minute))

	req := httptest.NewRequest(http.MethodPost, "/accounts/import-current", bytes.NewBufferString(`{"account_name":"current-codex"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /accounts/import-current status = %d, want %d body=%q", rec.Code, http.StatusCreated, rec.Body.String())
	}

	listed, err := repo.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("List returned %d accounts, want 1", len(listed))
	}
}
