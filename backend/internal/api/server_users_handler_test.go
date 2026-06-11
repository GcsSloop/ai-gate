package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/serverusers"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
)

func TestServerUsersHandlerCreateListDisableAndRotate(t *testing.T) {
	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	handler := NewServerUsersHandler(serverusers.NewSQLiteRepository(store.DB()))

	createReq := httptest.NewRequest(http.MethodPost, "/server-users", bytes.NewBufferString(`{"name":"alice"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("POST /server-users status = %d, want %d; body=%s", createRec.Code, http.StatusCreated, createRec.Body.String())
	}
	var created serverusers.CreatedUser
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created user: %v", err)
	}
	if created.Token == "" || created.User.Name != "alice" {
		t.Fatalf("created = %+v, want token and user", created)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/server-users", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("GET /server-users status = %d, want %d", listRec.Code, http.StatusOK)
	}

	disableReq := httptest.NewRequest(http.MethodPost, "/server-users/1/disable", nil)
	disableRec := httptest.NewRecorder()
	handler.ServeHTTP(disableRec, disableReq)
	if disableRec.Code != http.StatusOK {
		t.Fatalf("POST disable status = %d, want %d; body=%s", disableRec.Code, http.StatusOK, disableRec.Body.String())
	}

	rotateReq := httptest.NewRequest(http.MethodPost, "/server-users/1/rotate-token", nil)
	rotateRec := httptest.NewRecorder()
	handler.ServeHTTP(rotateRec, rotateReq)
	if rotateRec.Code != http.StatusOK {
		t.Fatalf("POST rotate-token status = %d, want %d; body=%s", rotateRec.Code, http.StatusOK, rotateRec.Body.String())
	}
	var rotated serverusers.CreatedUser
	if err := json.Unmarshal(rotateRec.Body.Bytes(), &rotated); err != nil {
		t.Fatalf("unmarshal rotated user: %v", err)
	}
	if rotated.Token == "" || rotated.Token == created.Token {
		t.Fatalf("rotated token = %q, original token = %q", rotated.Token, created.Token)
	}
}
