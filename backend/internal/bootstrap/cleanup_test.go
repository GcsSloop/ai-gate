package bootstrap

import (
	"path/filepath"
	"testing"
	"time"

	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
	"github.com/gcssloop/codex-router/backend/internal/usage"
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

func TestCleanupUsageSnapshotsDeletesOrphansCompactsAndMarksMaintenance(t *testing.T) {
	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	if _, err := store.DB().Exec(
		`INSERT INTO accounts (id, provider_type, account_name, auth_mode, credential_ref, status)
		 VALUES (1, 'openai-compatible', 'kept', 'api_key', 'sk', 'active')`,
	); err != nil {
		t.Fatalf("insert account returned error: %v", err)
	}

	usageRepo := usage.NewSQLiteRepository(store.DB())
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	for _, snapshot := range []usage.Snapshot{
		{AccountID: 999, CheckedAt: now.Add(-1 * time.Hour)},
		{AccountID: 1, CheckedAt: now.AddDate(0, 0, -10).Add(10 * time.Minute)},
		{AccountID: 1, CheckedAt: now.AddDate(0, 0, -10).Add(20 * time.Minute)},
	} {
		if err := usageRepo.Save(snapshot); err != nil {
			t.Fatalf("Save returned error: %v", err)
		}
	}

	if err := cleanupUsageSnapshots(store.DB(), usageRepo, now); err != nil {
		t.Fatalf("cleanupUsageSnapshots returned error: %v", err)
	}

	var count int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM account_usage_snapshots`).Scan(&count); err != nil {
		t.Fatalf("count snapshots returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("snapshot rows = %d, want 1", count)
	}

	var marker string
	if err := store.DB().QueryRow(`SELECT value FROM maintenance_state WHERE key = 'usage_snapshot_cleanup_last_run'`).Scan(&marker); err != nil {
		t.Fatalf("load maintenance marker returned error: %v", err)
	}
	if marker == "" {
		t.Fatalf("usage snapshot cleanup marker is empty")
	}
}
