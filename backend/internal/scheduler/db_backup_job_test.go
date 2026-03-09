package scheduler_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/scheduler"
	"github.com/gcssloop/codex-router/backend/internal/settings"
)

func TestDBBackupJobCreatesBackupWhenNoneExist(t *testing.T) {
	t.Parallel()

	repo := &stubSettingsRepo{
		settings: settings.AppSettings{
			AutoBackupIntervalHours: 24,
			BackupRetentionCount:    6,
		},
	}
	manager := &stubDBBackupManager{}
	job := scheduler.NewDBBackupJob(repo, manager)

	if err := job.Run(context.Background(), time.Date(2026, 3, 9, 10, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if manager.createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1", manager.createCalls)
	}
	if manager.lastRetention != 6 {
		t.Fatalf("lastRetention = %d, want 6", manager.lastRetention)
	}
}

func TestDBBackupJobSkipsBackupWhenLatestBackupIsRecent(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 9, 10, 0, 0, 0, time.UTC)
	repo := &stubSettingsRepo{
		settings: settings.AppSettings{
			AutoBackupIntervalHours: 24,
			BackupRetentionCount:    8,
		},
	}
	manager := &stubDBBackupManager{
		items: []settings.DBBackupItem{
			{BackupID: "20260309-020000.000", CreatedAt: now.Add(-8 * time.Hour).Format(time.RFC3339)},
		},
	}
	job := scheduler.NewDBBackupJob(repo, manager)

	if err := job.Run(context.Background(), now); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if manager.createCalls != 0 {
		t.Fatalf("createCalls = %d, want 0", manager.createCalls)
	}
}

func TestDBBackupJobCreatesBackupWhenLatestBackupExpired(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 9, 10, 0, 0, 0, time.UTC)
	repo := &stubSettingsRepo{
		settings: settings.AppSettings{
			AutoBackupIntervalHours: 12,
			BackupRetentionCount:    3,
		},
	}
	manager := &stubDBBackupManager{
		items: []settings.DBBackupItem{
			{BackupID: "20260308-120000.000", CreatedAt: now.Add(-25 * time.Hour).Format(time.RFC3339)},
		},
	}
	job := scheduler.NewDBBackupJob(repo, manager)

	if err := job.Run(context.Background(), now); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if manager.createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1", manager.createCalls)
	}
	if manager.lastRetention != 3 {
		t.Fatalf("lastRetention = %d, want 3", manager.lastRetention)
	}
}

func TestDBBackupJobReturnsSettingsError(t *testing.T) {
	t.Parallel()

	repo := &stubSettingsRepo{err: errors.New("boom")}
	manager := &stubDBBackupManager{}
	job := scheduler.NewDBBackupJob(repo, manager)

	err := job.Run(context.Background(), time.Date(2026, 3, 9, 10, 0, 0, 0, time.UTC))
	if err == nil || err.Error() != "boom" {
		t.Fatalf("Run error = %v, want boom", err)
	}
}

type stubSettingsRepo struct {
	settings settings.AppSettings
	err      error
}

func (r *stubSettingsRepo) GetAppSettings() (settings.AppSettings, error) {
	if r.err != nil {
		return settings.AppSettings{}, r.err
	}
	return r.settings, nil
}

type stubDBBackupManager struct {
	items         []settings.DBBackupItem
	createCalls   int
	lastRetention int
	err           error
}

func (m *stubDBBackupManager) ListBackups() ([]settings.DBBackupItem, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.items, nil
}

func (m *stubDBBackupManager) CreateBackup(retention int) (settings.DBBackupItem, error) {
	if m.err != nil {
		return settings.DBBackupItem{}, m.err
	}
	m.createCalls++
	m.lastRetention = retention
	return settings.DBBackupItem{BackupID: "20260309-100000.000"}, nil
}
