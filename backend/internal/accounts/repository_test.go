package accounts_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
)

func TestSQLiteRepositoryCreateAndListAccounts(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := accounts.NewSQLiteRepository(store.DB())
	cooldownUntil := time.Now().Add(3 * time.Hour).UTC().Truncate(time.Second)

	official := accounts.Account{
		ProviderType:  accounts.ProviderOpenAIOfficial,
		AccountName:   "official-primary",
		AuthMode:      accounts.AuthModeOAuth,
		Status:        accounts.StatusActive,
		CredentialRef: "cred-1",
	}
	if err := repo.Create(official); err != nil {
		t.Fatalf("Create(official) returned error: %v", err)
	}

	thirdParty := accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "mirror-east",
		AuthMode:      accounts.AuthModeAPIKey,
		Status:        accounts.StatusCooldown,
		BaseURL:       "https://example.test/v1",
		CredentialRef: "cred-2",
		CooldownUntil: &cooldownUntil,
	}
	if err := repo.Create(thirdParty); err != nil {
		t.Fatalf("Create(thirdParty) returned error: %v", err)
	}

	got, err := repo.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("List returned %d accounts, want 2", len(got))
	}

	if got[0].AuthMode != accounts.AuthModeOAuth {
		t.Fatalf("got[0].AuthMode = %q, want %q", got[0].AuthMode, accounts.AuthModeOAuth)
	}
	if got[1].Status != accounts.StatusCooldown {
		t.Fatalf("got[1].Status = %q, want %q", got[1].Status, accounts.StatusCooldown)
	}
	if got[1].CooldownUntil == nil || !got[1].CooldownUntil.Equal(cooldownUntil) {
		t.Fatalf("got[1].CooldownUntil = %v, want %v", got[1].CooldownUntil, cooldownUntil)
	}
}
