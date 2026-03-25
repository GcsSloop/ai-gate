package api_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/api"
	"github.com/gcssloop/codex-router/backend/internal/conversations"
	"github.com/gcssloop/codex-router/backend/internal/settings"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()

	var count int
	query := "SELECT COUNT(*) FROM " + table
	if err := db.QueryRow(query).Scan(&count); err != nil {
		t.Fatalf("count %s returned error: %v", table, err)
	}
	return count
}

func TestResponsesHandlerThinModeThirdPartyResponsesPassthrough(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %q, want /v1/responses", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-third-party" {
			t.Fatalf("authorization = %q, want Bearer sk-third-party", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_tp_1","object":"response","status":"completed","output_text":"tp-pong"}`)
	}))
	defer upstream.Close()

	handler := newResponsesHandlerTestHandler(t, accounts.Account{
		ProviderType:      accounts.ProviderOpenAICompatible,
		AccountName:       "team3",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           upstream.URL + "/v1",
		CredentialRef:     "sk-third-party",
		Status:            accounts.StatusActive,
		Priority:          100,
		SupportsResponses: true,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"output_text":"tp-pong"`) {
		t.Fatalf("body = %s, want third-party output", rec.Body.String())
	}
}

func TestResponsesHandlerThinModeRecordsUsageEventWithoutAuditRows(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id":"resp_tp_usage_1",
			"object":"response",
			"status":"completed",
			"output_text":"tp-pong",
			"usage":{"input_tokens":1200,"output_tokens":300,"total_tokens":1500}
		}`)
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
		SupportsResponses: true,
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
		t.Fatalf("Save(snapshot) returned error: %v", err)
	}

	settingsRepo := settings.NewSQLiteRepository(store.DB())
	current := settings.DefaultAppSettings()
	current.AutoFailoverEnabled = true
	if err := settingsRepo.SaveAppSettings(current); err != nil {
		t.Fatalf("SaveAppSettings returned error: %v", err)
	}

	handler := api.NewResponsesHandler(
		accountRepo,
		usageRepo,
		conversations.NewSQLiteRepository(store.DB()),
		api.WithResponsesSettings(settingsRepo),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if count := countRows(t, store.DB(), "usage_events"); count != 1 {
		t.Fatalf("usage_events row count = %d, want 1", count)
	}
	for _, table := range []string{"conversations", "messages", "runs"} {
		if count := countRows(t, store.DB(), table); count != 0 {
			t.Fatalf("%s row count = %d, want 0", table, count)
		}
	}
}

