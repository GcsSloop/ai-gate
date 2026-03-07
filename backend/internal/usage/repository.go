package usage

import (
	"database/sql"
	"fmt"
)

type Repository interface {
	Save(snapshot Snapshot) error
	GetLatest(accountID int64) (Snapshot, error)
}

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(db *sql.DB) *SQLiteRepository {
	return &SQLiteRepository{db: db}
}

func (r *SQLiteRepository) Save(snapshot Snapshot) error {
	_, err := r.db.Exec(
		`INSERT INTO account_usage_snapshots (
			account_id, balance, quota_remaining, rpm_remaining, tpm_remaining, recent_error_rate, avg_latency_ms, checked_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		snapshot.AccountID,
		snapshot.Balance,
		snapshot.QuotaRemaining,
		snapshot.RPMRemaining,
		snapshot.TPMRemaining,
		snapshot.RecentErrorRate,
		snapshot.AvgLatencyMS,
		snapshot.CheckedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert usage snapshot: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) GetLatest(accountID int64) (Snapshot, error) {
	var snapshot Snapshot

	err := r.db.QueryRow(
		`SELECT id, account_id, balance, quota_remaining, rpm_remaining, tpm_remaining, recent_error_rate, avg_latency_ms, checked_at
		 FROM account_usage_snapshots
		 WHERE account_id = ?
		 ORDER BY checked_at DESC, id DESC
		 LIMIT 1`,
		accountID,
	).Scan(
		&snapshot.ID,
		&snapshot.AccountID,
		&snapshot.Balance,
		&snapshot.QuotaRemaining,
		&snapshot.RPMRemaining,
		&snapshot.TPMRemaining,
		&snapshot.RecentErrorRate,
		&snapshot.AvgLatencyMS,
		&snapshot.CheckedAt,
	)
	if err != nil {
		return Snapshot{}, fmt.Errorf("select latest usage snapshot: %w", err)
	}

	return snapshot, nil
}
