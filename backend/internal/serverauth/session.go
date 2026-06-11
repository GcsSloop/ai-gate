package serverauth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

const CookieName = "ai_gate_session"

type Manager struct {
	password string
	ttl      time.Duration
	mu       sync.Mutex
	sessions map[string]time.Time
}

func NewManager(password string, ttl time.Duration) *Manager {
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	return &Manager{
		password: strings.TrimSpace(password),
		ttl:      ttl,
		sessions: make(map[string]time.Time),
	}
}

func (m *Manager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/login"):
		m.login(w, r)
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
		if m.validRequest(r) {
			next.ServeHTTP(w, r)
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
	if subtle.ConstantTimeCompare([]byte(payload.Password), []byte(m.password)) != 1 {
		http.Error(w, "invalid password", http.StatusUnauthorized)
		return
	}
	token, err := randomToken()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	expires := time.Now().UTC().Add(m.ttl)
	m.mu.Lock()
	m.sessions[token] = expires
	m.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
	})
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true})
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
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": m.validRequest(r)})
}

func (m *Manager) validRequest(r *http.Request) bool {
	cookie, err := r.Cookie(CookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return false
	}
	now := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	expires, ok := m.sessions[cookie.Value]
	if !ok {
		return false
	}
	if !expires.After(now) {
		delete(m.sessions, cookie.Value)
		return false
	}
	return true
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
