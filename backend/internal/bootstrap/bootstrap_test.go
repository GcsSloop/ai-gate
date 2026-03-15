package bootstrap_test

import (
	"context"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/bootstrap"
)

func TestNewApp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

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
