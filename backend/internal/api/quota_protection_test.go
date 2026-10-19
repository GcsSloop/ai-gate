package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/api"
	"github.com/gcssloop/codex-router/backend/internal/conversations"
	"github.com/gcssloop/codex-router/backend/internal/serverusers"
	"github.com/gcssloop/codex-router/backend/internal/settings"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

func TestQuotaProtectionRouting(t *testing.T) {
	for _, tt := range []struct {
		name, path                                                            string
		stream, disable, thirdParty, noFallback, lockedFallback, autoFailover bool
		secondary                                                             bool
		serverMode                                                            bool
	}{
		{name: "responses five hour", path: "/v1/responses"},
		{name: "responses overall", path: "/v1/responses", secondary: true},
		{name: "responses stream", path: "/v1/responses", stream: true},
		{name: "responses server user", path: "/v1/responses", serverMode: true},
		{name: "responses auto failover", path: "/v1/responses", autoFailover: true},
		{name: "responses disabled", path: "/v1/responses", disable: true},
		{name: "responses third party", path: "/v1/responses", thirdParty: true},
		{name: "responses no fallback", path: "/v1/responses", noFallback: true},
		{name: "responses stream no fallback", path: "/v1/responses", noFallback: true, stream: true},
		{name: "responses locked fallback", path: "/v1/responses", lockedFallback: true},
		{name: "compact", path: "/v1/responses/compact"},
		{name: "compact no fallback", path: "/v1/responses/compact", noFallback: true},
		{name: "chat", path: "/v1/chat/completions"},
		{name: "chat stream", path: "/v1/chat/completions", stream: true},
		{name: "chat server user", path: "/v1/chat/completions", serverMode: true},
		{name: "chat disabled", path: "/v1/chat/completions", disable: true},
		{name: "chat no fallback", path: "/v1/chat/completions", noFallback: true},
		{name: "chat stream no fallback", path: "/v1/chat/completions", stream: true, noFallback: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var primaryCalls, fallbackCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				name := "primary"
				if strings.Contains(r.URL.Path, "/fallback/") {
					name = "fallback"
					fallbackCalls.Add(1)
				} else {
					primaryCalls.Add(1)
				}
				if tt.stream {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprintf(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_%s\",\"status\":\"completed\"}}\n\ndata: [DONE]\n\n", name)
				} else {
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprintf(w, `{"id":"resp_%s","object":"response","status":"completed","output_text":"%s"}`, name, name)
				}
			}))
			defer upstream.Close()
			store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			repo := accounts.NewSQLiteRepository(store.DB())
			usageRepo := usage.NewSQLiteRepository(store.DB())
			accountsHandler := api.NewAccountsHandler(repo, usageRepo, nil, nil)
			provider := accounts.ProviderOpenAIOfficial
			if tt.thirdParty {
				provider = accounts.ProviderOpenAICompatible
			}
			authMode := accounts.AuthModeLocalImport
			credential := `{"auth_mode":"chatgpt","tokens":{"access_token":"fixture-token","account_id":"fixture-account"}}`
			if tt.thirdParty {
				authMode = accounts.AuthModeAPIKey
				credential = "sk-fixture"
			}
			createBody, _ := json.Marshal(map[string]any{
				"provider_type": provider, "account_name": "primary", "auth_mode": authMode,
				"base_url": upstream.URL + "/primary/v1", "credential_ref": credential,
			})
			createRec := httptest.NewRecorder()
			accountsHandler.ServeHTTP(createRec, httptest.NewRequest(http.MethodPost, "/accounts", bytes.NewReader(createBody)))
			if createRec.Code != http.StatusCreated {
				t.Fatalf("create: %d %s", createRec.Code, createRec.Body.String())
			}
			primary, err := repo.GetByID(1)
			if err != nil {
				t.Fatal(err)
			}
			if !tt.thirdParty && !primary.QuotaProtectionEnabled {
				t.Fatal("official protection must default to enabled")
			}
			if tt.disable {
				updateRec := httptest.NewRecorder()
				accountsHandler.ServeHTTP(updateRec, httptest.NewRequest(http.MethodPut, "/accounts/1", strings.NewReader(`{"quota_protection_enabled":false}`)))
				if updateRec.Code != http.StatusOK {
					t.Fatalf("update: %s", updateRec.Body.String())
				}
			}
			if err := repo.SetActive(1); err != nil {
				t.Fatal(err)
			}
			if !tt.noFallback {
				if err := repo.Create(accounts.Account{
					ProviderType: accounts.ProviderOpenAICompatible, AccountName: "fallback", AuthMode: accounts.AuthModeAPIKey,
					BaseURL: upstream.URL + "/fallback/v1", CredentialRef: "sk-fallback", Status: accounts.StatusActive,
					SupportsResponses: true, IsLocked: tt.lockedFallback,
				}); err != nil {
					t.Fatal(err)
				}
			}
			snapshot := usage.Snapshot{AccountID: 1, PrimaryUsedPercent: 97, CheckedAt: time.Now().UTC()}
			if tt.secondary {
				snapshot.PrimaryUsedPercent = 10
				snapshot.SecondaryUsedPercent = 97
			}
			if err := usageRepo.Save(snapshot); err != nil {
				t.Fatal(err)
			}
			settingsRepo := settings.NewSQLiteRepository(store.DB())
			appSettings := settings.DefaultAppSettings()
			appSettings.AutoFailoverEnabled = tt.autoFailover
			if err := settingsRepo.SaveAppSettings(appSettings); err != nil {
				t.Fatal(err)
			}
			var handler http.Handler = api.NewResponsesHandler(repo, usageRepo, conversations.NewSQLiteRepository(store.DB()), api.WithResponsesSettings(settingsRepo))
			if strings.Contains(tt.path, "chat/completions") {
				handler = api.NewGatewayHandler(repo, usageRepo, conversations.NewSQLiteRepository(store.DB()), api.WithGatewaySettings(settingsRepo))
			}
			if tt.serverMode {
				preferredID := int64(1)
				handler = api.WithServerGatewayAuth(stickyResponsesUsers{user: serverusers.User{ID: 7, PreferredAccountID: &preferredID, RouteLocked: true}}, handler)
			}
			body := fmt.Sprintf(`{"model":"gpt-5.4","input":"ping","messages":[{"role":"user","content":"ping"}],"stream":%t}`, tt.stream)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(body)).WithContext(ctx)
			req.Header.Set("Authorization", "Bearer valid")
			handler.ServeHTTP(rec, req)
			if tt.noFallback || tt.lockedFallback {
				if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), `"code":"quota_protected"`) {
					t.Fatalf("protected response = %d %s", rec.Code, rec.Body.String())
				}
				if primaryCalls.Load() != 0 || fallbackCalls.Load() != 0 {
					t.Fatal("protected request reached upstream")
				}
				return
			}
			want := "fallback"
			if tt.disable || tt.thirdParty {
				want = "primary"
			}
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "resp_"+want) {
				t.Fatalf("response = %d %s; want %s", rec.Code, rec.Body.String(), want)
			}
			if tt.stream && !strings.Contains(rec.Body.String(), "[DONE]") {
				t.Fatal("stream missing terminal output")
			}
			if want == "fallback" && (primaryCalls.Load() != 0 || fallbackCalls.Load() != 1) {
				t.Fatalf("calls primary=%d fallback=%d", primaryCalls.Load(), fallbackCalls.Load())
			}
			if want == "primary" && (primaryCalls.Load() != 1 || fallbackCalls.Load() != 0) {
				t.Fatalf("calls primary=%d fallback=%d", primaryCalls.Load(), fallbackCalls.Load())
			}
			activeID := int64(2)
			if want == "primary" {
				activeID = 1
			}
			if tt.serverMode {
				activeID = 1 // User-scoped failover must not change the shared active account.
			}
			active, err := repo.GetByID(activeID)
			if err != nil || !active.IsActive {
				t.Fatalf("active account: %+v, %v", active, err)
			}
		})
	}
}
