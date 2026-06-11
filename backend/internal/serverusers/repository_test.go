package serverusers_test

import (
	"path/filepath"
	"testing"
	"time"

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
