package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/settings"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
)

func TestLANShareAccessControlAllowsLoopbackEvenWithWhitelist(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	repo := settings.NewSQLiteRepository(store.DB())
	appSettings := settings.DefaultAppSettings()
	appSettings.LANShareEnabled = true
	appSettings.LANShareWhitelistEnabled = true
	appSettings.LANShareIPWhitelist = "192.168.1.10"
	if err := repo.SaveAppSettings(appSettings); err != nil {
		t.Fatalf("SaveAppSettings returned error: %v", err)
	}

	handler := withLANShareAccessControl(repo, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/ai-router/api/settings/app", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestLANShareAccessControlBlocksNonWhitelistedRemoteAddr(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	repo := settings.NewSQLiteRepository(store.DB())
	appSettings := settings.DefaultAppSettings()
	appSettings.LANShareEnabled = true
	appSettings.LANShareWhitelistEnabled = true
	appSettings.LANShareIPWhitelist = "192.168.1.10"
	if err := repo.SaveAppSettings(appSettings); err != nil {
		t.Fatalf("SaveAppSettings returned error: %v", err)
	}

	handler := withLANShareAccessControl(repo, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/ai-router/api/settings/app", nil)
	req.RemoteAddr = "192.168.1.11:54321"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestLANShareAccessControlAllowsAllWhenWhitelistDisabled(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	repo := settings.NewSQLiteRepository(store.DB())
	appSettings := settings.DefaultAppSettings()
	appSettings.LANShareEnabled = true
	appSettings.LANShareWhitelistEnabled = false
	appSettings.LANShareIPWhitelist = ""
	if err := repo.SaveAppSettings(appSettings); err != nil {
		t.Fatalf("SaveAppSettings returned error: %v", err)
	}

	handler := withLANShareAccessControl(repo, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/ai-router/api/settings/app", nil)
	req.RemoteAddr = "192.168.1.11:54321"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestLANShareAccessControlBlocksAllWhenWhitelistEnabledButEmpty(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	repo := settings.NewSQLiteRepository(store.DB())
	appSettings := settings.DefaultAppSettings()
	appSettings.LANShareEnabled = true
	appSettings.LANShareWhitelistEnabled = true
	appSettings.LANShareIPWhitelist = ""
	if err := repo.SaveAppSettings(appSettings); err != nil {
		t.Fatalf("SaveAppSettings returned error: %v", err)
	}

	handler := withLANShareAccessControl(repo, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/ai-router/api/settings/app", nil)
	req.RemoteAddr = "192.168.1.11:54321"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}
