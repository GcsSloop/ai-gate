package settings_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/settings"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
)

func TestDBBackupManagerCreatesListsAndRestoresBackups(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "router.sqlite")
	store, err := sqlitestore.Open(dbPath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := settings.NewSQLiteRepository(store.DB())
	initial := settings.DefaultAppSettings()
	initial.ProxyPort = 15721
	if err := repo.SaveAppSettings(initial); err != nil {
		t.Fatalf("SaveAppSettings(initial) returned error: %v", err)
	}

	manager := settings.NewDBBackupManager(store.DB(), dbPath)
	first, err := manager.CreateBackup(10)
	if err != nil {
		t.Fatalf("CreateBackup(first) returned error: %v", err)
	}

	updated := initial
	updated.ProxyPort = 16888
	if err := repo.SaveAppSettings(updated); err != nil {
		t.Fatalf("SaveAppSettings(updated) returned error: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	second, err := manager.CreateBackup(10)
	if err != nil {
		t.Fatalf("CreateBackup(second) returned error: %v", err)
	}

	items, err := manager.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("ListBackups returned %d items, want 2", len(items))
	}
	if items[0].BackupID != second.BackupID || items[1].BackupID != first.BackupID {
		t.Fatalf("backup order = %+v, want [%s %s]", items, second.BackupID, first.BackupID)
	}

	if err := manager.RestoreBackup(first.BackupID); err != nil {
		t.Fatalf("RestoreBackup returned error: %v", err)
	}

	restored, err := repo.GetAppSettings()
	if err != nil {
		t.Fatalf("GetAppSettings(restored) returned error: %v", err)
	}
	if restored.ProxyPort != 15721 {
		t.Fatalf("restored ProxyPort = %d, want 15721", restored.ProxyPort)
	}

	preRestoreItems, err := manager.ListPreRestoreBackups()
	if err != nil {
		t.Fatalf("ListPreRestoreBackups returned error: %v", err)
	}
	if len(preRestoreItems) == 0 {
		t.Fatal("expected pre-restore backup to be created")
	}
}

func TestDBBackupManagerPrunesOldBackups(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "router.sqlite")
	store, err := sqlitestore.Open(dbPath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := settings.NewSQLiteRepository(store.DB())
	manager := settings.NewDBBackupManager(store.DB(), dbPath)

	for _, port := range []int{15721, 15722, 15723} {
		current := settings.DefaultAppSettings()
		current.ProxyPort = port
		if err := repo.SaveAppSettings(current); err != nil {
			t.Fatalf("SaveAppSettings(%d) returned error: %v", port, err)
		}
		if _, err := manager.CreateBackup(2); err != nil {
			t.Fatalf("CreateBackup(%d) returned error: %v", port, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	items, err := manager.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("ListBackups returned %d items, want 2", len(items))
	}
}
