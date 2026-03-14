package settings

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type DBBackupItem struct {
	BackupID  string `json:"backup_id"`
	CreatedAt string `json:"created_at"`
	SizeBytes int64  `json:"size_bytes"`
}

type DBBackupManager struct {
	db           *sql.DB
	databasePath string
}

func NewDBBackupManager(db *sql.DB, databasePath string) *DBBackupManager {
	return &DBBackupManager{db: db, databasePath: databasePath}
}

func (m *DBBackupManager) CreateBackup(retention int) (DBBackupItem, error) {
	item, err := m.createBackupInRoot(m.backupRoot(), true)
	if err != nil {
		return DBBackupItem{}, err
	}
	if retention > 0 {
		if err := m.pruneBackups(retention); err != nil {
			return DBBackupItem{}, err
		}
	}
	return item, nil
}

func (m *DBBackupManager) ListBackups() ([]DBBackupItem, error) {
	return listBackupItems(m.backupRoot())
}

func (m *DBBackupManager) ListPreRestoreBackups() ([]DBBackupItem, error) {
	return listBackupItems(m.preRestoreRoot())
}

func (m *DBBackupManager) RestoreBackup(backupID string) error {
	backupPath := filepath.Join(m.backupRoot(), backupID+".sqlite")
	if _, err := os.Stat(backupPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("backup_id not found")
		}
		return fmt.Errorf("stat backup %s: %w", backupID, err)
	}
	if _, err := m.createBackupInRoot(m.preRestoreRoot(), false); err != nil {
		return err
	}
	return replaceOwnedTablesFromDatabase(m.db, backupPath)
}

func (m *DBBackupManager) DeleteBackup(backupID string) error {
	backupPath := filepath.Join(m.backupRoot(), backupID+".sqlite")
	if _, err := os.Stat(backupPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("backup_id not found")
		}
		return fmt.Errorf("stat backup %s: %w", backupID, err)
	}
	if err := os.Remove(backupPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("backup_id not found")
		}
		return fmt.Errorf("remove backup %s: %w", backupID, err)
	}
	return nil
}

func (m *DBBackupManager) pruneBackups(retention int) error {
	items, err := m.ListBackups()
	if err != nil {
		return err
	}
	if len(items) <= retention {
		return nil
	}
	for _, item := range items[retention:] {
		if err := os.Remove(filepath.Join(m.backupRoot(), item.BackupID+".sqlite")); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove old backup %s: %w", item.BackupID, err)
		}
	}
	return nil
}

func (m *DBBackupManager) createBackupInRoot(root string, forceCheckpoint bool) (DBBackupItem, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return DBBackupItem{}, fmt.Errorf("create backup root: %w", err)
	}

	backupID := time.Now().Format("20060102-150405.000")
	targetPath := filepath.Join(root, backupID+".sqlite")
	if forceCheckpoint {
		_, _ = m.db.Exec(`PRAGMA wal_checkpoint(FULL)`)
	}
	if _, err := m.db.Exec(fmt.Sprintf("VACUUM INTO '%s'", escapeSQLiteString(targetPath))); err != nil {
		return DBBackupItem{}, fmt.Errorf("vacuum into backup %s: %w", targetPath, err)
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		return DBBackupItem{}, fmt.Errorf("stat backup %s: %w", targetPath, err)
	}

	return DBBackupItem{
		BackupID:  backupID,
		CreatedAt: info.ModTime().UTC().Format(time.RFC3339),
		SizeBytes: info.Size(),
	}, nil
}

func (m *DBBackupManager) backupRoot() string {
	return filepath.Join(filepath.Dir(m.databasePath), "backups", "db")
}

func (m *DBBackupManager) preRestoreRoot() string {
	return filepath.Join(filepath.Dir(m.databasePath), "backups", "db-pre-restore")
}

func listBackupItems(root string) ([]DBBackupItem, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []DBBackupItem{}, nil
		}
		return nil, fmt.Errorf("read backup root %s: %w", root, err)
	}

	items := make([]DBBackupItem, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sqlite") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("read backup info %s: %w", entry.Name(), err)
		}
		backupID := strings.TrimSuffix(entry.Name(), ".sqlite")
		items = append(items, DBBackupItem{
			BackupID:  backupID,
			CreatedAt: info.ModTime().UTC().Format(time.RFC3339),
			SizeBytes: info.Size(),
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].BackupID > items[j].BackupID
	})
	return items, nil
}
