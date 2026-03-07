package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNeedsLocalRefresh(t *testing.T) {
	t.Parallel()

	file := LocalAuthFile{
		AuthMode:    "chatgpt",
		LastRefresh: "2026-03-07T10:00:00Z",
	}
	file.Tokens.AccessToken = testJWT(t, map[string]any{
		"exp":       time.Now().UTC().Add(2 * time.Minute).Unix(),
		"client_id": "app-test-client",
	})
	file.Tokens.RefreshToken = "rt-test"

	if !NeedsLocalRefresh(file, time.Now().UTC(), 5*time.Minute) {
		t.Fatal("NeedsLocalRefresh = false, want true")
	}
}

func TestRefreshLocalAuthFile(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); !strings.Contains(got, "application/x-www-form-urlencoded") {
			t.Fatalf("Content-Type = %q, want form urlencoded", got)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		values, err := url.ParseQuery(string(raw))
		if err != nil {
			t.Fatalf("ParseQuery returned error: %v", err)
		}
		if values.Get("grant_type") != "refresh_token" {
			t.Fatalf("grant_type = %q, want refresh_token", values.Get("grant_type"))
		}
		if values.Get("refresh_token") != "rt-old" {
			t.Fatalf("refresh_token = %q, want rt-old", values.Get("refresh_token"))
		}
		if values.Get("client_id") != "app-test-client" {
			t.Fatalf("client_id = %q, want app-test-client", values.Get("client_id"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "at-new",
			"id_token":      "id-new",
			"refresh_token": "rt-new",
			"expires_in":    3600,
			"token_type":    "Bearer",
		})
	}))
	defer server.Close()

	file := LocalAuthFile{
		AuthMode:    "chatgpt",
		LastRefresh: "2026-03-07T10:00:00Z",
	}
	file.Tokens.AccessToken = testJWT(t, map[string]any{
		"exp":       time.Now().UTC().Add(-1 * time.Minute).Unix(),
		"client_id": "app-test-client",
	})
	file.Tokens.RefreshToken = "rt-old"
	file.Tokens.AccountID = "acct-1"

	refreshed, err := RefreshLocalAuthFile(context.Background(), http.DefaultClient, server.URL, file)
	if err != nil {
		t.Fatalf("RefreshLocalAuthFile returned error: %v", err)
	}
	if refreshed.Tokens.AccessToken != "at-new" {
		t.Fatalf("AccessToken = %q, want at-new", refreshed.Tokens.AccessToken)
	}
	if refreshed.Tokens.RefreshToken != "rt-new" {
		t.Fatalf("RefreshToken = %q, want rt-new", refreshed.Tokens.RefreshToken)
	}
	if refreshed.Tokens.IDToken != "id-new" {
		t.Fatalf("IDToken = %q, want id-new", refreshed.Tokens.IDToken)
	}
	if refreshed.LastRefresh == "" {
		t.Fatal("LastRefresh is empty, want populated value")
	}
}

func testJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	headerRaw, err := json.Marshal(map[string]any{"alg": "none", "typ": "JWT"})
	if err != nil {
		t.Fatalf("Marshal header returned error: %v", err)
	}
	claimsRaw, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("Marshal claims returned error: %v", err)
	}
	return encodeJWTPart(headerRaw) + "." + encodeJWTPart(claimsRaw) + "."
}

func encodeJWTPart(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}
