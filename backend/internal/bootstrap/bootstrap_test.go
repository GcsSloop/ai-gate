package bootstrap_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/bootstrap"
)

func TestNewApp(t *testing.T) {
	t.Parallel()

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
	if responsesRec.Code == http.StatusNotFound {
		t.Fatalf("POST /ai-router/api/responses status = %d, want non-404 alias route", responsesRec.Code)
	}
}
