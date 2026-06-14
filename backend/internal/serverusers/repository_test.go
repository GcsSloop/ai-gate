package serverusers_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/serverusers"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

func TestRepositoryCreatesAuthenticatesAndListsUsage(t *testing.T) {
	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := serverusers.NewSQLiteRepository(store.DB())
	created, err := repo.Create("alice")
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if created.Token == "" || created.User.TokenHash != "" {
		t.Fatalf("created = %+v, want one-time token without token hash", created)
	}
	if created.User.Username != "alice" || created.User.Role != serverusers.RoleUser {
		t.Fatalf("created user = %+v, want username alice and role user", created.User)
	}

	authenticated, err := repo.Authenticate(created.Token)
	if err != nil {
		t.Fatalf("Authenticate returned error: %v", err)
	}
	if authenticated.ID != created.User.ID || authenticated.LastUsedAt == nil {
		t.Fatalf("authenticated = %+v, want same user with last_used_at", authenticated)
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	userID := created.User.ID
	if err := usageRepo.SaveEvent(usage.Event{
		AccountID:    9,
		ServerUserID: &userID,
		ProviderType: "openai_compatible",
		RequestKind:  "responses",
		Model:        "gpt-test",
		Status:       "completed",
		TotalTokens:  42,
		CreatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveEvent returned error: %v", err)
	}

	users, err := repo.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(users) != 1 || users[0].RequestCount != 1 || users[0].TotalTokens != 42 {
		t.Fatalf("users = %+v, want usage totals", users)
	}
}

func TestRepositoryDisablesAndRotatesToken(t *testing.T) {
	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := serverusers.NewSQLiteRepository(store.DB())
	created, err := repo.Create("bob")
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if err := repo.Disable(created.User.ID); err != nil {
		t.Fatalf("Disable returned error: %v", err)
	}
	if _, err := repo.Authenticate(created.Token); err == nil {
		t.Fatal("Authenticate returned nil error for disabled user")
	}

	rotated, err := repo.RotateToken(created.User.ID)
	if err != nil {
		t.Fatalf("RotateToken returned error: %v", err)
	}
	if rotated.Token == "" || rotated.Token == created.Token {
		t.Fatalf("rotated token = %q, original = %q", rotated.Token, created.Token)
	}
	if _, err := repo.Authenticate(rotated.Token); err != nil {
		t.Fatalf("Authenticate rotated token returned error: %v", err)
	}
}

func TestRepositoryAuthenticatesLoginByUsernameAndToken(t *testing.T) {
	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := serverusers.NewSQLiteRepository(store.DB())
	created, err := repo.Create("alice")
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	authenticated, err := repo.AuthenticateLogin("alice", created.Token)
	if err != nil {
		t.Fatalf("AuthenticateLogin returned error: %v", err)
	}
	if authenticated.ID != created.User.ID || authenticated.Role != serverusers.RoleUser {
		t.Fatalf("authenticated = %+v, want alice user role", authenticated)
	}
	if _, err := repo.AuthenticateLogin("bob", created.Token); err == nil {
		t.Fatal("AuthenticateLogin returned nil error for wrong username")
	}
	if _, err := repo.AuthenticateLogin("alice", created.Token+"bad"); err == nil {
		t.Fatal("AuthenticateLogin returned nil error for wrong token")
	}
}

func TestRepositoryAssignsAccountPoolWithoutGlobalAccountMutation(t *testing.T) {
	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	createAccount := func(name string, priority int, active bool, locked bool) int64 {
		t.Helper()
		if err := accountRepo.Create(accounts.Account{
			ProviderType:  accounts.ProviderOpenAICompatible,
			AccountName:   name,
			AuthMode:      accounts.AuthModeAPIKey,
			CredentialRef: "sk-" + name,
			BaseURL:       "https://example.invalid/v1",
			Status:        accounts.StatusActive,
			Priority:      priority,
			IsActive:      active,
			IsLocked:      locked,
		}); err != nil {
			t.Fatalf("Create(account %s) returned error: %v", name, err)
		}
		list, err := accountRepo.List()
		if err != nil {
			t.Fatalf("List accounts returned error: %v", err)
		}
		for _, account := range list {
			if account.AccountName == name {
				return account.ID
			}
		}
		t.Fatalf("created account %s not found", name)
		return 0
	}
	firstID := createAccount("first", 100, true, false)
	secondID := createAccount("second", 10, false, false)

	repo := serverusers.NewSQLiteRepository(store.DB())
	created, err := repo.Create("alice")
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	assigned, err := repo.ListAssignedAccounts(created.User.ID)
	if err != nil {
		t.Fatalf("ListAssignedAccounts returned error: %v", err)
	}
	if len(assigned) != 0 {
		t.Fatalf("new user assigned accounts = %+v, want none", assigned)
	}

	if err := repo.SetAccountAssignments(created.User.ID, []int64{secondID, firstID}); err != nil {
		t.Fatalf("SetAccountAssignments returned error: %v", err)
	}
	assigned, err = repo.ListAssignedAccounts(created.User.ID)
	if err != nil {
		t.Fatalf("ListAssignedAccounts returned error: %v", err)
	}
	if len(assigned) != 2 {
		t.Fatalf("assigned accounts = %+v, want two", assigned)
	}
	if assigned[0].AccountID != secondID || assigned[0].Position != 0 || assigned[1].AccountID != firstID || assigned[1].Position != 1 {
		t.Fatalf("assigned accounts = %+v, want saved order second, first", assigned)
	}
	if assigned[0].CredentialRef != "" || assigned[1].CredentialRef != "" {
		t.Fatalf("assigned accounts leaked credentials: %+v", assigned)
	}

	if err := repo.UpdateAccountState(created.User.ID, firstID, serverusers.AccountStateUpdate{
		Position: 0,
		IsActive: true,
		IsLocked: true,
	}); err != nil {
		t.Fatalf("UpdateAccountState returned error: %v", err)
	}
	assigned, err = repo.ListAssignedAccounts(created.User.ID)
	if err != nil {
		t.Fatalf("ListAssignedAccounts returned error: %v", err)
	}
	if assigned[0].AccountID != firstID || !assigned[0].IsActive || !assigned[0].IsLocked {
		t.Fatalf("assigned accounts after state update = %+v, want first active and locked first", assigned)
	}

	globalFirst, err := accountRepo.GetByID(firstID)
	if err != nil {
		t.Fatalf("GetByID returned error: %v", err)
	}
	if globalFirst.Priority != 100 || !globalFirst.IsActive || globalFirst.IsLocked {
		t.Fatalf("global first account mutated = %+v, want original priority/active/locked", globalFirst)
	}
}
