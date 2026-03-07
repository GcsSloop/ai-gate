package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type LocalAuthFile struct {
	AuthMode     string `json:"auth_mode"`
	OpenAIAPIKey any    `json:"OPENAI_API_KEY,omitempty"`
	LastRefresh  string `json:"last_refresh,omitempty"`
	Tokens       struct {
		AccessToken  string `json:"access_token"`
		IDToken      string `json:"id_token"`
		RefreshToken string `json:"refresh_token"`
		AccountID    string `json:"account_id"`
	} `json:"tokens"`
}

func LoadLocalAuthFile(path string) (LocalAuthFile, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return LocalAuthFile{}, nil, fmt.Errorf("read local auth file: %w", err)
	}

	file, err := LoadLocalAuthFileContent(raw)
	if err != nil {
		return LocalAuthFile{}, nil, err
	}

	return file, raw, nil
}

func LoadLocalAuthFileContent(raw []byte) (LocalAuthFile, error) {
	var file LocalAuthFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return LocalAuthFile{}, fmt.Errorf("decode local auth file: %w", err)
	}
	if file.AuthMode == "" {
		return LocalAuthFile{}, fmt.Errorf("local auth file missing auth_mode")
	}
	if file.Tokens.AccessToken == "" && file.Tokens.IDToken == "" {
		return LocalAuthFile{}, fmt.Errorf("local auth file missing tokens")
	}
	if file.Tokens.AccountID == "" {
		file.Tokens.AccountID = accountIDFromJWT(file.Tokens.AccessToken)
	}

	return file, nil
}

func (f LocalAuthFile) AccessTokenExpiresAt() (time.Time, bool) {
	parts := strings.Split(f.Tokens.AccessToken, ".")
	if len(parts) < 2 {
		return time.Time{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, false
	}
	if claims.Exp <= 0 {
		return time.Time{}, false
	}
	return time.Unix(claims.Exp, 0).UTC(), true
}

func (f LocalAuthFile) ClientID() string {
	parts := strings.Split(f.Tokens.AccessToken, ".")
	if len(parts) >= 2 {
		payload, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err == nil {
			var claims struct {
				ClientID string   `json:"client_id"`
				Aud      []string `json:"aud"`
			}
			if json.Unmarshal(payload, &claims) == nil {
				if claims.ClientID != "" {
					return claims.ClientID
				}
				if len(claims.Aud) > 0 {
					return claims.Aud[0]
				}
			}
		}
	}
	parts = strings.Split(f.Tokens.IDToken, ".")
	if len(parts) >= 2 {
		payload, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err == nil {
			var claims struct {
				Aud []string `json:"aud"`
			}
			if json.Unmarshal(payload, &claims) == nil && len(claims.Aud) > 0 {
				return claims.Aud[0]
			}
		}
	}
	return ""
}

func NeedsLocalRefresh(file LocalAuthFile, now time.Time, skew time.Duration) bool {
	if file.Tokens.RefreshToken == "" {
		return false
	}
	expiresAt, ok := file.AccessTokenExpiresAt()
	if !ok {
		return false
	}
	return !expiresAt.After(now.Add(skew))
}

func RefreshLocalAuthFile(ctx context.Context, client *http.Client, tokenURL string, file LocalAuthFile) (LocalAuthFile, error) {
	if file.Tokens.RefreshToken == "" {
		return LocalAuthFile{}, fmt.Errorf("local auth file missing refresh_token")
	}
	clientID := file.ClientID()
	if clientID == "" {
		return LocalAuthFile{}, fmt.Errorf("local auth file missing client_id")
	}

	values := url.Values{}
	values.Set("grant_type", "refresh_token")
	values.Set("refresh_token", file.Tokens.RefreshToken)
	values.Set("client_id", clientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, bytes.NewBufferString(values.Encode()))
	if err != nil {
		return LocalAuthFile{}, fmt.Errorf("build refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	httpClient := client
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return LocalAuthFile{}, fmt.Errorf("refresh local auth token: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return LocalAuthFile{}, fmt.Errorf("read refresh response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return LocalAuthFile{}, fmt.Errorf("refresh local auth token: %s %s", resp.Status, strings.TrimSpace(string(raw)))
	}

	var payload struct {
		AccessToken  string `json:"access_token"`
		IDToken      string `json:"id_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return LocalAuthFile{}, fmt.Errorf("decode refresh response: %w", err)
	}
	if payload.AccessToken == "" {
		return LocalAuthFile{}, fmt.Errorf("refresh response missing access_token")
	}

	refreshed := file
	refreshed.Tokens.AccessToken = payload.AccessToken
	if payload.IDToken != "" {
		refreshed.Tokens.IDToken = payload.IDToken
	}
	if payload.RefreshToken != "" {
		refreshed.Tokens.RefreshToken = payload.RefreshToken
	}
	refreshed.LastRefresh = time.Now().UTC().Format(time.RFC3339Nano)
	if refreshed.Tokens.AccountID == "" {
		refreshed.Tokens.AccountID = accountIDFromJWT(refreshed.Tokens.AccessToken)
	}
	return refreshed, nil
}

func MarshalLocalAuthFile(file LocalAuthFile) ([]byte, error) {
	raw, err := json.Marshal(file)
	if err != nil {
		return nil, fmt.Errorf("encode local auth file: %w", err)
	}
	return raw, nil
}

func accountIDFromJWT(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		OpenAIAuth struct {
			ChatGPTAccountID string `json:"chatgpt_account_id"`
		} `json:"https://api.openai.com/auth"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	return claims.OpenAIAuth.ChatGPTAccountID
}
