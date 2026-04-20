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
	DeviceAuthTokenURL    string
	DeviceRedirectURL     string
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

var (
	ErrAuthorizationPending = errors.New("authorization pending")
	ErrDeviceCodeExpired    = errors.New("device code expired")
)

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

func (c *OAuthConnector) CompleteDeviceAuth(
	ctx context.Context,
	client *http.Client,
	deviceCode string,
	userCode string,
) ([]byte, error) {
	httpClient := client
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	deviceTokenEndpoint := strings.TrimSpace(c.config.DeviceAuthTokenURL)
	if deviceTokenEndpoint == "" {
		return nil, errors.New("device auth token endpoint is not configured")
	}

	cleanDeviceCode := strings.TrimSpace(deviceCode)
	cleanUserCode := strings.TrimSpace(userCode)
	if cleanDeviceCode == "" || cleanUserCode == "" {
		return nil, errors.New("device code and user code are required")
	}

	pollPayload, err := json.Marshal(map[string]string{
		"device_auth_id": cleanDeviceCode,
		"user_code":      cleanUserCode,
	})
	if err != nil {
		return nil, err
	}
	pollReq, err := http.NewRequestWithContext(ctx, http.MethodPost, deviceTokenEndpoint, strings.NewReader(string(pollPayload)))
	if err != nil {
		return nil, err
	}
	pollReq.Header.Set("Content-Type", "application/json")
	pollReq.Header.Set("Accept", "application/json")
	pollReq.Header.Set("User-Agent", "codex-router")

	pollResp, err := httpClient.Do(pollReq)
	if err != nil {
		return nil, fmt.Errorf("poll device auth token: %w", err)
	}
	defer pollResp.Body.Close()
	pollRaw, err := io.ReadAll(pollResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read device auth token response: %w", err)
	}
	switch pollResp.StatusCode {
	case http.StatusForbidden, http.StatusNotFound:
		return nil, fmt.Errorf("%w: login not completed", ErrAuthorizationPending)
	case http.StatusGone:
		return nil, fmt.Errorf("%w: device code expired", ErrDeviceCodeExpired)
	}
	if pollResp.StatusCode >= 400 {
		return nil, fmt.Errorf("device auth poll failed: %s %s", pollResp.Status, strings.TrimSpace(string(pollRaw)))
	}

	var pollResult struct {
		AuthorizationCode string `json:"authorization_code"`
		CodeVerifier      string `json:"code_verifier"`
	}
	if err := json.Unmarshal(pollRaw, &pollResult); err != nil {
		return nil, fmt.Errorf("decode device auth poll response: %w", err)
	}
	if strings.TrimSpace(pollResult.AuthorizationCode) == "" || strings.TrimSpace(pollResult.CodeVerifier) == "" {
		return nil, errors.New("device auth poll response missing authorization_code or code_verifier")
	}

	clientID := strings.TrimSpace(c.config.ClientID)
	if clientID == "" {
		clientID = "app_EMoamEEZ73f0CkXaXp7hrann"
	}
	tokenEndpoint := strings.TrimSpace(c.config.TokenURL)
	if tokenEndpoint == "" {
		return nil, errors.New("oauth token endpoint is not configured")
	}
	redirectURI := strings.TrimSpace(c.config.DeviceRedirectURL)
	if redirectURI == "" {
		redirectURI = "https://auth.openai.com/deviceauth/callback"
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", strings.TrimSpace(pollResult.AuthorizationCode))
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", clientID)
	form.Set("code_verifier", strings.TrimSpace(pollResult.CodeVerifier))

	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenReq.Header.Set("Accept", "application/json")
	tokenReq.Header.Set("User-Agent", "codex-router")

	tokenResp, err := httpClient.Do(tokenReq)
	if err != nil {
		return nil, fmt.Errorf("exchange oauth token: %w", err)
	}
	defer tokenResp.Body.Close()
	tokenRaw, err := io.ReadAll(tokenResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read oauth token response: %w", err)
	}
	if tokenResp.StatusCode >= 400 {
		return nil, fmt.Errorf("exchange oauth token failed: %s %s", tokenResp.Status, strings.TrimSpace(string(tokenRaw)))
	}

	var tokenPayload struct {
		AccessToken  string `json:"access_token"`
		IDToken      string `json:"id_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(tokenRaw, &tokenPayload); err != nil {
		return nil, fmt.Errorf("decode oauth token response: %w", err)
	}
	if strings.TrimSpace(tokenPayload.AccessToken) == "" {
		return nil, errors.New("oauth token response missing access_token")
	}

	authFile := map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]any{
			"access_token":  strings.TrimSpace(tokenPayload.AccessToken),
			"id_token":      strings.TrimSpace(tokenPayload.IDToken),
			"refresh_token": strings.TrimSpace(tokenPayload.RefreshToken),
		},
	}
	raw, err := json.Marshal(authFile)
	if err != nil {
		return nil, err
	}
	return raw, nil
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
