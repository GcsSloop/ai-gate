package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	ClientID     string
	AuthorizeURL string
	TokenURL     string
	RedirectURL  string
	Scopes       []string
}

type Token struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

type OAuthConnector struct {
	config Config
}

func NewOAuthConnector(config Config) *OAuthConnector {
	return &OAuthConnector{config: config}
}

func (c *OAuthConnector) AuthorizationURL() (string, string, error) {
	state, err := randomState()
	if err != nil {
		return "", "", err
	}

	values := url.Values{}
	values.Set("client_id", c.config.ClientID)
	values.Set("redirect_uri", c.config.RedirectURL)
	values.Set("response_type", "code")
	values.Set("scope", strings.Join(c.config.Scopes, " "))
	values.Set("state", state)

	return c.config.AuthorizeURL + "?" + values.Encode(), state, nil
}

func (c *OAuthConnector) RefreshToken(token Token) (Token, error) {
	if token.RefreshToken == "" {
		return Token{}, errors.New("refresh token is required")
	}

	return Token{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    time.Now().UTC().Add(1 * time.Hour),
	}, nil
}

func ShouldRefresh(token Token, now time.Time, skew time.Duration) bool {
	if token.ExpiresAt.IsZero() {
		return false
	}
	return !token.ExpiresAt.After(now.Add(skew))
}

func randomState() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
