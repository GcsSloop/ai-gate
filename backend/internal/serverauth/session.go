package serverauth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/serverusers"
)

const CookieName = "ai_gate_session"

type UserStore interface {
	AuthenticateLogin(username string, token string) (serverusers.User, error)
}

type Session struct {
	Authenticated bool   `json:"authenticated"`
	Role          string `json:"role,omitempty"`
	UserID        int64  `json:"user_id,omitempty"`
	Username      string `json:"username,omitempty"`
	ExpiresAt     string `json:"expires_at,omitempty"`
}

type sessionRecord struct {
	session Session
	expires time.Time
}

type Manager struct {
	password string
	ttl      time.Duration
	users    UserStore
	mu       sync.Mutex
	sessions map[string]sessionRecord
}

func NewManager(password string, ttl time.Duration) *Manager {
	return NewManagerWithUsers(password, ttl, nil)
}

func NewManagerWithUsers(password string, ttl time.Duration, users UserStore) *Manager {
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	return &Manager{
		password: strings.TrimSpace(password),
		ttl:      ttl,
		users:    users,
		sessions: make(map[string]sessionRecord),
	}
}

func (m *Manager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/login"):
		m.login(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/user-login"):
		m.userLogin(w, r)
	case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/password"):
		m.changePassword(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/logout"):
		m.logout(w, r)
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/session"):
		m.session(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (m *Manager) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := m.sessionFromRequest(r)
		if ok && session.Role == serverusers.RoleAdmin {
			next.ServeHTTP(w, r)
			return
		}
		if ok {
			http.Error(w, "admin session required", http.StatusForbidden)
			return
		}
		http.Error(w, "authentication required", http.StatusUnauthorized)
	})
}

func (m *Manager) RequireUserSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := m.sessionFromRequest(r)
		if ok && session.Role == serverusers.RoleUser && session.UserID > 0 {
			next.ServeHTTP(w, r.WithContext(ContextWithSession(r.Context(), session)))
			return
		}
		if ok {
			http.Error(w, "user session required", http.StatusForbidden)
			return
		}
		http.Error(w, "authentication required", http.StatusUnauthorized)
	})
}

func (m *Manager) login(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid login payload", http.StatusBadRequest)
		return
	}
	if !m.passwordMatches(payload.Password) {
		http.Error(w, "invalid password", http.StatusUnauthorized)
		return
	}
	token, err := randomToken()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	expires := time.Now().UTC().Add(m.ttl)
	session := Session{Authenticated: true, Role: serverusers.RoleAdmin, ExpiresAt: expires.Format(time.RFC3339)}
	m.setSessionCookie(w, token, session, expires)
	writeJSON(w, http.StatusOK, session)
}

func (m *Manager) changePassword(w http.ResponseWriter, r *http.Request) {
	session, ok := m.sessionFromRequest(r)
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	if session.Role != serverusers.RoleAdmin {
		http.Error(w, "admin session required", http.StatusForbidden)
		return
	}
	var payload struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid password payload", http.StatusBadRequest)
		return
	}
	newPassword := strings.TrimSpace(payload.NewPassword)
	if newPassword == "" {
		http.Error(w, "new password is required", http.StatusBadRequest)
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if subtle.ConstantTimeCompare([]byte(payload.CurrentPassword), []byte(m.password)) != 1 {
		http.Error(w, "invalid current password", http.StatusUnauthorized)
		return
	}
	m.password = newPassword
	writeJSON(w, http.StatusOK, map[string]any{"updated": true})
}

func (m *Manager) userLogin(w http.ResponseWriter, r *http.Request) {
	if m.users == nil {
		http.Error(w, "user login is not configured", http.StatusNotFound)
		return
	}
	var payload struct {
		Username string `json:"username"`
		Token    string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid user login payload", http.StatusBadRequest)
		return
	}
	user, err := m.users.AuthenticateLogin(payload.Username, payload.Token)
	if err != nil {
		http.Error(w, "invalid username or token", http.StatusUnauthorized)
		return
	}
	token, err := randomToken()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	expires := time.Now().UTC().Add(m.ttl)
	session := Session{
		Authenticated: true,
		Role:          serverusers.RoleUser,
		UserID:        user.ID,
		Username:      user.Username,
		ExpiresAt:     expires.Format(time.RFC3339),
	}
	m.setSessionCookie(w, token, session, expires)
	writeJSON(w, http.StatusOK, session)
}

func (m *Manager) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(CookieName); err == nil {
		m.mu.Lock()
		delete(m.sessions, cookie.Value)
		m.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": false})
}

func (m *Manager) session(w http.ResponseWriter, r *http.Request) {
	session, ok := m.sessionFromRequest(r)
	if !ok {
		writeJSON(w, http.StatusOK, Session{Authenticated: false})
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func (m *Manager) validRequest(r *http.Request) bool {
	_, ok := m.sessionFromRequest(r)
	return ok
}

func (m *Manager) sessionFromRequest(r *http.Request) (Session, bool) {
	cookie, err := r.Cookie(CookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return Session{}, false
	}
	now := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	record, ok := m.sessions[cookie.Value]
	if !ok {
		return Session{}, false
	}
	if !record.expires.After(now) {
		delete(m.sessions, cookie.Value)
		return Session{}, false
	}
	return record.session, true
}

func (m *Manager) passwordMatches(password string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return subtle.ConstantTimeCompare([]byte(password), []byte(m.password)) == 1
}

func (m *Manager) setSessionCookie(w http.ResponseWriter, token string, session Session, expires time.Time) {
	m.mu.Lock()
	m.sessions[token] = sessionRecord{session: session, expires: expires}
	m.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
	})
}

type sessionContextKey struct{}

func ContextWithSession(ctx context.Context, session Session) context.Context {
	return context.WithValue(ctx, sessionContextKey{}, session)
}

func SessionFromContext(ctx context.Context) (Session, bool) {
	session, ok := ctx.Value(sessionContextKey{}).(Session)
	return session, ok
}

func randomToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
