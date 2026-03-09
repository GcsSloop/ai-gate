package settings

import (
	"database/sql"
	"fmt"
)

type AppSettings struct {
	LaunchAtLogin           bool   `json:"launch_at_login"`
	SilentStart             bool   `json:"silent_start"`
	CloseToTray             bool   `json:"close_to_tray"`
	ShowProxySwitchOnHome   bool   `json:"show_proxy_switch_on_home"`
	ProxyHost               string `json:"proxy_host"`
	ProxyPort               int    `json:"proxy_port"`
	AutoFailoverEnabled     bool   `json:"auto_failover_enabled"`
	AutoBackupIntervalHours int    `json:"auto_backup_interval_hours"`
	BackupRetentionCount    int    `json:"backup_retention_count"`
}

type ReadRepository interface {
	GetAppSettings() (AppSettings, error)
	ListFailoverQueue() ([]int64, error)
}

type Repository interface {
	ReadRepository
	SaveAppSettings(AppSettings) error
	SaveFailoverQueue([]int64) error
}

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(db *sql.DB) *SQLiteRepository {
	return &SQLiteRepository{db: db}
}

func DefaultAppSettings() AppSettings {
	return AppSettings{
		CloseToTray:             true,
		ShowProxySwitchOnHome:   true,
		ProxyHost:               "127.0.0.1",
		ProxyPort:               6789,
		AutoBackupIntervalHours: 24,
		BackupRetentionCount:    10,
	}
}

func (r *SQLiteRepository) GetAppSettings() (AppSettings, error) {
	row := r.db.QueryRow(
		`SELECT launch_at_login, silent_start, close_to_tray, show_proxy_switch_on_home, proxy_host, proxy_port, auto_failover_enabled, auto_backup_interval_hours, backup_retention_count
		 FROM app_settings WHERE id = 1`,
	)

	var launchAtLogin int
	var silentStart int
	var closeToTray int
	var showProxySwitchOnHome int
	var proxyHost string
	var proxyPort int
	var autoFailoverEnabled int
	var autoBackupIntervalHours int
	var backupRetentionCount int

	if err := row.Scan(
		&launchAtLogin,
		&silentStart,
		&closeToTray,
		&showProxySwitchOnHome,
		&proxyHost,
		&proxyPort,
		&autoFailoverEnabled,
		&autoBackupIntervalHours,
		&backupRetentionCount,
	); err != nil {
		if err == sql.ErrNoRows {
			return DefaultAppSettings(), nil
		}
		return AppSettings{}, fmt.Errorf("select app settings: %w", err)
	}

	return sanitize(AppSettings{
		LaunchAtLogin:           launchAtLogin == 1,
		SilentStart:             silentStart == 1,
		CloseToTray:             closeToTray == 1,
		ShowProxySwitchOnHome:   showProxySwitchOnHome == 1,
		ProxyHost:               proxyHost,
		ProxyPort:               proxyPort,
		AutoFailoverEnabled:     autoFailoverEnabled == 1,
		AutoBackupIntervalHours: autoBackupIntervalHours,
		BackupRetentionCount:    backupRetentionCount,
	}), nil
}

func (r *SQLiteRepository) SaveAppSettings(value AppSettings) error {
	value = sanitize(value)
	_, err := r.db.Exec(
		`INSERT INTO app_settings (
			id, launch_at_login, silent_start, close_to_tray, show_proxy_switch_on_home, proxy_host, proxy_port, auto_failover_enabled, auto_backup_interval_hours, backup_retention_count, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			launch_at_login = excluded.launch_at_login,
			silent_start = excluded.silent_start,
			close_to_tray = excluded.close_to_tray,
			show_proxy_switch_on_home = excluded.show_proxy_switch_on_home,
			proxy_host = excluded.proxy_host,
			proxy_port = excluded.proxy_port,
			auto_failover_enabled = excluded.auto_failover_enabled,
			auto_backup_interval_hours = excluded.auto_backup_interval_hours,
			backup_retention_count = excluded.backup_retention_count,
			updated_at = CURRENT_TIMESTAMP`,
		1,
		boolToInt(value.LaunchAtLogin),
		boolToInt(value.SilentStart),
		boolToInt(value.CloseToTray),
		boolToInt(value.ShowProxySwitchOnHome),
		value.ProxyHost,
		value.ProxyPort,
		boolToInt(value.AutoFailoverEnabled),
		value.AutoBackupIntervalHours,
		value.BackupRetentionCount,
	)
	if err != nil {
		return fmt.Errorf("upsert app settings: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) ListFailoverQueue() ([]int64, error) {
	rows, err := r.db.Query(`SELECT account_id FROM failover_queue_items ORDER BY position ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("query failover queue: %w", err)
	}
	defer rows.Close()

	var accountIDs []int64
	for rows.Next() {
		var accountID int64
		if err := rows.Scan(&accountID); err != nil {
			return nil, fmt.Errorf("scan failover queue: %w", err)
		}
		accountIDs = append(accountIDs, accountID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate failover queue: %w", err)
	}
	return accountIDs, nil
}

func (r *SQLiteRepository) SaveFailoverQueue(accountIDs []int64) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin save failover queue: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.Exec(`DELETE FROM failover_queue_items`); err != nil {
		return fmt.Errorf("clear failover queue: %w", err)
	}
	for index, accountID := range accountIDs {
		if _, err = tx.Exec(
			`INSERT INTO failover_queue_items (account_id, position) VALUES (?, ?)`,
			accountID,
			index,
		); err != nil {
			return fmt.Errorf("insert failover queue item: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit failover queue: %w", err)
	}
	return nil
}

func sanitize(value AppSettings) AppSettings {
	defaults := DefaultAppSettings()
	if value.ProxyHost == "" {
		value.ProxyHost = defaults.ProxyHost
	}
	if value.ProxyPort <= 0 {
		value.ProxyPort = defaults.ProxyPort
	}
	if value.AutoBackupIntervalHours <= 0 {
		value.AutoBackupIntervalHours = defaults.AutoBackupIntervalHours
	}
	if value.BackupRetentionCount <= 0 {
		value.BackupRetentionCount = defaults.BackupRetentionCount
	}
	return value
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
