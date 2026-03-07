package auth_test

import (
	"net/url"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/auth"
)

func TestConnectorAuthorizationURL(t *testing.T) {
	t.Parallel()

	connector := auth.NewOAuthConnector(auth.Config{
		ClientID:     "client-id",
		AuthorizeURL: "https://auth.example.test/oauth/authorize",
		TokenURL:     "https://auth.example.test/oauth/token",
		RedirectURL:  "http://localhost:8080/callback",
		Scopes:       []string{"model.read", "usage.read"},
	})

	authURL, state, err := connector.AuthorizationURL()
	if err != nil {
		t.Fatalf("AuthorizationURL returned error: %v", err)
	}
	if state == "" {
		t.Fatal("AuthorizationURL returned empty state")
	}

	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("url.Parse returned error: %v", err)
	}
	if parsed.Query().Get("client_id") != "client-id" {
		t.Fatalf("client_id = %q, want %q", parsed.Query().Get("client_id"), "client-id")
	}
	if parsed.Query().Get("redirect_uri") != "http://localhost:8080/callback" {
		t.Fatalf("redirect_uri = %q, want redirect URL", parsed.Query().Get("redirect_uri"))
	}
}

func TestValidateState(t *testing.T) {
	t.Parallel()

	store := auth.NewStateStore(5 * time.Minute)
	state, err := store.New()
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if err := store.Validate(state); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if err := store.Validate(state); err == nil {
		t.Fatal("Validate returned nil error on replayed state")
	}
}

func TestShouldRefresh(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	token := auth.Token{
		AccessToken:  "access",
		RefreshToken: "refresh",
		ExpiresAt:    now.Add(2 * time.Minute),
	}

	if !auth.ShouldRefresh(token, now, 5*time.Minute) {
		t.Fatal("ShouldRefresh = false, want true near expiry")
	}
}

func TestRefreshRejectsMissingRefreshToken(t *testing.T) {
	t.Parallel()

	connector := auth.NewOAuthConnector(auth.Config{
		ClientID:     "client-id",
		AuthorizeURL: "https://auth.example.test/oauth/authorize",
		TokenURL:     "https://auth.example.test/oauth/token",
		RedirectURL:  "http://localhost:8080/callback",
	})

	_, err := connector.RefreshToken(auth.Token{})
	if err == nil {
		t.Fatal("RefreshToken returned nil error, want validation error")
	}
}
