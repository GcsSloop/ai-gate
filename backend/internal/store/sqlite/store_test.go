package sqlite_test

import (
	"path/filepath"
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/store/sqlite"
)

func TestOpenCreatesCoreTables(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "codex-router.sqlite")

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	for _, table := range []string{
		"providers",
		"accounts",
		"account_usage_snapshots",
		"usage_events",
		"routing_policies",
		"maintenance_state",
		"conversations",
		"messages",
		"runs",
	} {
		exists, err := store.HasTable(table)
		if err != nil {
			t.Fatalf("HasTable(%q) returned error: %v", table, err)
		}
		if !exists {
			t.Fatalf("table %q was not created", table)
		}
	}
}
