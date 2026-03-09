package api

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/auth"
)

var officialTokenRefreshURL = "https://auth.openai.com/oauth/token"
var officialRefreshes = newOfficialRefreshCoordinator()

type accountUpdater interface {
	Update(account accounts.Account) error
}

func ensureOfficialAccountSession(ctx context.Context, client *http.Client, updater accountUpdater, account *accounts.Account) error {
	if account == nil || account.AuthMode != accounts.AuthModeLocalImport {
		return nil
	}
	file, err := auth.LoadLocalAuthFileContent([]byte(account.CredentialRef))
	if err != nil {
		return err
	}
	if !auth.NeedsLocalRefresh(file, time.Now().UTC(), 5*time.Minute) {
		return nil
	}

	key := officialRefreshKey{
		accountID:    account.ID,
		accountName:  account.AccountName,
		providerType: account.ProviderType,
		refreshToken: file.Tokens.RefreshToken,
	}
	rawCredential, err := officialRefreshes.do(key, func() (string, error) {
		refreshed, refreshErr := auth.RefreshLocalAuthFile(ctx, client, officialTokenRefreshURL, file)
		if refreshErr != nil {
			return "", refreshErr
		}
		raw, marshalErr := auth.MarshalLocalAuthFile(refreshed)
		if marshalErr != nil {
			return "", marshalErr
		}
		refreshedAccount := *account
		refreshedAccount.CredentialRef = string(raw)
		if updater != nil {
			if updateErr := updater.Update(refreshedAccount); updateErr != nil {
				return "", fmt.Errorf("persist refreshed local auth: %w", updateErr)
			}
		}
		return string(raw), nil
	})
	if err != nil {
		return err
	}
	account.CredentialRef = rawCredential
	return nil
}

type officialRefreshKey struct {
	accountID    int64
	accountName  string
	providerType accounts.ProviderType
	refreshToken string
}

type officialRefreshResult struct {
	credentialRef string
	expiresAt     time.Time
}

type officialRefreshCall struct {
	done          chan struct{}
	credentialRef string
	err           error
}

type officialRefreshCoordinator struct {
	mu       sync.Mutex
	inFlight map[officialRefreshKey]*officialRefreshCall
	recent   map[officialRefreshKey]officialRefreshResult
}

func newOfficialRefreshCoordinator() *officialRefreshCoordinator {
	return &officialRefreshCoordinator{
		inFlight: make(map[officialRefreshKey]*officialRefreshCall),
		recent:   make(map[officialRefreshKey]officialRefreshResult),
	}
}

func (c *officialRefreshCoordinator) do(key officialRefreshKey, refresh func() (string, error)) (string, error) {
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

	call := &officialRefreshCall{done: make(chan struct{})}
	c.inFlight[key] = call
	c.mu.Unlock()

	credentialRef, err := refresh()

	c.mu.Lock()
	delete(c.inFlight, key)
	call.credentialRef = credentialRef
	call.err = err
	if err == nil {
		c.recent[key] = officialRefreshResult{
			credentialRef: credentialRef,
			expiresAt:     time.Now().UTC().Add(15 * time.Second),
		}
	}
	close(call.done)
	c.mu.Unlock()

	return credentialRef, err
}

func (c *officialRefreshCoordinator) pruneExpiredLocked(now time.Time) {
	for key, result := range c.recent {
		if now.After(result.expiresAt) {
			delete(c.recent, key)
		}
	}
}
