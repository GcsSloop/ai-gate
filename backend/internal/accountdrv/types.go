package accountdrv

import (
	"context"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
)

type ResolvedCredential struct {
	Kind         string
	APIKey       string
	AccessToken  string
	RefreshToken string
	Session      map[string]string
	Headers      map[string]string
	Metadata     map[string]any
}

type AccountDriver interface {
	Name() string
	Supports(account accounts.Account) bool
	Resolve(ctx context.Context, account accounts.Account) (ResolvedCredential, error)
}
