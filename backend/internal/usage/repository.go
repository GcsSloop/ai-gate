package usage

import (
	"database/sql"
	"fmt"
)

type Repository interface {
	Save(snapshot Snapshot) error
	GetLatest(accountID int64) (Snapshot, error)
	ListLatest() ([]Snapshot, error)
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

func (r *SQLiteRepository) ListLatest() ([]Snapshot, error) {
	rows, err := r.db.Query(
		`SELECT s.id, s.account_id, s.balance, s.quota_remaining, s.rpm_remaining, s.tpm_remaining, s.recent_error_rate, s.avg_latency_ms, s.checked_at
		 FROM account_usage_snapshots s
		 INNER JOIN (
			SELECT account_id, MAX(checked_at) AS checked_at
			FROM account_usage_snapshots
			GROUP BY account_id
		 ) latest ON latest.account_id = s.account_id AND latest.checked_at = s.checked_at
		 ORDER BY s.account_id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query latest usage snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []Snapshot
	for rows.Next() {
		var snapshot Snapshot
		if err := rows.Scan(
			&snapshot.ID,
			&snapshot.AccountID,
			&snapshot.Balance,
			&snapshot.QuotaRemaining,
			&snapshot.RPMRemaining,
			&snapshot.TPMRemaining,
			&snapshot.RecentErrorRate,
			&snapshot.AvgLatencyMS,
			&snapshot.CheckedAt,
		); err != nil {
			return nil, fmt.Errorf("scan latest usage snapshot: %w", err)
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate latest usage snapshots: %w", err)
	}
	return snapshots, nil
}
