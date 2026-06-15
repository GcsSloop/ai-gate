package api_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/api"
	"github.com/gcssloop/codex-router/backend/internal/conversations"
	"github.com/gcssloop/codex-router/backend/internal/netproxy"
	"github.com/gcssloop/codex-router/backend/internal/serverusers"
	"github.com/gcssloop/codex-router/backend/internal/settings"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

func countGatewayRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()

	var count int
	query := "SELECT COUNT(*) FROM " + table
	if err := db.QueryRow(query).Scan(&count); err != nil {
		t.Fatalf("count %s returned error: %v", table, err)
	}
	return count
}

func TestGatewayHandlerProxiesToConfiguredAccount(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("upstream path = %q, want %q", r.URL.Path, "/v1/chat/completions")
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Fatalf("Authorization = %q, want %q", r.Header.Get("Authorization"), "Bearer sk-test")
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode returned error: %v", err)
		}
		if payload["model"] != "gpt-5.2-codex" {
			t.Fatalf("model = %v, want %v", payload["model"], "gpt-5.2-codex")
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "chat.completion",
			"model":  "gpt-5.2-codex",
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "ok"}},
			},
		})
	}))
	defer upstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	if err := accountRepo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "ppchat-main",
		AuthMode:      accounts.AuthModeAPIKey,
		BaseURL:       upstream.URL + "/v1",
		CredentialRef: "sk-test",
		Status:        accounts.StatusActive,
		Priority:      100,
	}); err != nil {
		t.Fatalf("Create(account) returned error: %v", err)
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	if err := usageRepo.Save(usage.Snapshot{
		AccountID:      1,
		Balance:        100,
		QuotaRemaining: 100000,
		RPMRemaining:   100,
		TPMRemaining:   100000,
		HealthScore:    0.9,
	}); err != nil {
		t.Fatalf("Save(snapshot) returned error: %v", err)
	}

	conversationRepo := conversations.NewSQLiteRepository(store.DB())
	handler := api.NewGatewayHandler(accountRepo, usageRepo, conversationRepo)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{
		"model":"gpt-5.2-codex",
		"stream":false,
		"messages":[{"role":"user","content":"ping"}]
	}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("gateway status = %d, want %d", rec.Code, http.StatusOK)
	}

	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if response["model"] != "gpt-5.2-codex" {
		t.Fatalf("response model = %v, want %v", response["model"], "gpt-5.2-codex")
	}
}

