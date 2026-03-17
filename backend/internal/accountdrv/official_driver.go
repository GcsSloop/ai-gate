package accountdrv

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/auth"
)

const defaultOfficialTokenURL = "https://auth.openai.com/oauth/token"

type ResolveErrorKind string

const (
	ResolveErrorKindAuth ResolveErrorKind = "auth"
)

type ResolveError struct {
	Kind ResolveErrorKind
	Op   string
	Err  error
}

func (e *ResolveError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%s: %v", e.Op, e.Err)
}

func (e *ResolveError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type accountUpdater interface {
	Update(account accounts.Account) error
}

type OfficialDriver struct {
	client        *http.Client
	updater       accountUpdater
	tokenURL      string
	refreshes     *refreshCoordinator
	now           func() time.Time
	refreshWindow time.Duration
}

func NewOfficialDriver(client *http.Client, updater accountUpdater) *OfficialDriver {
	return &OfficialDriver{
		client:        client,
		updater:       updater,
		tokenURL:      defaultOfficialTokenURL,
		refreshes:     newRefreshCoordinator(),
		now:           func() time.Time { return time.Now().UTC() },
		refreshWindow: 5 * time.Minute,
	}
}

func (d *OfficialDriver) SetTokenURLForTest(tokenURL string) {
	d.tokenURL = tokenURL
}

func (d *OfficialDriver) Name() string {
	return "builtin_openai_official_session"
}

func (d *OfficialDriver) Supports(account accounts.Account) bool {
	return account.AuthMode == accounts.AuthModeLocalImport && account.ProviderType == accounts.ProviderOpenAIOfficial
}

func (d *OfficialDriver) Resolve(ctx context.Context, account accounts.Account) (ResolvedCredential, error) {
	file, err := auth.LoadLocalAuthFileContent([]byte(account.CredentialRef))
	if err != nil {
		return ResolvedCredential{}, authResolveError("load_local_auth", err)
	}

	now := d.now()
	if auth.NeedsLocalRefresh(file, now, d.refreshWindow) {
		if file.Tokens.RefreshToken == "" {
			return ResolvedCredential{}, authResolveError("refresh_local_auth", fmt.Errorf("missing refresh token"))
		}
		key := refreshKey{
			accountID:    account.ID,
			accountName:  account.AccountName,
			providerType: account.ProviderType,
			refreshToken: file.Tokens.RefreshToken,
		}
		rawCredential, refreshErr := d.refreshes.do(key, func() (string, error) {
			refreshed, err := auth.RefreshLocalAuthFile(ctx, d.client, d.tokenURL, file)
			if err != nil {
				return "", authResolveError("refresh_local_auth", err)
			}
			raw, err := auth.MarshalLocalAuthFile(refreshed)
			if err != nil {
				return "", authResolveError("marshal_local_auth", err)
			}
			refreshedAccount := account
			refreshedAccount.CredentialRef = string(raw)
			if d.updater != nil {
				if err := d.updater.Update(refreshedAccount); err != nil {
					return "", authResolveError("persist_refreshed_local_auth", err)
				}
			}
			return string(raw), nil
		})
		if refreshErr != nil {
			var resolveErr *ResolveError
			if errors.As(refreshErr, &resolveErr) {
				return ResolvedCredential{}, refreshErr
			}
			return ResolvedCredential{}, authResolveError("refresh_local_auth", refreshErr)
		}
		account.CredentialRef = rawCredential
		file, err = auth.LoadLocalAuthFileContent([]byte(rawCredential))
		if err != nil {
			return ResolvedCredential{}, authResolveError("reload_local_auth", err)
		}
	}

	accessToken := file.Tokens.AccessToken
	if accessToken == "" {
		return ResolvedCredential{}, authResolveError("resolve_access_token", fmt.Errorf("missing access_token"))
	}

	metadata := map[string]any{
		"account_id": file.Tokens.AccountID,
	}
	if file.AuthMode != "" {
		metadata["auth_mode"] = file.AuthMode
	}
	if clientID := file.ClientID(); clientID != "" {
		metadata["client_id"] = clientID
	}

	return ResolvedCredential{
		Kind:         "bearer",
		AccessToken:  accessToken,
		RefreshToken: file.Tokens.RefreshToken,
		Metadata:     metadata,
	}, nil
}

func authResolveError(op string, err error) error {
	return &ResolveError{
		Kind: ResolveErrorKindAuth,
		Op:   op,
		Err:  err,
	}
}

type refreshKey struct {
	accountID    int64
	accountName  string
	providerType accounts.ProviderType
	refreshToken string
}

type refreshResult struct {
	credentialRef string
	expiresAt     time.Time
}

type refreshCall struct {
	done          chan struct{}
	credentialRef string
	err           error
}

type refreshCoordinator struct {
	mu       sync.Mutex
	inFlight map[refreshKey]*refreshCall
	recent   map[refreshKey]refreshResult
}

func newRefreshCoordinator() *refreshCoordinator {
	return &refreshCoordinator{
		inFlight: make(map[refreshKey]*refreshCall),
		recent:   make(map[refreshKey]refreshResult),
	}
}

func (c *refreshCoordinator) do(key refreshKey, refresh func() (string, error)) (string, error) {
	now := time.Now().UTC()

	c.mu.Lock()
	c.pruneExpiredLocked(now)
	if result, ok := c.recent[key]; ok {
		c.mu.Unlock()
		return result.credentialRef, nil
	}
	if call, ok := c.inFlight[key]; ok {
		c.mu.Unlock()
		<-call.done
		return call.credentialRef, call.err
	}

	call := &refreshCall{done: make(chan struct{})}
	c.inFlight[key] = call
	c.mu.Unlock()

	credentialRef, err := refresh()

	c.mu.Lock()
	delete(c.inFlight, key)
	call.credentialRef = credentialRef
	call.err = err
	if err == nil {
		c.recent[key] = refreshResult{
			credentialRef: credentialRef,
			expiresAt:     time.Now().UTC().Add(15 * time.Second),
		}
	}
	close(call.done)
	c.mu.Unlock()

	return credentialRef, err
}

func (c *refreshCoordinator) pruneExpiredLocked(now time.Time) {
	for key, result := range c.recent {
		if now.After(result.expiresAt) {
			delete(c.recent, key)
		}
	}
}
