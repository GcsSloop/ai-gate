package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/api"
	"github.com/gcssloop/codex-router/backend/internal/auth"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

func TestAccountsHandler(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := accounts.NewSQLiteRepository(store.DB())
	usageRepo := usage.NewSQLiteRepository(store.DB())
	connector := auth.NewOAuthConnector(auth.Config{
		ClientID:     "client-id",
		AuthorizeURL: "https://auth.example.test/oauth/authorize",
		TokenURL:     "https://auth.example.test/oauth/token",
		RedirectURL:  "http://localhost:8080/callback",
		Scopes:       []string{"model.read"},
	})
	handler := api.NewAccountsHandler(repo, usageRepo, connector, auth.NewStateStore(5*time.Minute))

	createBody := bytes.NewBufferString(`{
		"provider_type":"openai-compatible",
		"account_name":"mirror-east",
		"auth_mode":"api_key",
		"base_url":"https://mirror.example.test/v1",
		"credential_ref":"cred-api-key"
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
		ProviderType:  accounts.ProviderOpenAIOfficial,
		AccountName:   "official-secondary",
		AuthMode:      accounts.AuthModeOAuth,
		CredentialRef: "cred-oauth",
		Status:        accounts.StatusCooldown,
		CooldownUntil: &cooldownUntil,
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
	if listed[1]["cooldown_remaining_seconds"] == nil {
		t.Fatal("cooldown_remaining_seconds missing from cooldown account")
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
		"base_url":"`+upstream.URL+`/v1",
		"credential_ref":"sk-updated",
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

func TestAccountsHandlerListAccountsFetchesOfficialWhamUsage(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/wham/usage" {
			t.Fatalf("path = %q, want /backend-api/wham/usage", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token-1" {
			t.Fatalf("authorization = %q, want Bearer token-1", got)
		}
		if got := r.Header.Get("ChatGPT-Account-Id"); got != "acct-1" {
			t.Fatalf("ChatGPT-Account-Id = %q, want acct-1", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"plan_type":"plus",
			"rate_limit":{
				"allowed":true,
				"limit_reached":false,
				"primary_window":{"used_percent":34,"limit_window_seconds":18000,"reset_after_seconds":1200,"reset_at":1772895924},
				"secondary_window":{"used_percent":58,"limit_window_seconds":604800,"reset_after_seconds":86400,"reset_at":1773332429}
			},
			"credits":{"has_credits":true,"unlimited":false,"balance":"5.39"}
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