func TestGatewayHandlerUsesAccountTLSVerificationBypass(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("upstream path = %q, want %q", r.URL.Path, "/v1/chat/completions")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "chat.completion",
			"model":  "gpt-5.2-codex",
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "ok"}},
			},
		})
	}))
	defer upstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	if err := accountRepo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "self-signed",
		AuthMode:      accounts.AuthModeAPIKey,
		BaseURL:       upstream.URL + "/v1",
		CredentialRef: "sk-test",
		Status:        accounts.StatusActive,
		Priority:      100,
		SkipTLSVerify: true,
	}); err != nil {
		t.Fatalf("Create(account) returned error: %v", err)
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	if err := usageRepo.Save(usage.Snapshot{
		AccountID:      1,
		QuotaRemaining: 100000,
		RPMRemaining:   100,
		TPMRemaining:   100000,
		HealthScore:    0.9,
	}); err != nil {
		t.Fatalf("Save(snapshot) returned error: %v", err)
	}

	conversationRepo := conversations.NewSQLiteRepository(store.DB())
	handler := api.NewGatewayHandler(
		accountRepo,
		usageRepo,
		conversationRepo,
		api.WithGatewayHTTPClient(netproxy.NewHTTPClient(nil)),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{
		"model":"gpt-5.2-codex",
		"stream":false,
		"messages":[{"role":"user","content":"ping"}]
	}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("gateway status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestGatewayHandlerInServerModeUsesOnlyAssignedUserAccounts(t *testing.T) {
	t.Parallel()

	var firstHits int
	firstUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstHits++
		http.Error(w, "first should not be used", http.StatusInternalServerError)
	}))
	defer firstUpstream.Close()

	var secondHits int
	secondUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondHits++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "chat.completion",
			"model":  "gpt-test",
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "ok"}},
			},
		})
	}))
	defer secondUpstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	if err := accountRepo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "unassigned",
		AuthMode:      accounts.AuthModeAPIKey,
		BaseURL:       firstUpstream.URL + "/v1",
		CredentialRef: "sk-first",
		Status:        accounts.StatusActive,
		Priority:      100,
	}); err != nil {
		t.Fatalf("Create first account returned error: %v", err)
	}
	if err := accountRepo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "assigned",
		AuthMode:      accounts.AuthModeAPIKey,
		BaseURL:       secondUpstream.URL + "/v1",
		CredentialRef: "sk-second",
		Status:        accounts.StatusActive,
		Priority:      1,
	}); err != nil {
		t.Fatalf("Create second account returned error: %v", err)
	}
	accountList, err := accountRepo.List()
	if err != nil {
		t.Fatalf("List accounts returned error: %v", err)
	}
	var assignedID int64
	for _, account := range accountList {
		if account.AccountName == "assigned" {
			assignedID = account.ID
		}
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	for _, account := range accountList {
		if err := usageRepo.Save(usage.Snapshot{
			AccountID:      account.ID,
			Balance:        100,
			QuotaRemaining: 100000,
			RPMRemaining:   100,
			TPMRemaining:   100000,
			HealthScore:    0.9,
		}); err != nil {
			t.Fatalf("Save snapshot returned error: %v", err)
		}
	}

	userRepo := serverusers.NewSQLiteRepository(store.DB())
	created, err := userRepo.Create("alice")
	if err != nil {
		t.Fatalf("Create user returned error: %v", err)
	}
	conversationRepo := conversations.NewSQLiteRepository(store.DB())
	handler := api.WithServerGatewayAuth(userRepo, api.NewGatewayHandler(
		accountRepo,
		usageRepo,
		conversationRepo,
		api.WithGatewayServerUsers(userRepo),
	))

	noPoolReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{
		"model":"gpt-test",
		"messages":[{"role":"user","content":"ping"}]
	}`))
	noPoolReq.Header.Set("Content-Type", "application/json")
	noPoolReq.Header.Set("Authorization", "Bearer "+created.Token)
	noPoolRec := httptest.NewRecorder()
	handler.ServeHTTP(noPoolRec, noPoolReq)
	if noPoolRec.Code != http.StatusForbidden {
		t.Fatalf("no pool status = %d, want %d; body=%s", noPoolRec.Code, http.StatusForbidden, noPoolRec.Body.String())
	}

	if err := userRepo.SetAccountAssignments(created.User.ID, []int64{assignedID}); err != nil {
		t.Fatalf("SetAccountAssignments returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{
		"model":"gpt-test",
		"messages":[{"role":"user","content":"ping"}]
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+created.Token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("assigned pool status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if firstHits != 0 || secondHits != 1 {
		t.Fatalf("upstream hits first=%d second=%d, want only assigned second", firstHits, secondHits)
	}
}

func TestGatewayHandlerAddsV1WhenThirdPartyBaseURLHasNoPath(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("upstream path = %q, want %q", r.URL.Path, "/v1/chat/completions")
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "chat.completion",
			"model":  "gpt-5.2-codex",
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "ok"}},
			},
		})
	}))
	defer upstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	if err := accountRepo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "nodeseek",
		AuthMode:      accounts.AuthModeAPIKey,
		BaseURL:       upstream.URL,
		CredentialRef: "sk-test",
		Status:        accounts.StatusActive,
		Priority:      100,
	}); err != nil {
		t.Fatalf("Create(account) returned error: %v", err)
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	if err := usageRepo.Save(usage.Snapshot{
		AccountID:      1,
		QuotaRemaining: 100000,
		RPMRemaining:   100,
		TPMRemaining:   100000,
		HealthScore:    1,
	}); err != nil {
		t.Fatalf("Save(snapshot) returned error: %v", err)
	}

	handler := api.NewGatewayHandler(
		accountRepo,
		usageRepo,
		conversations.NewSQLiteRepository(store.DB()),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{
		"model":"gpt-5.2-codex",
		"stream":false,
		"messages":[{"role":"user","content":"ping"}]
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("gateway status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestGatewayHandlerTransparentlyProxiesArrayMessageContent(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode returned error: %v", err)
		}

		messages, ok := payload["messages"].([]any)
		if !ok || len(messages) != 1 {
			t.Fatalf("messages = %#v, want one message", payload["messages"])
		}
		message, ok := messages[0].(map[string]any)
		if !ok {
			t.Fatalf("message = %#v, want object", messages[0])
		}
		content, ok := message["content"].([]any)
		if !ok || len(content) != 2 {
			t.Fatalf("content = %#v, want two-item array", message["content"])
		}

		first, _ := content[0].(map[string]any)
		second, _ := content[1].(map[string]any)
		if first["text"] != "ping" || second["text"] != "pong" {
			t.Fatalf("content text = %#v, want ping/pong", content)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "chat.completion",
			"model":  "gpt-5.2-codex",
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "ok"}},
			},
		})
	}))
	defer upstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	if err := accountRepo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "ppchat-main",
		AuthMode:      accounts.AuthModeAPIKey,
		BaseURL:       upstream.URL + "/v1",
		CredentialRef: "sk-test",
		Status:        accounts.StatusActive,
		Priority:      100,
	}); err != nil {
		t.Fatalf("Create(account) returned error: %v", err)
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	if err := usageRepo.Save(usage.Snapshot{
		AccountID:      1,
		Balance:        100,
		QuotaRemaining: 100000,
		RPMRemaining:   100,
		TPMRemaining:   100000,
		HealthScore:    0.9,
	}); err != nil {
		t.Fatalf("Save(snapshot) returned error: %v", err)
	}

	handler := api.NewGatewayHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{
		"model":"gpt-5.2-codex",
		"stream":false,
		"messages":[
			{
				"role":"user",
				"content":[
					{"type":"text","text":"ping"},
					{"type":"text","text":"pong"}
				]
			}
		]
	}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("gateway status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestGatewayHandlerRecordsUsageEventWithoutAuditRows(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "chat.completion",
			"model":  "gpt-5.2-codex",
			"usage": map[string]any{
				"prompt_tokens":     640,
				"completion_tokens": 128,
				"total_tokens":      768,
			},
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "ok"}},
			},
		})
	}))
	defer upstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	if err := accountRepo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "ppchat-main",
		AuthMode:      accounts.AuthModeAPIKey,
		BaseURL:       upstream.URL + "/v1",
		CredentialRef: "sk-test",
		Status:        accounts.StatusActive,
		Priority:      100,
	}); err != nil {
		t.Fatalf("Create(account) returned error: %v", err)
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	if err := usageRepo.Save(usage.Snapshot{
		AccountID:      1,
		Balance:        100,
		QuotaRemaining: 100000,
		RPMRemaining:   100,
		TPMRemaining:   100000,
		HealthScore:    0.9,
	}); err != nil {
		t.Fatalf("Save(snapshot) returned error: %v", err)
	}

	handler := api.NewGatewayHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{
		"model":"gpt-5.2-codex",
		"stream":false,
		"messages":[{"role":"user","content":"ping"}]
	}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("gateway status = %d, want %d", rec.Code, http.StatusOK)
	}
	if count := countGatewayRows(t, store.DB(), "usage_events"); count != 1 {
		t.Fatalf("usage_events row count = %d, want 1", count)
	}
	for _, table := range []string{"conversations", "messages", "runs"} {
		if count := countGatewayRows(t, store.DB(), table); count != 0 {
			t.Fatalf("%s row count = %d, want 0", table, count)
		}
	}
}

func TestGatewayHandlerUsesPriorityOrderWhenAutoFailoverEnabled(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk-priority" {
			t.Fatalf("Authorization = %q, want %q", got, "Bearer sk-priority")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "chat.completion",
			"model":  "gpt-5.2-codex",
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "ok"}},
			},
		})
	}))
	defer upstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	for _, item := range []accounts.Account{
		{
			ProviderType:  accounts.ProviderOpenAICompatible,
			AccountName:   "high-priority",
			AuthMode:      accounts.AuthModeAPIKey,
			BaseURL:       upstream.URL + "/v1",
			CredentialRef: "sk-priority",
			Status:        accounts.StatusActive,
			Priority:      100,
		},
		{
			ProviderType:  accounts.ProviderOpenAICompatible,
			AccountName:   "low-priority",
			AuthMode:      accounts.AuthModeAPIKey,
			BaseURL:       upstream.URL + "/v1",
			CredentialRef: "sk-low",
			Status:        accounts.StatusActive,
			Priority:      10,
		},
	} {
		if err := accountRepo.Create(item); err != nil {
			t.Fatalf("Create(account) returned error: %v", err)
		}
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	for _, snapshot := range []usage.Snapshot{
		{AccountID: 1, Balance: 100, QuotaRemaining: 100000, RPMRemaining: 100, TPMRemaining: 100000, HealthScore: 0.95},
		{AccountID: 2, Balance: 100, QuotaRemaining: 100000, RPMRemaining: 100, TPMRemaining: 100000, HealthScore: 0.5},
	} {
		if err := usageRepo.Save(snapshot); err != nil {
			t.Fatalf("Save(snapshot) returned error: %v", err)
		}
	}

	settingsRepo := settings.NewSQLiteRepository(store.DB())
	current := settings.DefaultAppSettings()
	current.AutoFailoverEnabled = true
	if err := settingsRepo.SaveAppSettings(current); err != nil {
		t.Fatalf("SaveAppSettings returned error: %v", err)
	}
	if err := settingsRepo.SaveFailoverQueue([]int64{2, 1}); err != nil {
		t.Fatalf("SaveFailoverQueue returned error: %v", err)
	}

	conversationRepo := conversations.NewSQLiteRepository(store.DB())
	handler := api.NewGatewayHandler(accountRepo, usageRepo, conversationRepo, api.WithGatewaySettings(settingsRepo))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{
		"model":"gpt-5.2-codex",
		"stream":false,
		"messages":[{"role":"user","content":"ping"}]
	}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("gateway status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestGatewayHandlerUsesOnlyActiveAccountWhenAutoFailoverDisabled(t *testing.T) {
	t.Parallel()

	activeCalls := 0
	activeUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		activeCalls++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer activeUpstream.Close()

	otherCalls := 0
	otherUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherCalls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "chat.completion",
			"model":  "gpt-5.2-codex",
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "should-not-switch"}},
			},
		})
	}))
	defer otherUpstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	for _, item := range []accounts.Account{
		{
			ProviderType:  accounts.ProviderOpenAICompatible,
			AccountName:   "higher-priority",
			AuthMode:      accounts.AuthModeAPIKey,
			BaseURL:       otherUpstream.URL + "/v1",
			CredentialRef: "sk-other",
			Status:        accounts.StatusActive,
			Priority:      100,
		},
		{
			ProviderType:  accounts.ProviderOpenAICompatible,
			AccountName:   "selected-active",
			AuthMode:      accounts.AuthModeAPIKey,
			BaseURL:       activeUpstream.URL + "/v1",
			CredentialRef: "sk-active",
			Status:        accounts.StatusActive,
			Priority:      10,
			IsActive:      true,
		},
	} {
		if err := accountRepo.Create(item); err != nil {
			t.Fatalf("Create(account) returned error: %v", err)
		}
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	for id := int64(1); id <= 2; id++ {
		if err := usageRepo.Save(usage.Snapshot{
			AccountID:      id,
			Balance:        100,
			QuotaRemaining: 100000,
			RPMRemaining:   100,
			TPMRemaining:   100000,
			HealthScore:    0.9,
		}); err != nil {
			t.Fatalf("Save(snapshot %d) returned error: %v", id, err)
		}
	}

	settingsRepo := settings.NewSQLiteRepository(store.DB())
	current := settings.DefaultAppSettings()
	current.AutoFailoverEnabled = false
	if err := settingsRepo.SaveAppSettings(current); err != nil {
		t.Fatalf("SaveAppSettings returned error: %v", err)
	}

	handler := api.NewGatewayHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()), api.WithGatewaySettings(settingsRepo))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{
		"model":"gpt-5.2-codex",
		"stream":false,
		"messages":[{"role":"user","content":"ping"}]
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("gateway status = %d, want %d body=%s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
	if activeCalls != 1 {
		t.Fatalf("activeCalls = %d, want 1", activeCalls)
	}
	if otherCalls != 0 {
		t.Fatalf("otherCalls = %d, want 0", otherCalls)
	}
}

func TestGatewayHandlerSyncsActiveAccountAfterFailover(t *testing.T) {
	t.Parallel()

	primaryCalls := 0
	primaryUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryCalls++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer primaryUpstream.Close()

	fallbackUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk-fallback" {
			t.Fatalf("Authorization = %q, want %q", got, "Bearer sk-fallback")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "chat.completion",
			"model":  "gpt-5.2-codex",
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "ok"}},
			},
		})
	}))
	defer fallbackUpstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	for _, item := range []accounts.Account{
		{
			ProviderType:  accounts.ProviderOpenAICompatible,
			AccountName:   "selected-primary",
			AuthMode:      accounts.AuthModeAPIKey,
			BaseURL:       primaryUpstream.URL + "/v1",
			CredentialRef: "sk-primary",
			Status:        accounts.StatusActive,
			Priority:      100,
			IsActive:      true,
		},
		{
			ProviderType:  accounts.ProviderOpenAICompatible,
			AccountName:   "fallback-next",
			AuthMode:      accounts.AuthModeAPIKey,
			BaseURL:       fallbackUpstream.URL + "/v1",
			CredentialRef: "sk-fallback",
			Status:        accounts.StatusActive,
			Priority:      90,
		},
	} {
		if err := accountRepo.Create(item); err != nil {
			t.Fatalf("Create(account) returned error: %v", err)
		}
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	for id := int64(1); id <= 2; id++ {
		if err := usageRepo.Save(usage.Snapshot{
			AccountID:      id,
			Balance:        100,
			QuotaRemaining: 100000,
			RPMRemaining:   100,
			TPMRemaining:   100000,
			HealthScore:    0.9,
		}); err != nil {
			t.Fatalf("Save(snapshot %d) returned error: %v", id, err)
		}
	}

	settingsRepo := settings.NewSQLiteRepository(store.DB())
	current := settings.DefaultAppSettings()
	current.AutoFailoverEnabled = true
	if err := settingsRepo.SaveAppSettings(current); err != nil {
		t.Fatalf("SaveAppSettings returned error: %v", err)
	}

	handler := api.NewGatewayHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()), api.WithGatewaySettings(settingsRepo))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{
		"model":"gpt-5.2-codex",
		"stream":false,
		"messages":[{"role":"user","content":"ping"}]
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("gateway status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if primaryCalls != 1 {
		t.Fatalf("primaryCalls = %d, want 1", primaryCalls)
	}
	account, err := accountRepo.GetByID(2)
	if err != nil {
		t.Fatalf("GetByID returned error: %v", err)
	}
	if !account.IsActive {
		t.Fatal("fallback account IsActive = false, want true")
	}
}

func TestGatewayHandlerStreamsRawFramesWithoutRewriting(t *testing.T) {
	t.Parallel()

	const upstreamStream = "" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"lookup\",\"arguments\":\"{}\"}}]}}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":3,\"total_tokens\":15}}\n\n" +
		"data: [DONE]\n\n"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(upstreamStream))
	}))
	defer upstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	if err := accountRepo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "ppchat-main",
		AuthMode:      accounts.AuthModeAPIKey,
		BaseURL:       upstream.URL + "/v1",
		CredentialRef: "sk-test",
		Status:        accounts.StatusActive,
		Priority:      100,
	}); err != nil {
		t.Fatalf("Create(account) returned error: %v", err)
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	if err := usageRepo.Save(usage.Snapshot{
		AccountID:      1,
		Balance:        100,
		QuotaRemaining: 100000,
		RPMRemaining:   100,
		TPMRemaining:   100000,
		HealthScore:    0.9,
	}); err != nil {
		t.Fatalf("Save(snapshot) returned error: %v", err)
	}

	handler := api.NewGatewayHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{
		"model":"gpt-5.2-codex",
		"stream":true,
		"messages":[{"role":"user","content":"ping"}]
	}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("gateway status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("content type = %q, want %q", got, "text/event-stream")
	}
	if rec.Body.String() != upstreamStream {
		t.Fatalf("stream body = %s, want exact passthrough %s", rec.Body.String(), upstreamStream)
	}

	events, err := usageRepo.ListRecentEvents(usage.EventFilter{Limit: 5})
	if err != nil {
		t.Fatalf("ListRecentEvents returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	if events[0].InputTokens != 12 || events[0].OutputTokens != 3 || events[0].TotalTokens != 15 {
		t.Fatalf("event tokens = in:%d out:%d total:%d, want in:12 out:3 total:15", events[0].InputTokens, events[0].OutputTokens, events[0].TotalTokens)
	}
}

func TestGatewayHandlerStreamsFailoverBeforeFirstByte(t *testing.T) {
	t.Parallel()

	primaryCalls := 0
	primaryUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryCalls++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer primaryUpstream.Close()

	const fallbackStream = "" +
		"data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"Hello world\"}}]}\n\n" +
		"data: [DONE]\n\n"
	fallbackUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(fallbackStream))
	}))
	defer fallbackUpstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	for _, item := range []accounts.Account{
		{
			ProviderType:  accounts.ProviderOpenAICompatible,
			AccountName:   "ppchat-primary",
			AuthMode:      accounts.AuthModeAPIKey,
			BaseURL:       primaryUpstream.URL + "/v1",
			CredentialRef: "sk-primary",
			Status:        accounts.StatusActive,
			Priority:      100,
		},
		{
			ProviderType:  accounts.ProviderOpenAICompatible,
			AccountName:   "ppchat-fallback",
			AuthMode:      accounts.AuthModeAPIKey,
			BaseURL:       fallbackUpstream.URL + "/v1",
			CredentialRef: "sk-fallback",
			Status:        accounts.StatusActive,
			Priority:      90,
		},
	} {
		if err := accountRepo.Create(item); err != nil {
			t.Fatalf("Create(account) returned error: %v", err)
		}
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	for id := int64(1); id <= 2; id++ {
		if err := usageRepo.Save(usage.Snapshot{
			AccountID:      id,
			Balance:        100,
			QuotaRemaining: 100000,
			RPMRemaining:   100,
			TPMRemaining:   100000,
			HealthScore:    0.9,
		}); err != nil {
			t.Fatalf("Save(snapshot %d) returned error: %v", id, err)
		}
	}

	handler := api.NewGatewayHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{
		"model":"gpt-5.2-codex",
		"stream":true,
		"messages":[{"role":"user","content":"ping"}]
	}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("gateway status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if primaryCalls != 1 {
		t.Fatalf("primaryCalls = %d, want 1", primaryCalls)
	}
	if rec.Body.String() != fallbackStream {
		t.Fatalf("stream body = %s, want exact failover passthrough %s", rec.Body.String(), fallbackStream)
	}
}

func TestGatewayHandlerPrefersActiveAccountOverPriority(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk-active" {
			t.Fatalf("Authorization = %q, want %q", got, "Bearer sk-active")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "chat.completion",
			"model":  "gpt-5.2-codex",
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "ok"}},
			},
		})
	}))
	defer upstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	for _, item := range []accounts.Account{
		{
			ProviderType:  accounts.ProviderOpenAICompatible,
			AccountName:   "high-priority",
			AuthMode:      accounts.AuthModeAPIKey,
			BaseURL:       upstream.URL + "/v1",
			CredentialRef: "sk-high",
			Status:        accounts.StatusActive,
			Priority:      100,
		},
		{
			ProviderType:  accounts.ProviderOpenAICompatible,
			AccountName:   "manual-active",
			AuthMode:      accounts.AuthModeAPIKey,
			BaseURL:       upstream.URL + "/v1",
			CredentialRef: "sk-active",
			Status:        accounts.StatusActive,
			Priority:      1,
			IsActive:      true,
		},
	} {
		if err := accountRepo.Create(item); err != nil {
			t.Fatalf("Create(account) returned error: %v", err)
		}
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	for id := int64(1); id <= 2; id++ {
		if err := usageRepo.Save(usage.Snapshot{
			AccountID:      id,
			Balance:        100,
			QuotaRemaining: 100000,
			RPMRemaining:   100,
			TPMRemaining:   100000,
			HealthScore:    0.9,
		}); err != nil {
			t.Fatalf("Save(snapshot %d) returned error: %v", id, err)
		}
	}

	conversationRepo := conversations.NewSQLiteRepository(store.DB())
	handler := api.NewGatewayHandler(accountRepo, usageRepo, conversationRepo)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{
		"model":"gpt-5.2-codex",
		"stream":false,
		"messages":[{"role":"user","content":"ping"}]
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("gateway status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestGatewayHandlerPrefersActiveAccountWhenAutoFailoverEnabled(t *testing.T) {
	t.Parallel()

	highPriorityCalls := 0
	highPriorityUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		highPriorityCalls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "chat.completion",
			"model":  "gpt-5.2-codex",
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "should-not-run"}},
			},
		})
	}))
	defer highPriorityUpstream.Close()

	activeCalls := 0
	activeUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		activeCalls++
		if got := r.Header.Get("Authorization"); got != "Bearer sk-active" {
			t.Fatalf("Authorization = %q, want %q", got, "Bearer sk-active")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "chat.completion",
			"model":  "gpt-5.2-codex",
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "active-first"}},
			},
		})
	}))
	defer activeUpstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	for _, item := range []accounts.Account{
		{
			ProviderType:  accounts.ProviderOpenAICompatible,
			AccountName:   "higher-priority",
			AuthMode:      accounts.AuthModeAPIKey,
			BaseURL:       highPriorityUpstream.URL + "/v1",
			CredentialRef: "sk-high",
			Status:        accounts.StatusActive,
			Priority:      100,
		},
		{
			ProviderType:  accounts.ProviderOpenAICompatible,
			AccountName:   "manual-active",
			AuthMode:      accounts.AuthModeAPIKey,
			BaseURL:       activeUpstream.URL + "/v1",
			CredentialRef: "sk-active",
			Status:        accounts.StatusActive,
			Priority:      10,
			IsActive:      true,
		},
	} {
		if err := accountRepo.Create(item); err != nil {
			t.Fatalf("Create(account) returned error: %v", err)
		}
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	for id := int64(1); id <= 2; id++ {
		if err := usageRepo.Save(usage.Snapshot{
			AccountID:      id,
			Balance:        100,
			QuotaRemaining: 100000,
			RPMRemaining:   100,
			TPMRemaining:   100000,
			HealthScore:    0.9,
		}); err != nil {
			t.Fatalf("Save(snapshot %d) returned error: %v", id, err)
		}
	}

	settingsRepo := settings.NewSQLiteRepository(store.DB())
	current := settings.DefaultAppSettings()
	current.AutoFailoverEnabled = true
	if err := settingsRepo.SaveAppSettings(current); err != nil {
		t.Fatalf("SaveAppSettings returned error: %v", err)
	}

	handler := api.NewGatewayHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()), api.WithGatewaySettings(settingsRepo))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{
		"model":"gpt-5.2-codex",
		"stream":false,
		"messages":[{"role":"user","content":"ping"}]
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("gateway status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if activeCalls != 1 {
		t.Fatalf("activeCalls = %d, want 1", activeCalls)
	}
	if highPriorityCalls != 0 {
		t.Fatalf("highPriorityCalls = %d, want 0", highPriorityCalls)
	}
}
