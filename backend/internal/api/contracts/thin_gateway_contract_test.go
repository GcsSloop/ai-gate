package contracts_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/api"
	"github.com/gcssloop/codex-router/backend/internal/conversations"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

func TestThinGatewayPassthroughJSONContract(t *testing.T) {
	t.Parallel()

	requestBody := `{"model":"gpt-5.4","input":"ping","stream":false}`
	responseBody := `{"id":"resp_contract_json","object":"response","status":"completed","output_text":"pong"}`
	var upstreamRequest string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		upstreamRequest = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, responseBody)
	}))
	defer upstream.Close()

	handler := newThinGatewayContractHandler(t, upstream.URL+"/backend-api/codex")
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(requestBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if upstreamRequest != requestBody {
		t.Fatalf("upstream request = %s, want exact passthrough %s", upstreamRequest, requestBody)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if strings.TrimSpace(rec.Body.String()) != responseBody {
		t.Fatalf("response body = %s, want exact passthrough %s", rec.Body.String(), responseBody)
	}
	if strings.Contains(rec.Body.String(), `"response_id":`) || strings.Contains(rec.Body.String(), `"sequence_number":`) {
		t.Fatalf("response body = %s, want no synthetic protocol fields", rec.Body.String())
	}
}

func TestThinGatewayPassthroughErrorContract(t *testing.T) {
	t.Parallel()

	errorBody := `{"error":{"type":"server_error","code":"upstream_503","message":"boom"}}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, errorBody)
	}))
	defer upstream.Close()

	handler := newThinGatewayContractHandler(t, upstream.URL+"/backend-api/codex")
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"ping","stream":false}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if strings.TrimSpace(rec.Body.String()) != errorBody {
		t.Fatalf("response body = %s, want exact passthrough %s", rec.Body.String(), errorBody)
	}
}

func TestThinGatewayPassthroughSSEContract(t *testing.T) {
	t.Parallel()

	streamFixture, err := os.ReadFile(filepath.Join("testdata", "passthrough_stream.http"))
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(streamFixture)
	}))
	defer upstream.Close()

	handler := newThinGatewayContractHandler(t, upstream.URL+"/backend-api/codex")
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"ping","stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	if rec.Body.String() != string(streamFixture) {
		t.Fatalf("stream body = %s, want exact passthrough %s", rec.Body.String(), string(streamFixture))
	}
	if strings.Contains(rec.Body.String(), `"response_id":`) || strings.Contains(rec.Body.String(), `"sequence_number":`) {
		t.Fatalf("stream body = %s, want no synthetic protocol fields", rec.Body.String())
	}
}

func newThinGatewayContractHandler(t *testing.T, baseURL string) http.Handler {
	t.Helper()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	if err := accountRepo.Create(accounts.Account{
		ProviderType: accounts.ProviderOpenAIOfficial,
		AccountName:  "official",
		AuthMode:     accounts.AuthModeLocalImport,
		BaseURL:      baseURL,
		CredentialRef: `{
			"auth_mode":"chatgpt",
			"tokens":{"access_token":"token-1","account_id":"acct-1"}
		}`,
		Status:   accounts.StatusActive,
		Priority: 100,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
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
		t.Fatalf("Save returned error: %v", err)
	}

	return api.NewResponsesHandler(
		accountRepo,
		usageRepo,
		conversations.NewSQLiteRepository(store.DB()),
		api.WithThinGatewayMode(true),
	)
}
