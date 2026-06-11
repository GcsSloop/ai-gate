package serverauth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestManagerLoginAndRequireSession(t *testing.T) {
	manager := NewManager("secret-password", time.Minute)

	loginReq := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"password":"secret-password"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	manager.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d; body=%s", loginRec.Code, http.StatusOK, loginRec.Body.String())
	}
	cookies := loginRec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != CookieName {
		t.Fatalf("login cookies = %+v, want %s", cookies, CookieName)
	}

	protected := manager.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	unauthReq := httptest.NewRequest(http.MethodGet, "/api/accounts", nil)
	unauthRec := httptest.NewRecorder()
	protected.ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status = %d, want %d", unauthRec.Code, http.StatusUnauthorized)
	}

	authReq := httptest.NewRequest(http.MethodGet, "/api/accounts", nil)
	authReq.AddCookie(cookies[0])
	authRec := httptest.NewRecorder()
	protected.ServeHTTP(authRec, authReq)
	if authRec.Code != http.StatusNoContent {
		t.Fatalf("auth status = %d, want %d", authRec.Code, http.StatusNoContent)
	}
}

func TestManagerRejectsWrongPassword(t *testing.T) {
	manager := NewManager("secret-password", time.Minute)

	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"password":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	manager.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("login status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
