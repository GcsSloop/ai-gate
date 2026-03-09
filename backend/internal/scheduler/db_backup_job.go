package scheduler

import (
	"context"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/settings"
)

type BackupSettingsRepository interface {
	GetAppSettings() (settings.AppSettings, error)
}

type BackupManager interface {
	ListBackups() ([]settings.DBBackupItem, error)
	CreateBackup(retention int) (settings.DBBackupItem, error)
}

type DBBackupJob struct {
	repo    BackupSettingsRepository
	manager BackupManager
}

func NewDBBackupJob(repo BackupSettingsRepository, manager BackupManager) *DBBackupJob {
	return &DBBackupJob{
		repo:    repo,
		manager: manager,
	}
}

func (j *DBBackupJob) Run(_ context.Context, now time.Time) error {
	appSettings, err := j.repo.GetAppSettings()
	if err != nil {
		return err
	}

	interval := time.Duration(appSettings.AutoBackupIntervalHours) * time.Hour
	if interval <= 0 {
		return nil
	}

	items, err := j.manager.ListBackups()
	if err != nil {
		return err
	}

	if len(items) > 0 {
		if latest := parseBackupTimestamp(items[0]); !latest.IsZero() && now.UTC().Before(latest.Add(interval)) {
			return nil
		}
	}

	_, err = j.manager.CreateBackup(appSettings.BackupRetentionCount)
	return err
}

func parseBackupTimestamp(item settings.DBBackupItem) time.Time {
	if item.CreatedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, item.CreatedAt); err == nil {
			return parsed.UTC()
		}
	}
	if item.BackupID != "" {
		if parsed, err := time.Parse("20060102-150405.000", item.BackupID); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}
