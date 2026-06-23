package serverusers_test

import (
	"database/sql"
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

func TestRepositoryDeletesUser(t *testing.T) {
	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := serverusers.NewSQLiteRepository(store.DB())
	created, err := repo.Create("delete-me")
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if err := repo.Delete(created.User.ID); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	users, err := repo.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(users) != 0 {
		t.Fatalf("users = %+v, want deleted user omitted", users)
	}
	if _, err := repo.Authenticate(created.Token); err == nil {
		t.Fatal("Authenticate returned nil error for deleted user")
	}
	if err := repo.Delete(created.User.ID); err != sql.ErrNoRows {
		t.Fatalf("Delete missing user error = %v, want sql.ErrNoRows", err)
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

func TestRepositoryThrottlesLastUsedAtWrites(t *testing.T) {
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
	first, err := repo.Authenticate(created.Token)
	if err != nil {
		t.Fatalf("first Authenticate returned error: %v", err)
	}
	if first.LastUsedAt == nil {
		t.Fatal("first LastUsedAt = nil, want timestamp")
	}
	second, err := repo.Authenticate(created.Token)
	if err != nil {
		t.Fatalf("second Authenticate returned error: %v", err)
	}
	if second.LastUsedAt == nil || !second.LastUsedAt.Equal(*first.LastUsedAt) {
		t.Fatalf("second LastUsedAt = %v, want unchanged %v", second.LastUsedAt, first.LastUsedAt)
	}
}

func TestRepositoryPersistsRoutePreference(t *testing.T) {
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
	accountID := int64(42)
	updated, err := repo.UpdateRoute(created.User.ID, &accountID, true)
	if err != nil {
		t.Fatalf("UpdateRoute returned error: %v", err)
	}
	if updated.PreferredAccountID == nil || *updated.PreferredAccountID != accountID || !updated.RouteLocked {
		t.Fatalf("updated route = account:%v locked:%v, want account 42 locked", updated.PreferredAccountID, updated.RouteLocked)
	}

	authenticated, err := repo.Authenticate(created.Token)
	if err != nil {
		t.Fatalf("Authenticate returned error: %v", err)
	}
	if authenticated.PreferredAccountID == nil || *authenticated.PreferredAccountID != accountID || !authenticated.RouteLocked {
		t.Fatalf("authenticated route = account:%v locked:%v, want account 42 locked", authenticated.PreferredAccountID, authenticated.RouteLocked)
	}

	users, err := repo.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if users[0].PreferredAccountID == nil || *users[0].PreferredAccountID != accountID || !users[0].RouteLocked {
		t.Fatalf("listed route = account:%v locked:%v, want account 42 locked", users[0].PreferredAccountID, users[0].RouteLocked)
	}

	cleared, err := repo.UpdateRoute(created.User.ID, nil, false)
	if err != nil {
		t.Fatalf("UpdateRoute clear returned error: %v", err)
	}
	if cleared.PreferredAccountID != nil || cleared.RouteLocked {
		t.Fatalf("cleared route = account:%v locked:%v, want automatic route", cleared.PreferredAccountID, cleared.RouteLocked)
	}
}
