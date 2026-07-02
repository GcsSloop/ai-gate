package sqlite_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestOpenAddsUsageDriverColumnsToAccounts(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "codex-router.sqlite")

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	for _, column := range []string{"account_driver", "usage_driver", "usage_config_json", "is_locked"} {
		var count int
		if err := store.DB().QueryRow(`SELECT COUNT(*) FROM pragma_table_info('accounts') WHERE name = ?`, column).Scan(&count); err != nil {
			t.Fatalf("QueryRow(%q) returned error: %v", column, err)
		}
		if count != 1 {
			t.Fatalf("column %q count = %d, want 1", column, count)
		}
	}
}

func TestOpenAddsSnapshotMetadataColumns(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "codex-router.sqlite")

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	for _, column := range []string{"source", "confidence", "provider_snapshot_json", "stale", "last_error"} {
		var count int
		if err := store.DB().QueryRow(`SELECT COUNT(*) FROM pragma_table_info('account_usage_snapshots') WHERE name = ?`, column).Scan(&count); err != nil {
			t.Fatalf("QueryRow(%q) returned error: %v", column, err)
		}
		if count != 1 {
			t.Fatalf("column %q count = %d, want 1", column, count)
		}
	}
}

func TestOpenNormalizesDuplicateActiveAccounts(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "codex-router.sqlite")

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	if _, err := store.DB().Exec(`INSERT INTO accounts (provider_type, account_name, source_icon, auth_mode, credential_ref, base_url, status, priority, is_active, supports_responses)
		VALUES
			('openai-compatible', 'primary', 'openai', 'api_key', 'secret-a', 'https://a.example.test/v1', 'active', 10, 1, 1),
			('openai-compatible', 'secondary', 'openai', 'api_key', 'secret-b', 'https://b.example.test/v1', 'active', 20, 1, 1)`); err != nil {
		t.Fatalf("seed duplicate active accounts returned error: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	reopened, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = reopened.Close()
	})

	var activeCount int
	if err := reopened.DB().QueryRow(`SELECT COUNT(*) FROM accounts WHERE is_active = 1`).Scan(&activeCount); err != nil {
		t.Fatalf("count active accounts returned error: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("active accounts = %d, want 1", activeCount)
	}
	var activeName string
	if err := reopened.DB().QueryRow(`SELECT account_name FROM accounts WHERE is_active = 1`).Scan(&activeName); err != nil {
		t.Fatalf("select active account returned error: %v", err)
	}
	if activeName != "secondary" {
		t.Fatalf("active account = %q, want highest-priority secondary", activeName)
	}
}

func TestOpenConfiguresSQLiteForBusyDesktopWorkload(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "codex-router.sqlite")

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	var journalMode string
	if err := store.DB().QueryRow(`PRAGMA journal_mode;`).Scan(&journalMode); err != nil {
		t.Fatalf("QueryRow(journal_mode) returned error: %v", err)
	}
	if strings.ToLower(journalMode) != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}

	var busyTimeout int
	if err := store.DB().QueryRow(`PRAGMA busy_timeout;`).Scan(&busyTimeout); err != nil {
		t.Fatalf("QueryRow(busy_timeout) returned error: %v", err)
	}
	if busyTimeout < 5000 {
		t.Fatalf("busy_timeout = %d, want >= 5000", busyTimeout)
	}

}

func TestOpenAvoidsSQLITEBUSYDuringConcurrentReadWrite(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "codex-router.sqlite")

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	if _, err := store.DB().Exec(`INSERT INTO accounts (provider_type, account_name, source_icon, auth_mode, credential_ref, base_url, status, priority, is_active, supports_responses)
		VALUES ('openai-compatible', 'busy-check', 'openai', 'api_key', 'secret', 'https://example.test/v1', 'active', 1, 1, 1)`); err != nil {
		t.Fatalf("seed account returned error: %v", err)
	}

	tx, err := store.DB().Begin()
	if err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.Exec(`UPDATE accounts SET status = 'cooldown' WHERE id = 1`); err != nil {
		t.Fatalf("UPDATE inside transaction returned error: %v", err)
	}

	errCh := make(chan error, 1)
	started := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		close(started)
		var count int
		err := store.DB().QueryRow(`SELECT COUNT(*) FROM accounts`).Scan(&count)
		errCh <- err
	}()

	<-started
	time.Sleep(150 * time.Millisecond)
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit returned error: %v", err)
	}
	wg.Wait()

	if err := <-errCh; err != nil {
		if sqliteBusy(err) {
			t.Fatalf("concurrent read returned SQLITE_BUSY: %v", err)
		}
		t.Fatalf("concurrent read returned error: %v", err)
	}
}

func sqliteBusy(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, sql.ErrConnDone) {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "database is locked") ||
		strings.Contains(strings.ToLower(err.Error()), "sqlite_busy")
}
