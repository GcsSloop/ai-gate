package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ClientID     string
	AuthorizeURL string
	TokenURL     string
	RedirectURL  string
	Scopes       []string

	DeviceAuthUserCodeURL string
	DeviceVerificationURL string
}

type Token struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

type OAuthConnector struct {
	config Config
}

type DeviceAuthSession struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int64  `json:"expires_in,omitempty"`
	Interval        int64  `json:"interval,omitempty"`
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

func (c *OAuthConnector) StartDeviceAuth(ctx context.Context, client *http.Client) (DeviceAuthSession, error) {
	endpoint := strings.TrimSpace(c.config.DeviceAuthUserCodeURL)
	if endpoint == "" {
		return DeviceAuthSession{}, errors.New("device auth endpoint is not configured")
	}
	verificationURI := strings.TrimSpace(c.config.DeviceVerificationURL)
	if verificationURI == "" {
		verificationURI = "https://auth.openai.com/codex/device"
	}

	clientID := strings.TrimSpace(c.config.ClientID)
	if clientID == "" {
		// Match official Codex/OpenCode device auth client.
		clientID = "app_EMoamEEZ73f0CkXaXp7hrann"
	}

	payload, err := json.Marshal(map[string]string{
		"client_id": clientID,
	})
	if err != nil {
		return DeviceAuthSession{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(payload)))
	if err != nil {
		return DeviceAuthSession{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "codex-router")

	httpClient := client
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return DeviceAuthSession{}, fmt.Errorf("request device auth code: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return DeviceAuthSession{}, fmt.Errorf("read device auth response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return DeviceAuthSession{}, fmt.Errorf("device auth start failed: %s %s", resp.Status, strings.TrimSpace(string(raw)))
	}

	var result struct {
		DeviceAuthID string      `json:"device_auth_id"`
		UserCode     string      `json:"user_code"`
		Interval     interface{} `json:"interval"`
		ExpiresIn    int64       `json:"expires_in"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return DeviceAuthSession{}, fmt.Errorf("decode device auth response: %w", err)
	}
	if strings.TrimSpace(result.DeviceAuthID) == "" || strings.TrimSpace(result.UserCode) == "" {
		return DeviceAuthSession{}, errors.New("device auth response missing device_auth_id or user_code")
	}

	return DeviceAuthSession{
		DeviceCode:      strings.TrimSpace(result.DeviceAuthID),
		UserCode:        strings.TrimSpace(result.UserCode),
		VerificationURI: verificationURI,
		ExpiresIn:       result.ExpiresIn,
		Interval:        parseJSONNumber(result.Interval),
	}, nil
}

func parseJSONNumber(value interface{}) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case float32:
		return int64(v)
	case int:
		return int64(v)
	case int64:
		return v
	case json.Number:
		i, err := v.Int64()
		if err == nil {
			return i
		}
		f, ferr := v.Float64()
		if ferr == nil {
			return int64(f)
		}
	case string:
		i, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err == nil {
			return i
		}
	}
	return 0
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