func TestResponsesHandlerThinModeStreamCapturesUsageTokens(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, ""+
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"tp-pong\"}\n\n"+
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_tp_usage_stream_1\",\"usage\":{\"input_tokens\":1200,\"input_tokens_details\":{\"cached_tokens\":100},\"output_tokens\":300,\"total_tokens\":1500}}}\n\n"+
			"data: [DONE]\n\n")
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
		AccountName:       "team3-stream",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           upstream.URL + "/v1",
		CredentialRef:     "sk-third-party",
		Status:            accounts.StatusActive,
		Priority:          100,
		SupportsResponses: true,
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
		t.Fatalf("Save(snapshot) returned error: %v", err)
	}

	settingsRepo := settings.NewSQLiteRepository(store.DB())
	current := settings.DefaultAppSettings()
	current.AutoFailoverEnabled = true
	if err := settingsRepo.SaveAppSettings(current); err != nil {
		t.Fatalf("SaveAppSettings returned error: %v", err)
	}

	handler := api.NewResponsesHandler(
		accountRepo,
		usageRepo,
		conversations.NewSQLiteRepository(store.DB()),
		api.WithResponsesSettings(settingsRepo),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"ping","stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	events, err := usageRepo.ListRecentEvents(usage.EventFilter{Limit: 5})
	if err != nil {
		t.Fatalf("ListRecentEvents returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	if events[0].InputTokens != 1300 || events[0].OutputTokens != 300 || events[0].TotalTokens != 1500 {
		t.Fatalf("event tokens = in:%d out:%d total:%d, want in:1300 out:300 total:1500", events[0].InputTokens, events[0].OutputTokens, events[0].TotalTokens)
	}
}

func TestResponsesHandlerThinModeOfficialStreamCapturesWrappedTokenCount(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, ""+
			"data: {\"msg\":{\"type\":\"token_count\",\"info\":{\"total_token_usage\":{\"input_tokens\":1200,\"cached_input_tokens\":100,\"output_tokens\":300,\"total_tokens\":1600},\"model_context_window\":200000}}}\n\n"+
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"official-pong\"}\n\n"+
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_official_usage_stream_1\",\"output\":[]}}\n\n"+
			"data: [DONE]\n\n")
	}))
	defer upstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	if err := accountRepo.Create(accounts.Account{
		ProviderType: accounts.ProviderOpenAIOfficial,
		AccountName:  "official-stream",
		AuthMode:     accounts.AuthModeLocalImport,
		BaseURL:      upstream.URL + "/backend-api/codex",
		CredentialRef: `{
			"auth_mode":"chatgpt",
			"tokens":{"access_token":"token-1","account_id":"acct-1"}
		}`,
		Status:            accounts.StatusActive,
		Priority:          100,
		SupportsResponses: true,
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
		t.Fatalf("Save(snapshot) returned error: %v", err)
	}

	settingsRepo := settings.NewSQLiteRepository(store.DB())
	current := settings.DefaultAppSettings()
	current.AutoFailoverEnabled = true
	if err := settingsRepo.SaveAppSettings(current); err != nil {
		t.Fatalf("SaveAppSettings returned error: %v", err)
	}

	handler := api.NewResponsesHandler(
		accountRepo,
		usageRepo,
		conversations.NewSQLiteRepository(store.DB()),
		api.WithResponsesSettings(settingsRepo),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"ping","stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	events, err := usageRepo.ListRecentEvents(usage.EventFilter{Limit: 5})
	if err != nil {
		t.Fatalf("ListRecentEvents returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	if events[0].InputTokens != 1300 || events[0].OutputTokens != 300 || events[0].TotalTokens != 1600 {
		t.Fatalf("event tokens = in:%d out:%d total:%d, want in:1300 out:300 total:1600", events[0].InputTokens, events[0].OutputTokens, events[0].TotalTokens)
	}
}

func TestResponsesHandlerPreservesExistingWindowUsageWhenResponseLacksRateLimits(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, ""+
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"official-pong\"}\n\n"+
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_preserve_usage_1\",\"usage\":{\"input_tokens\":100,\"input_tokens_details\":{\"cached_tokens\":20},\"output_tokens\":30,\"total_tokens\":130},\"output\":[]}}\n\n"+
			"data: [DONE]\n\n")
	}))
	defer upstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	if err := accountRepo.Create(accounts.Account{
		ProviderType: accounts.ProviderOpenAIOfficial,
		AccountName:  "official-preserve",
		AuthMode:     accounts.AuthModeLocalImport,
		BaseURL:      upstream.URL + "/backend-api/codex",
		CredentialRef: `{
			"auth_mode":"chatgpt",
			"tokens":{"access_token":"token-1","account_id":"acct-1"}
		}`,
		Status:            accounts.StatusActive,
		Priority:          100,
		SupportsResponses: true,
		IsActive:          true,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	primaryReset := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	secondaryReset := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	if err := usageRepo.Save(usage.Snapshot{
		AccountID:            1,
		Source:               "remote",
		Confidence:           "high",
		ProviderSnapshotJSON: `{"capacity_model":"official_window","has_rpm":true,"has_tpm":true}`,
		Balance:              0,
		RPMRemaining:         70,
		TPMRemaining:         3,
		HealthScore:          0.365,
		PrimaryUsedPercent:   30,
		SecondaryUsedPercent: 97,
		PrimaryResetsAt:      &primaryReset,
		SecondaryResetsAt:    &secondaryReset,
		CheckedAt:            time.Now().UTC().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	settingsRepo := settings.NewSQLiteRepository(store.DB())
	current := settings.DefaultAppSettings()
	current.AutoFailoverEnabled = true
	if err := settingsRepo.SaveAppSettings(current); err != nil {
		t.Fatalf("SaveAppSettings returned error: %v", err)
	}

	handler := api.NewResponsesHandler(
		accountRepo,
		usageRepo,
		conversations.NewSQLiteRepository(store.DB()),
		api.WithResponsesSettings(settingsRepo),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"ping","stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	snapshot, err := usageRepo.GetLatest(1)
	if err != nil {
		t.Fatalf("GetLatest returned error: %v", err)
	}
	if snapshot.LastInputTokens != 120 || snapshot.LastOutputTokens != 30 || snapshot.LastTotalTokens != 130 {
		t.Fatalf("latest usage tokens = in:%v out:%v total:%v, want in:120 out:30 total:130", snapshot.LastInputTokens, snapshot.LastOutputTokens, snapshot.LastTotalTokens)
	}
	if snapshot.PrimaryUsedPercent != 30 {
		t.Fatalf("PrimaryUsedPercent = %v, want 30", snapshot.PrimaryUsedPercent)
	}
	if snapshot.SecondaryUsedPercent != 97 {
		t.Fatalf("SecondaryUsedPercent = %v, want 97", snapshot.SecondaryUsedPercent)
	}
	if snapshot.RPMRemaining != 70 {
		t.Fatalf("RPMRemaining = %v, want 70", snapshot.RPMRemaining)
	}
	if snapshot.TPMRemaining != 3 {
		t.Fatalf("TPMRemaining = %v, want 3", snapshot.TPMRemaining)
	}
	if snapshot.PrimaryResetsAt == nil || !snapshot.PrimaryResetsAt.Equal(primaryReset) {
		t.Fatalf("PrimaryResetsAt = %v, want %v", snapshot.PrimaryResetsAt, primaryReset)
	}
	if snapshot.SecondaryResetsAt == nil || !snapshot.SecondaryResetsAt.Equal(secondaryReset) {
		t.Fatalf("SecondaryResetsAt = %v, want %v", snapshot.SecondaryResetsAt, secondaryReset)
	}
}

func TestResponsesHandlerThinModeOfficialStreamWithoutContentTypeStillCapturesUsage(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, ""+
			"event: response.created\n"+
			"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_missing_header_1\",\"usage\":null}}\n\n"+
			"event: response.completed\n"+
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_missing_header_1\",\"output\":[],\"usage\":{\"input_tokens\":17,\"input_tokens_details\":{\"cached_tokens\":0},\"output_tokens\":5,\"total_tokens\":22}}}\n\n")
	}))
	defer upstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	if err := accountRepo.Create(accounts.Account{
		ProviderType: accounts.ProviderOpenAIOfficial,
		AccountName:  "official-stream-no-header",
		AuthMode:     accounts.AuthModeLocalImport,
		BaseURL:      upstream.URL + "/backend-api/codex",
		CredentialRef: `{
			"auth_mode":"chatgpt",
			"tokens":{"access_token":"token-1","account_id":"acct-1"}
		}`,
		Status:            accounts.StatusActive,
		Priority:          100,
		SupportsResponses: true,
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
		t.Fatalf("Save(snapshot) returned error: %v", err)
	}

	settingsRepo := settings.NewSQLiteRepository(store.DB())
	current := settings.DefaultAppSettings()
	current.AutoFailoverEnabled = true
	if err := settingsRepo.SaveAppSettings(current); err != nil {
		t.Fatalf("SaveAppSettings returned error: %v", err)
	}

	handler := api.NewResponsesHandler(
		accountRepo,
		usageRepo,
		conversations.NewSQLiteRepository(store.DB()),
		api.WithResponsesSettings(settingsRepo),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"ping","stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	events, err := usageRepo.ListRecentEvents(usage.EventFilter{Limit: 5})
	if err != nil {
		t.Fatalf("ListRecentEvents returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	if events[0].InputTokens != 17 || events[0].OutputTokens != 5 || events[0].TotalTokens != 22 {
		t.Fatalf("event tokens = in:%d out:%d total:%d, want in:17 out:5 total:22", events[0].InputTokens, events[0].OutputTokens, events[0].TotalTokens)
	}
}

func TestResponsesHandlerThinModeRejectsUnsupportedActiveAccount(t *testing.T) {
	t.Parallel()

	handler := newResponsesHandlerTestHandler(t, accounts.Account{
		ProviderType:      accounts.ProviderOpenAICompatible,
		AccountName:       "legacy",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           "https://example.invalid/v1",
		CredentialRef:     "sk-third-party",
		Status:            accounts.StatusActive,
		Priority:          100,
		SupportsResponses: false,
		IsActive:          true,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "support /responses") {
		t.Fatalf("body = %s, want explicit capability error", rec.Body.String())
	}
}

func TestResponsesHandlerThinModeUsesPriorityOrderWhenAutoFailoverEnabled(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk-priority" {
			t.Fatalf("authorization = %q, want Bearer sk-priority", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_tp_q","object":"response","status":"completed","output_text":"priority-first"}`)
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
			ProviderType:      accounts.ProviderOpenAICompatible,
			AccountName:       "lower-priority",
			AuthMode:          accounts.AuthModeAPIKey,
			BaseURL:           upstream.URL + "/v1",
			CredentialRef:     "sk-lower",
			Status:            accounts.StatusActive,
			Priority:          10,
			SupportsResponses: true,
		},
		{
			ProviderType:      accounts.ProviderOpenAICompatible,
			AccountName:       "higher-priority",
			AuthMode:          accounts.AuthModeAPIKey,
			BaseURL:           upstream.URL + "/v1",
			CredentialRef:     "sk-priority",
			Status:            accounts.StatusActive,
			Priority:          100,
			SupportsResponses: true,
		},
	} {
		if err := accountRepo.Create(item); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	for _, snapshot := range []usage.Snapshot{
		{AccountID: 1, Balance: 100, QuotaRemaining: 100000, RPMRemaining: 100, TPMRemaining: 100000, HealthScore: 0.95},
		{AccountID: 2, Balance: 100, QuotaRemaining: 100000, RPMRemaining: 100, TPMRemaining: 100000, HealthScore: 0.5},
	} {
		if err := usageRepo.Save(snapshot); err != nil {
			t.Fatalf("Save returned error: %v", err)
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

	handler := api.NewResponsesHandler(
		accountRepo,
		usageRepo,
		conversations.NewSQLiteRepository(store.DB()),
		api.WithResponsesSettings(settingsRepo),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"output_text":"priority-first"`) {
		t.Fatalf("body = %s, want priority-selected output", rec.Body.String())
	}
}

func TestResponsesHandlerThinModePrefersActiveAccountWhenAvailable(t *testing.T) {
	t.Parallel()

	highPriorityCalls := 0
	highPriorityUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		highPriorityCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_high","object":"response","status":"completed","output_text":"should-not-run"}`)
	}))
	defer highPriorityUpstream.Close()

	activeCalls := 0
	activeUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		activeCalls++
		if got := r.Header.Get("Authorization"); got != "Bearer sk-active" {
			t.Fatalf("authorization = %q, want Bearer sk-active", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_active","object":"response","status":"completed","output_text":"active-first"}`)
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
			ProviderType:      accounts.ProviderOpenAICompatible,
			AccountName:       "higher-priority",
			AuthMode:          accounts.AuthModeAPIKey,
			BaseURL:           highPriorityUpstream.URL + "/v1",
			CredentialRef:     "sk-high",
			Status:            accounts.StatusActive,
			Priority:          100,
			SupportsResponses: true,
		},
		{
			ProviderType:      accounts.ProviderOpenAICompatible,
			AccountName:       "manual-active",
			AuthMode:          accounts.AuthModeAPIKey,
			BaseURL:           activeUpstream.URL + "/v1",
			CredentialRef:     "sk-active",
			Status:            accounts.StatusActive,
			Priority:          10,
			SupportsResponses: true,
			IsActive:          true,
		},
	} {
		if err := accountRepo.Create(item); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	for _, snapshot := range []usage.Snapshot{
		{AccountID: 1, Balance: 100, QuotaRemaining: 100000, RPMRemaining: 100, TPMRemaining: 100000, HealthScore: 0.95},
		{AccountID: 2, Balance: 100, QuotaRemaining: 100000, RPMRemaining: 100, TPMRemaining: 100000, HealthScore: 0.8},
	} {
		if err := usageRepo.Save(snapshot); err != nil {
			t.Fatalf("Save returned error: %v", err)
		}
	}

	settingsRepo := settings.NewSQLiteRepository(store.DB())
	current := settings.DefaultAppSettings()
	current.AutoFailoverEnabled = true
	if err := settingsRepo.SaveAppSettings(current); err != nil {
		t.Fatalf("SaveAppSettings returned error: %v", err)
	}

	handler := api.NewResponsesHandler(
		accountRepo,
		usageRepo,
		conversations.NewSQLiteRepository(store.DB()),
		api.WithResponsesSettings(settingsRepo),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if activeCalls != 1 {
		t.Fatalf("activeCalls = %d, want 1", activeCalls)
	}
	if highPriorityCalls != 0 {
		t.Fatalf("highPriorityCalls = %d, want 0", highPriorityCalls)
	}
	if !strings.Contains(rec.Body.String(), `"output_text":"active-first"`) {
		t.Fatalf("body = %s, want active-selected output", rec.Body.String())
	}
}

func TestResponsesHandlerThinModeUsesOnlyActiveAccountWhenAutoFailoverDisabled(t *testing.T) {
	t.Parallel()

	activeCalls := 0
	activeUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		activeCalls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"You've hit your usage limit. Upgrade to Pro or try again later."}}`)
	}))
	defer activeUpstream.Close()

	otherCalls := 0
	otherUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_other_1","object":"response","status":"completed","output_text":"should-not-switch"}`)
	}))
	defer otherUpstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	for _, account := range []accounts.Account{
		{
			ProviderType: accounts.ProviderOpenAIOfficial,
			AccountName:  "selected-active",
			AuthMode:     accounts.AuthModeLocalImport,
			BaseURL:      activeUpstream.URL + "/backend-api/codex",
			CredentialRef: `{
				"auth_mode":"chatgpt",
				"tokens":{"access_token":"token-active","account_id":"acct-active"}
			}`,
			Status:            accounts.StatusActive,
			Priority:          10,
			SupportsResponses: true,
			IsActive:          true,
		},
		{
			ProviderType:      accounts.ProviderOpenAICompatible,
			AccountName:       "fallback-candidate",
			AuthMode:          accounts.AuthModeAPIKey,
			BaseURL:           otherUpstream.URL + "/v1",
			CredentialRef:     "sk-fallback",
			Status:            accounts.StatusActive,
			Priority:          100,
			SupportsResponses: true,
		},
	} {
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	for _, snapshot := range []usage.Snapshot{
		{AccountID: 1, Balance: 5.39, QuotaRemaining: 100000, RPMRemaining: 85, TPMRemaining: 40, HealthScore: 0.9, CheckedAt: time.Now().UTC()},
		{AccountID: 2, Balance: 100, QuotaRemaining: 100000, RPMRemaining: 100, TPMRemaining: 100000, HealthScore: 0.8, CheckedAt: time.Now().UTC()},
	} {
		if err := usageRepo.Save(snapshot); err != nil {
			t.Fatalf("Save returned error: %v", err)
		}
	}

	settingsRepo := settings.NewSQLiteRepository(store.DB())
	current := settings.DefaultAppSettings()
	current.AutoFailoverEnabled = false
	if err := settingsRepo.SaveAppSettings(current); err != nil {
		t.Fatalf("SaveAppSettings returned error: %v", err)
	}

	handler := api.NewResponsesHandler(
		accountRepo,
		usageRepo,
		conversations.NewSQLiteRepository(store.DB()),
		api.WithResponsesSettings(settingsRepo),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusTooManyRequests, rec.Body.String())
	}
	if activeCalls != 1 {
		t.Fatalf("activeCalls = %d, want 1", activeCalls)
	}
	if otherCalls != 0 {
		t.Fatalf("otherCalls = %d, want 0", otherCalls)
	}
	if !strings.Contains(rec.Body.String(), "usage limit") {
		t.Fatalf("body = %s, want original upstream error", rec.Body.String())
	}
}

