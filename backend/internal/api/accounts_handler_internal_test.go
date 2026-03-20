package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/settings"
)

func TestNewAccountsHandlerDefaultsToFifteenSecondRefreshTTL(t *testing.T) {
	t.Parallel()

	handler := NewAccountsHandler(nil, nil, nil, nil)
	if handler.refreshTTL != 15*time.Second {
		t.Fatalf("refreshTTL = %s, want %s", handler.refreshTTL, 15*time.Second)
	}
}

type accountsSettingsStub struct {
	value settings.AppSettings
}

func (s accountsSettingsStub) GetAppSettings() (settings.AppSettings, error) {
	return s.value, nil
}

type refresherStub struct {
	deadline time.Time
}

func (s *refresherStub) Run(ctx context.Context, _ time.Time) error {
	s.deadline, _ = ctx.Deadline()
	return nil
}

func TestAccountsHandlerRefreshUsesLatestSettingsTimeout(t *testing.T) {
	t.Parallel()

	current := settings.DefaultAppSettings()
	current.UsageRequestTimeoutSeconds = 3
	refresher := &refresherStub{}
	handler := NewAccountsHandler(nil, nil, nil, nil, WithAccountsUsageRefresher(refresher), WithAccountsSettings(accountsSettingsStub{value: current}))

	start := time.Now()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/accounts/usage/refresh", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("POST /accounts/usage/refresh status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if refresher.deadline.IsZero() {
		t.Fatal("refresh deadline was not captured")
	}
	remaining := time.Until(refresher.deadline)
	if remaining < 2*time.Second || remaining > 4*time.Second {
		t.Fatalf("remaining timeout = %s, want about 3s (start=%s)", remaining, start)
	}
}
