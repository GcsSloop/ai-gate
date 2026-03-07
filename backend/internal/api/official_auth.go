package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/auth"
)

var officialTokenRefreshURL = "https://auth.openai.com/oauth/token"

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
	refreshed, err := auth.RefreshLocalAuthFile(ctx, client, officialTokenRefreshURL, file)
	if err != nil {
		return err
	}
	raw, err := auth.MarshalLocalAuthFile(refreshed)
	if err != nil {
		return err
	}
	account.CredentialRef = string(raw)
	if updater != nil {
		if err := updater.Update(*account); err != nil {
			return fmt.Errorf("persist refreshed local auth: %w", err)
		}
	}
	return nil
}
