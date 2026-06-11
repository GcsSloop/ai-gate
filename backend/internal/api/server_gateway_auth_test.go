package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/serverusers"
)

type fakeGatewayUserStore struct {
	user serverusers.User
}

func (s fakeGatewayUserStore) Authenticate(token string) (serverusers.User, error) {
	if token != "valid" {
		return serverusers.User{}, errFakeAuth
	}
	return s.user, nil
}

type fakeAuthError string

func (e fakeAuthError) Error() string { return string(e) }

const errFakeAuth = fakeAuthError("invalid")

func TestWithServerGatewayAuthRequiresValidToken(t *testing.T) {
	user := serverusers.User{ID: 7, Name: "alice", Status: serverusers.StatusActive, CreatedAt: time.Now().UTC()}
	handler := WithServerGatewayAuth(fakeGatewayUserStore{user: user}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := ServerUserFromContext(r.Context())
		if !ok || got.ID != 7 {
			t.Fatalf("ServerUserFromContext = %+v, %t; want user 7", got, ok)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	unauthReq := httptest.NewRequest(http.MethodPost, "/ai-gate/v1/responses", nil)
	unauthRec := httptest.NewRecorder()
	handler.ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status = %d, want %d", unauthRec.Code, http.StatusUnauthorized)
	}

	authReq := httptest.NewRequest(http.MethodPost, "/ai-gate/v1/responses", nil)
	authReq.Header.Set("Authorization", "Bearer valid")
	authRec := httptest.NewRecorder()
	handler.ServeHTTP(authRec, authReq)
	if authRec.Code != http.StatusNoContent {
		t.Fatalf("auth status = %d, want %d", authRec.Code, http.StatusNoContent)
	}
}
