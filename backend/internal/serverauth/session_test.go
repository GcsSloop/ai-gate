package serverauth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/serverusers"
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

type fakeUserStore struct {
	user serverusers.User
	err  error
}

func (s fakeUserStore) AuthenticateLogin(username string, token string) (serverusers.User, error) {
	if s.err != nil {
		return serverusers.User{}, s.err
	}
	if username != s.user.Username || token != "agt-test" {
		return serverusers.User{}, http.ErrNoCookie
	}
	return s.user, nil
}

func TestManagerUserLoginSessionAndAdminProtection(t *testing.T) {
	manager := NewManagerWithUsers("secret-password", time.Minute, fakeUserStore{
		user: serverusers.User{ID: 7, Username: "alice", Name: "alice", Role: serverusers.RoleUser, Status: serverusers.StatusActive},
	})

	loginReq := httptest.NewRequest(http.MethodPost, "/auth/user-login", strings.NewReader(`{"username":"alice","token":"agt-test"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	manager.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("user-login status = %d, want %d; body=%s", loginRec.Code, http.StatusOK, loginRec.Body.String())
	}
	cookies := loginRec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != CookieName {
		t.Fatalf("user-login cookies = %+v, want %s", cookies, CookieName)
	}

	sessionReq := httptest.NewRequest(http.MethodGet, "/auth/session", nil)
	sessionReq.AddCookie(cookies[0])
	sessionRec := httptest.NewRecorder()
	manager.ServeHTTP(sessionRec, sessionReq)
	if sessionRec.Code != http.StatusOK {
		t.Fatalf("session status = %d, want %d", sessionRec.Code, http.StatusOK)
	}
	if body := sessionRec.Body.String(); !strings.Contains(body, `"role":"user"`) || !strings.Contains(body, `"user_id":7`) {
		t.Fatalf("session body = %s, want user role and id", body)
	}

	adminOnly := manager.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	adminReq := httptest.NewRequest(http.MethodGet, "/api/accounts", nil)
	adminReq.AddCookie(cookies[0])
	adminRec := httptest.NewRecorder()
	adminOnly.ServeHTTP(adminRec, adminReq)
	if adminRec.Code != http.StatusForbidden {
		t.Fatalf("ordinary user admin-only status = %d, want %d", adminRec.Code, http.StatusForbidden)
	}

	userOnly := manager.RequireUserSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := SessionFromContext(r.Context())
		if !ok || session.UserID != 7 || session.Role != serverusers.RoleUser {
			t.Fatalf("session from context = %+v, ok=%v; want user 7", session, ok)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	meReq := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	meReq.AddCookie(cookies[0])
	meRec := httptest.NewRecorder()
	userOnly.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusNoContent {
		t.Fatalf("ordinary user /me status = %d, want %d", meRec.Code, http.StatusNoContent)
	}
}
