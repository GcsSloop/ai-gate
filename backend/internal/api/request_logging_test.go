package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/conversations"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

func TestLogResponsesRequestSummaryReadableAndRedacted(t *testing.T) {
	summary := summarizeResponsesRequestLog(json.RawMessage(`"帮我查询最近的发布并返回总结"`), nil, "", false)
	if !strings.Contains(summary, `items=1`) {
		t.Fatalf("summary = %q, want items count", summary)
	}
	if !strings.Contains(summary, `roles=user`) {
		t.Fatalf("summary = %q, want user role", summary)
	}
	if !strings.Contains(summary, `preview="帮我查询最近的发布并返回总结"`) {
		t.Fatalf("summary = %q, want readable preview", summary)
	}
	if strings.Contains(summary, "Authorization") || strings.Contains(summary, "token") {
		t.Fatalf("summary = %q, want no sensitive tokens", summary)
	}
}

func TestResponsesHandlerLogsReadableRequestAndUpstreamSummary(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"pong"}}]}`))
	}))
	defer upstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	if err := accountRepo.Create(accounts.Account{
		ProviderType:      accounts.ProviderOpenAICompatible,
		AllowChatFallback: true,
		AccountName:       "ppchat-main",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           upstream.URL + "/v1",
		CredentialRef:     "sk-test-secret",
		Status:            accounts.StatusActive,
		Priority:          100,
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

	handler := NewResponsesHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()))

	var logs bytes.Buffer
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	}()

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{
		"model":"gpt-5.4",
		"input":"ping from user",
		"stream":false
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	output := logs.String()
	for _, want := range []string{
		`responses request path=/v1/responses method=POST stream=false`,
		`model_req=gpt-5.4`,
		`preview="ping from user"`,
		`responses upstream`,
		`account_id=1`,
		`account=ppchat-main`,
		`provider=openai-compatible`,
		`endpoint=/chat/completions`,
		`model_upstream=gpt-5.4`,
		`responses result`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("logs = %s, want substring %q", output, want)
		}
	}
	for _, unwanted := range []string{"sk-test-secret", "Authorization"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("logs = %s, should not contain %q", output, unwanted)
		}
	}
}

func TestGatewayHandlerLogsReadableRequestAndUpstreamSummary(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"object":"chat.completion","choices":[{"message":{"role":"assistant","content":"pong"}}]}`)
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
		AccountName:   "gateway-main",
		AuthMode:      accounts.AuthModeAPIKey,
		BaseURL:       upstream.URL + "/v1",
		CredentialRef: "sk-gateway-secret",
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

	handler := NewGatewayHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()))

	var logs bytes.Buffer
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	}()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{
		"model":"gpt-5.2-codex",
		"stream":false,
		"messages":[{"role":"user","content":"ping gateway"}]
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	output := logs.String()
	for _, want := range []string{
		`gateway request path=/v1/chat/completions method=POST stream=false`,
		`model_req=gpt-5.2-codex`,
		`preview="ping gateway"`,
		`gateway upstream`,
		`account_id=1`,
		`account=gateway-main`,
		`endpoint=/chat/completions`,
		`model_upstream=gpt-5.2-codex`,
		`gateway result`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("logs = %s, want substring %q", output, want)
		}
	}
	if strings.Contains(output, "sk-gateway-secret") {
		t.Fatalf("logs = %s, should not contain API key", output)
	}
}

func TestResponsesHandlerLogsThinGatewayCandidateSkipReason(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("upstream should not be called")
	}))
	defer upstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	if err := accountRepo.Create(accounts.Account{
		ProviderType:      accounts.ProviderOpenAICompatible,
		AccountName:       "team3",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           upstream.URL + "/v1",
		CredentialRef:     "sk-third-party",
		Status:            accounts.StatusActive,
		Priority:          100,
		IsActive:          true,
		SupportsResponses: false,
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

	handler := NewResponsesHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()), WithThinGatewayMode(true))

	var logs bytes.Buffer
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	}()

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
	output := logs.String()
	for _, want := range []string{
		`responses candidate account_id=1 account=team3 active=true supports_responses=false provider=openai-compatible action=reject`,
		`reason=active_account_missing_responses_capability`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("logs = %s, want substring %q", output, want)
		}
	}
}

func TestLogFailureSummaryIncludesTransportDiagnostics(t *testing.T) {
	var logs bytes.Buffer
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	}()

	logFailureSummary(
		"responses",
		99,
		14,
		"upstream_request",
		time.Now(),
		&url.Error{Op: "Post", URL: "https://chatgpt.com/backend-api/codex/responses", Err: io.EOF},
	)

	output := logs.String()
	for _, want := range []string{
		`stage=upstream_request`,
		`err_kind=url_error`,
		`url_op=Post`,
		`eof=true`,
		`timeout=false`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("logs = %s, want substring %q", output, want)
		}
	}
	if strings.Contains(output, "Authorization") {
		t.Fatalf("logs = %s, should not contain Authorization", output)
	}

	if errors.Is(io.EOF, io.EOF) == false {
		t.Fatal("sanity check failed")
	}
}
