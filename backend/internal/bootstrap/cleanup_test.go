package bootstrap

import (
	"path/filepath"
	"testing"

	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
)

func TestCleanupLegacyAuditDataClearsRowsAndSetsMarker(t *testing.T) {
	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	if _, err := store.DB().Exec(`INSERT INTO conversations (client_id, target_provider_family, default_model, state) VALUES ('client-1', 'openai', 'gpt-5.2', 'active')`); err != nil {
		t.Fatalf("insert conversations returned error: %v", err)
	}
	if _, err := store.DB().Exec(`INSERT INTO messages (conversation_id, role, content, sequence_no) VALUES (1, 'user', 'ping', 0)`); err != nil {
		t.Fatalf("insert messages returned error: %v", err)
	}
	if _, err := store.DB().Exec(`INSERT INTO runs (conversation_id, account_id, model, status) VALUES (1, 1, 'gpt-5.2', 'completed')`); err != nil {
		t.Fatalf("insert runs returned error: %v", err)
	}

	if err := cleanupLegacyAuditData(store.DB()); err != nil {
		t.Fatalf("cleanupLegacyAuditData returned error: %v", err)
	}

	for _, table := range []string{"conversations", "messages", "runs"} {
		var count int
		if err := store.DB().QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatalf("count %s returned error: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("%s rows = %d, want 0", table, count)
		}
	}

	var marker string
	if err := store.DB().QueryRow(`SELECT value FROM maintenance_state WHERE key = 'audit_cleanup_v1'`).Scan(&marker); err != nil {
		t.Fatalf("load maintenance marker returned error: %v", err)
	}
	if marker != "done" {
		t.Fatalf("marker = %q, want done", marker)
	}
}
