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

	deleteReq := httptest.NewRequest(http.MethodDelete, "/server-users/1", nil)
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("DELETE /server-users/1 status = %d, want %d; body=%s", deleteRec.Code, http.StatusOK, deleteRec.Body.String())
	}
	listAfterDeleteReq := httptest.NewRequest(http.MethodGet, "/server-users", nil)
	listAfterDeleteRec := httptest.NewRecorder()
	handler.ServeHTTP(listAfterDeleteRec, listAfterDeleteReq)
	if listAfterDeleteRec.Code != http.StatusOK {
		t.Fatalf("GET /server-users after delete status = %d, want %d; body=%s", listAfterDeleteRec.Code, http.StatusOK, listAfterDeleteRec.Body.String())
	}
	var usersAfterDelete []serverusers.User
	if err := json.Unmarshal(listAfterDeleteRec.Body.Bytes(), &usersAfterDelete); err != nil {
		t.Fatalf("unmarshal users after delete: %v", err)
	}
	if len(usersAfterDelete) != 0 {
		t.Fatalf("users after delete = %+v, want empty", usersAfterDelete)
	}
}

func TestServerUsersHandlerDoesNotExposeAccountAssignments(t *testing.T) {
	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := serverusers.NewSQLiteRepository(store.DB())
	created, err := repo.Create("alice")
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	handler := NewServerUsersHandler(repo)

	listBeforeReq := httptest.NewRequest(http.MethodGet, "/server-users", nil)
	listBeforeRec := httptest.NewRecorder()
	handler.ServeHTTP(listBeforeRec, listBeforeReq)
	if listBeforeRec.Code != http.StatusOK {
		t.Fatalf("GET /server-users before status = %d, want %d; body=%s", listBeforeRec.Code, http.StatusOK, listBeforeRec.Body.String())
	}
	var usersBefore []serverusers.User
	if err := json.Unmarshal(listBeforeRec.Body.Bytes(), &usersBefore); err != nil {
		t.Fatalf("unmarshal users before: %v", err)
	}
	if len(usersBefore) != 1 {
		t.Fatalf("users before = %+v, want one user", usersBefore)
	}

	assignReq := httptest.NewRequest(http.MethodPut, "/server-users/1/accounts", bytes.NewBufferString(`{"account_ids":[1,2]}`))
	assignReq.Header.Set("Content-Type", "application/json")
	assignRec := httptest.NewRecorder()
	handler.ServeHTTP(assignRec, assignReq)
	if assignRec.Code != http.StatusNotFound {
		t.Fatalf("PUT /server-users/1/accounts status = %d, want %d; body=%s", assignRec.Code, http.StatusNotFound, assignRec.Body.String())
	}

	accountsReq := httptest.NewRequest(http.MethodGet, "/server-users/1/accounts", nil)
	accountsRec := httptest.NewRecorder()
	handler.ServeHTTP(accountsRec, accountsReq)
	if accountsRec.Code != http.StatusNotFound {
		t.Fatalf("GET /server-users/1/accounts status = %d, want %d; body=%s", accountsRec.Code, http.StatusNotFound, accountsRec.Body.String())
	}

	listAfterReq := httptest.NewRequest(http.MethodGet, "/server-users", nil)
	listAfterRec := httptest.NewRecorder()
	handler.ServeHTTP(listAfterRec, listAfterReq)
	if listAfterRec.Code != http.StatusOK {
		t.Fatalf("GET /server-users after status = %d, want %d; body=%s", listAfterRec.Code, http.StatusOK, listAfterRec.Body.String())
	}
	var usersAfter []serverusers.User
	if err := json.Unmarshal(listAfterRec.Body.Bytes(), &usersAfter); err != nil {
		t.Fatalf("unmarshal users after: %v", err)
	}
	if len(usersAfter) != 1 || usersAfter[0].ID != created.User.ID {
		t.Fatalf("users after = %+v, want created user", usersAfter)
	}
}