func TestResponsesHandlerThinModeDisablesSyntheticEndpoints(t *testing.T) {
	t.Parallel()

	handler := newResponsesHandlerTestHandler(t, accounts.Account{
		ProviderType:      accounts.ProviderOpenAICompatible,
		AccountName:       "team3",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           "https://example.invalid/v1",
		CredentialRef:     "sk-third-party",
		Status:            accounts.StatusActive,
		Priority:          100,
		SupportsResponses: true,
	})

	tests := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/v1/responses/resp_1"},
		{method: http.MethodGet, path: "/v1/responses/resp_1/input_items"},
		{method: http.MethodPost, path: "/v1/responses/resp_1/cancel"},
		{method: http.MethodDelete, path: "/v1/responses/resp_1"},
		{method: http.MethodPost, path: "/v1/responses/input_tokens"},
	}

	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
			}
		})
	}
}

func TestResponsesHandlerThinModePassesThroughCompactSubpath(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses/compact" {
			t.Fatalf("path = %q, want /v1/responses/compact", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %q, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-third-party" {
			t.Fatalf("authorization = %q, want Bearer sk-third-party", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		if string(body) != `{"model":"gpt-5.4","input":"compact-me"}` {
			t.Fatalf("body = %s, want passthrough payload", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":"resp_compact_1","status":"completed","object":"response"}`)
	}))
	defer upstream.Close()

	handler := newResponsesHandlerTestHandler(t, accounts.Account{
		ProviderType:      accounts.ProviderOpenAICompatible,
		AccountName:       "team3",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           upstream.URL + "/v1",
		CredentialRef:     "sk-third-party",
		Status:            accounts.StatusActive,
		Priority:          100,
		SupportsResponses: true,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses/compact", bytes.NewBufferString(`{"model":"gpt-5.4","input":"compact-me"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"id":"resp_compact_1"`) {
		t.Fatalf("body = %s, want compact passthrough response", rec.Body.String())
	}
}

func TestResponsesHandlerThinModeRetriesOfficialEOFOnce(t *testing.T) {
	t.Parallel()

	attempts := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("http.ResponseWriter does not implement Hijacker")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatalf("Hijack returned error: %v", err)
			}
			_ = conn.Close()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_official_1","object":"response","status":"completed","output_text":"official-after-retry"}`)
	}))
	defer upstream.Close()

	handler := newResponsesHandlerTestHandler(t, accounts.Account{
		ProviderType: accounts.ProviderOpenAIOfficial,
		AccountName:  "official",
		AuthMode:     accounts.AuthModeLocalImport,
		BaseURL:      upstream.URL + "/backend-api/codex",
		CredentialRef: `{
			"auth_mode":"chatgpt",
			"tokens":{"access_token":"token-1","account_id":"acct-1"}
		}`,
		Status:            accounts.StatusActive,
		Priority:          100,
		SupportsResponses: true,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if !strings.Contains(rec.Body.String(), `"output_text":"official-after-retry"`) {
		t.Fatalf("body = %s, want retried output", rec.Body.String())
	}
}

func newResponsesHandlerTestHandler(t *testing.T, account accounts.Account) http.Handler {
	t.Helper()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	if err := accountRepo.Create(account); err != nil {
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

	return api.NewResponsesHandler(accountRepo, usageRepo, conversations.NewSQLiteRepository(store.DB()))
}

func TestResponsesHandlerThinModePassesThroughPreviousResponseID(t *testing.T) {
	t.Parallel()

	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Fatalf("Decode returned error: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_tp_2","object":"response","status":"completed","output_text":"next"}`)
	}))
	defer upstream.Close()

	handler := newResponsesHandlerTestHandler(t, accounts.Account{
		ProviderType:      accounts.ProviderOpenAICompatible,
		AccountName:       "team3",
		AuthMode:          accounts.AuthModeAPIKey,
		BaseURL:           upstream.URL + "/v1",
		CredentialRef:     "sk-third-party",
		Status:            accounts.StatusActive,
		Priority:          100,
		SupportsResponses: true,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"next","previous_response_id":"resp_prev_1"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got, _ := upstreamBody["previous_response_id"].(string); got != "resp_prev_1" {
		t.Fatalf("previous_response_id = %q, want resp_prev_1", got)
	}
}

func TestResponsesHandlerThinModeMarksOfficialUsageLimitedFrom429Body(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"You've hit your usage limit. Visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Mar 19th, 2026 8:37 AM."}}`)
	}))
	defer upstream.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	if err := accountRepo.Create(accounts.Account{
		ProviderType: accounts.ProviderOpenAIOfficial,
		AccountName:  "official-usage-limit",
		AuthMode:     accounts.AuthModeLocalImport,
		BaseURL:      upstream.URL + "/backend-api/codex",
		CredentialRef: `{
			"auth_mode":"chatgpt",
			"tokens":{"access_token":"token-1","account_id":"acct-1"}
		}`,
		Status:            accounts.StatusActive,
		Priority:          100,
		SupportsResponses: true,
		IsActive:          true,
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	if err := usageRepo.Save(usage.Snapshot{
		AccountID:      1,
		Balance:        5.39,
		QuotaRemaining: 100000,
		RPMRemaining:   85,
		TPMRemaining:   40,
		HealthScore:    0.9,
		CheckedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	settingsRepo := settings.NewSQLiteRepository(store.DB())
	current := settings.DefaultAppSettings()
	current.AutoFailoverEnabled = true
	if err := settingsRepo.SaveAppSettings(current); err != nil {
		t.Fatalf("SaveAppSettings returned error: %v", err)
	}

	handler := api.NewResponsesHandler(
		accountRepo,
		usageRepo,
		conversations.NewSQLiteRepository(store.DB()),
		api.WithResponsesSettings(settingsRepo),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusTooManyRequests, rec.Body.String())
	}

	events, err := usageRepo.ListRecentEvents(usage.EventFilter{Limit: 5})
	if err != nil {
		t.Fatalf("ListRecentEvents returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	if events[0].Status != "usage_limited" {
		t.Fatalf("event status = %q, want usage_limited", events[0].Status)
	}
}

func TestResponsesHandlerThinModeFailsOverAfterOfficialUsageLimit(t *testing.T) {
	t.Parallel()

	primaryCalls := 0
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryCalls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"You've hit your usage limit. Upgrade to Pro or try again later."}}`)
	}))
	defer primary.Close()

	secondaryCalls := 0
	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondaryCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_fallback_1","object":"response","status":"completed","output_text":"fallback-success"}`)
	}))
	defer secondary.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	for _, account := range []accounts.Account{
		{
			ProviderType: accounts.ProviderOpenAIOfficial,
			AccountName:  "official-primary",
			AuthMode:     accounts.AuthModeLocalImport,
			BaseURL:      primary.URL + "/backend-api/codex",
			CredentialRef: `{
				"auth_mode":"chatgpt",
				"tokens":{"access_token":"token-primary","account_id":"acct-primary"}
			}`,
			Status:            accounts.StatusActive,
			Priority:          100,
			SupportsResponses: true,
			IsActive:          true,
		},
		{
			ProviderType:      accounts.ProviderOpenAICompatible,
			AccountName:       "fallback-third-party",
			AuthMode:          accounts.AuthModeAPIKey,
			BaseURL:           secondary.URL + "/v1",
			CredentialRef:     "sk-fallback",
			Status:            accounts.StatusActive,
			Priority:          90,
			SupportsResponses: true,
		},
	} {
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	for _, snapshot := range []usage.Snapshot{
		{
			AccountID:            1,
			Balance:              5.39,
			QuotaRemaining:       100000,
			RPMRemaining:         85,
			TPMRemaining:         40,
			HealthScore:          0.9,
			PrimaryUsedPercent:   72,
			SecondaryUsedPercent: 41,
			PrimaryResetsAt:      timePtr(time.Now().UTC().Add(2 * time.Hour)),
			SecondaryResetsAt:    timePtr(time.Now().UTC().Add(24 * time.Hour)),
			CheckedAt:            time.Now().UTC(),
		},
		{
			AccountID:      2,
			Balance:        100,
			QuotaRemaining: 100000,
			RPMRemaining:   100,
			TPMRemaining:   100000,
			HealthScore:    0.8,
			CheckedAt:      time.Now().UTC(),
		},
	} {
		if err := usageRepo.Save(snapshot); err != nil {
			t.Fatalf("Save returned error: %v", err)
		}
	}

	settingsRepo := settings.NewSQLiteRepository(store.DB())
	current := settings.DefaultAppSettings()
	current.AutoFailoverEnabled = true
	if err := settingsRepo.SaveAppSettings(current); err != nil {
		t.Fatalf("SaveAppSettings returned error: %v", err)
	}

	stateEvents := api.NewStateEventBus()
	eventCtx, cancelEvents := context.WithCancel(context.Background())
	defer cancelEvents()
	eventCh := stateEvents.Subscribe(eventCtx)

	handler := api.NewResponsesHandler(
		accountRepo,
		usageRepo,
		conversations.NewSQLiteRepository(store.DB()),
		api.WithResponsesSettings(settingsRepo),
		api.WithResponsesStateEvents(stateEvents),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"output_text":"fallback-success"`) {
		t.Fatalf("body = %s, want fallback output", rec.Body.String())
	}
	if primaryCalls != 1 {
		t.Fatalf("primaryCalls = %d, want 1", primaryCalls)
	}
	if secondaryCalls != 1 {
		t.Fatalf("secondaryCalls = %d, want 1", secondaryCalls)
	}

	primaryAccount, err := accountRepo.GetByID(1)
	if err != nil {
		t.Fatalf("GetByID returned error: %v", err)
	}
	if primaryAccount.Status != accounts.StatusActive {
		t.Fatalf("status = %q, want %q", primaryAccount.Status, accounts.StatusActive)
	}
	if primaryAccount.CooldownUntil == nil {
		t.Fatal("CooldownUntil = nil, want cooldown timestamp")
	}
	if primaryAccount.CooldownReason != "usage_limited" {
		t.Fatalf("CooldownReason = %q, want usage_limited", primaryAccount.CooldownReason)
	}
	fallbackAccount, err := accountRepo.GetByID(2)
	if err != nil {
		t.Fatalf("GetByID fallback returned error: %v", err)
	}
	if !fallbackAccount.IsActive {
		t.Fatal("fallback IsActive = false, want true after failover")
	}
	select {
	case topic := <-eventCh:
		if topic != api.AccountRoutingStateChangedTopic {
			t.Fatalf("event topic = %q, want %q", topic, api.AccountRoutingStateChangedTopic)
		}
	default:
		t.Fatal("expected account routing state event after failover")
	}

	events, err := usageRepo.ListRecentEvents(usage.EventFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListRecentEvents returned error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2", len(events))
	}
	if events[0].AccountID != 2 || events[0].Status != "completed" {
		t.Fatalf("latest event = %+v, want fallback completed", events[0])
	}
	if events[1].AccountID != 1 || events[1].Status != "usage_limited" {
		t.Fatalf("previous event = %+v, want primary usage_limited", events[1])
	}
}

func TestResponsesHandlerThinModeFailsOverAfterThirdPartyForbiddenQuotaError(t *testing.T) {
	t.Parallel()

	primaryCalls := 0
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryCalls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":{"message":"令牌[Q8g0b**************************************FWIxL]额度不足"}}`)
	}))
	defer primary.Close()

	secondaryCalls := 0
	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondaryCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_fallback_403_quota","object":"response","status":"completed","output_text":"fallback-after-403"}`)
	}))
	defer secondary.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	for _, account := range []accounts.Account{
		{
			ProviderType:      accounts.ProviderOpenAICompatible,
			AccountName:       "third-party-primary",
			AuthMode:          accounts.AuthModeAPIKey,
			BaseURL:           primary.URL + "/v1",
			CredentialRef:     "sk-primary",
			Status:            accounts.StatusActive,
			Priority:          100,
			SupportsResponses: true,
			IsActive:          true,
		},
		{
			ProviderType:      accounts.ProviderOpenAICompatible,
			AccountName:       "fallback-third-party",
			AuthMode:          accounts.AuthModeAPIKey,
			BaseURL:           secondary.URL + "/v1",
			CredentialRef:     "sk-fallback",
			Status:            accounts.StatusActive,
			Priority:          90,
			SupportsResponses: true,
		},
	} {
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	for _, snapshot := range []usage.Snapshot{
		{
			AccountID:      1,
			Balance:        100,
			QuotaRemaining: 100000,
			RPMRemaining:   100,
			TPMRemaining:   100000,
			HealthScore:    0.9,
			CheckedAt:      time.Now().UTC(),
		},
		{
			AccountID:      2,
			Balance:        100,
			QuotaRemaining: 100000,
			RPMRemaining:   100,
			TPMRemaining:   100000,
			HealthScore:    0.8,
			CheckedAt:      time.Now().UTC(),
		},
	} {
		if err := usageRepo.Save(snapshot); err != nil {
			t.Fatalf("Save returned error: %v", err)
		}
	}

	settingsRepo := settings.NewSQLiteRepository(store.DB())
	current := settings.DefaultAppSettings()
	current.AutoFailoverEnabled = true
	if err := settingsRepo.SaveAppSettings(current); err != nil {
		t.Fatalf("SaveAppSettings returned error: %v", err)
	}

	handler := api.NewResponsesHandler(
		accountRepo,
		usageRepo,
		conversations.NewSQLiteRepository(store.DB()),
		api.WithResponsesSettings(settingsRepo),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"output_text":"fallback-after-403"`) {
		t.Fatalf("body = %s, want fallback output", rec.Body.String())
	}
	if primaryCalls != 1 {
		t.Fatalf("primaryCalls = %d, want 1", primaryCalls)
	}
	if secondaryCalls != 1 {
		t.Fatalf("secondaryCalls = %d, want 1", secondaryCalls)
	}

	primaryAccount, err := accountRepo.GetByID(1)
	if err != nil {
		t.Fatalf("GetByID returned error: %v", err)
	}
	if primaryAccount.Status != accounts.StatusActive {
		t.Fatalf("status = %q, want %q", primaryAccount.Status, accounts.StatusActive)
	}
	if primaryAccount.CooldownUntil == nil {
		t.Fatal("CooldownUntil = nil, want cooldown timestamp")
	}
	if primaryAccount.CooldownReason != "capacity_failed" {
		t.Fatalf("CooldownReason = %q, want capacity_failed", primaryAccount.CooldownReason)
	}

	events, err := usageRepo.ListRecentEvents(usage.EventFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListRecentEvents returned error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2", len(events))
	}
	if events[0].AccountID != 2 || events[0].Status != "completed" {
		t.Fatalf("latest event = %+v, want fallback completed", events[0])
	}
	if events[1].AccountID != 1 || events[1].Status != "capacity_failed" {
		t.Fatalf("previous event = %+v, want primary capacity_failed", events[1])
	}
}

func TestResponsesHandlerThinModeSkipsOfficialBelowRemainingThreshold(t *testing.T) {
	t.Parallel()

	primaryCalls := 0
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_should_not_run","object":"response","status":"completed","output_text":"should-not-run"}`)
	}))
	defer primary.Close()

	secondaryCalls := 0
	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondaryCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_threshold_fallback","object":"response","status":"completed","output_text":"threshold-fallback"}`)
	}))
	defer secondary.Close()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	for _, account := range []accounts.Account{
		{
			ProviderType: accounts.ProviderOpenAIOfficial,
			AccountName:  "official-low-remaining",
			AuthMode:     accounts.AuthModeLocalImport,
			BaseURL:      primary.URL + "/backend-api/codex",
			CredentialRef: `{
				"auth_mode":"chatgpt",
				"tokens":{"access_token":"token-primary","account_id":"acct-primary"}
			}`,
			Status:            accounts.StatusActive,
			Priority:          100,
			SupportsResponses: true,
			IsActive:          true,
		},
		{
			ProviderType:      accounts.ProviderOpenAICompatible,
			AccountName:       "fallback-third-party",
			AuthMode:          accounts.AuthModeAPIKey,
			BaseURL:           secondary.URL + "/v1",
			CredentialRef:     "sk-fallback",
			Status:            accounts.StatusActive,
			Priority:          90,
			SupportsResponses: true,
		},
	} {
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	for _, snapshot := range []usage.Snapshot{
		{
			AccountID:            1,
			Balance:              5.39,
			QuotaRemaining:       100000,
			RPMRemaining:         2,
			TPMRemaining:         2,
			HealthScore:          0.02,
			PrimaryUsedPercent:   98.5,
			SecondaryUsedPercent: 97.4,
			PrimaryResetsAt:      timePtr(time.Now().UTC().Add(2 * time.Hour)),
			SecondaryResetsAt:    timePtr(time.Now().UTC().Add(24 * time.Hour)),
			CheckedAt:            time.Now().UTC(),
		},
		{
			AccountID:      2,
			Balance:        100,
			QuotaRemaining: 100000,
			RPMRemaining:   100,
			TPMRemaining:   100000,
			HealthScore:    0.8,
			CheckedAt:      time.Now().UTC(),
		},
	} {
		if err := usageRepo.Save(snapshot); err != nil {
			t.Fatalf("Save returned error: %v", err)
		}
	}

	settingsRepo := settings.NewSQLiteRepository(store.DB())
	current := settings.DefaultAppSettings()
	current.AutoFailoverEnabled = true
	if err := settingsRepo.SaveAppSettings(current); err != nil {
		t.Fatalf("SaveAppSettings returned error: %v", err)
	}

	handler := api.NewResponsesHandler(
		accountRepo,
		usageRepo,
		conversations.NewSQLiteRepository(store.DB()),
		api.WithResponsesSettings(settingsRepo),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","input":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"output_text":"threshold-fallback"`) {
		t.Fatalf("body = %s, want threshold fallback output", rec.Body.String())
	}
	if primaryCalls != 0 {
		t.Fatalf("primaryCalls = %d, want 0", primaryCalls)
	}
	if secondaryCalls != 1 {
		t.Fatalf("secondaryCalls = %d, want 1", secondaryCalls)
	}
}

func timePtr(value time.Time) *time.Time {
	return &value
}
