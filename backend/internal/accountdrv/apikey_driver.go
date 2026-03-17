package accountdrv

import (
	"context"
	"fmt"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
)

type APIKeyDriver struct{}

func NewAPIKeyDriver() *APIKeyDriver {
	return &APIKeyDriver{}
}

func (d *APIKeyDriver) Name() string {
	return "builtin_api_key"
}

func (d *APIKeyDriver) Supports(account accounts.Account) bool {
	return account.AuthMode == accounts.AuthModeAPIKey
}

func (d *APIKeyDriver) Resolve(_ context.Context, account accounts.Account) (ResolvedCredential, error) {
	if account.CredentialRef == "" {
		return ResolvedCredential{}, &ResolveError{
			Kind: ResolveErrorKindAuth,
			Op:   "resolve_api_key",
			Err:  fmt.Errorf("missing credential_ref"),
		}
	}
	return ResolvedCredential{
		Kind:   "api_key",
		APIKey: account.CredentialRef,
	}, nil
}
