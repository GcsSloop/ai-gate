package usage

import (
	"database/sql"
	"fmt"
	"time"
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
			account_id, balance, quota_remaining, rpm_remaining, tpm_remaining, health_score,
			recent_error_rate, avg_latency_ms, throttled_recently, last_total_tokens, last_input_tokens,
			last_output_tokens, model_context_window, primary_used_percent, secondary_used_percent,
			primary_resets_at, secondary_resets_at, checked_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snapshot.AccountID,
		snapshot.Balance,
		snapshot.QuotaRemaining,
		snapshot.RPMRemaining,
		snapshot.TPMRemaining,
		snapshot.HealthScore,
		snapshot.RecentErrorRate,
		snapshot.AvgLatencyMS,
		snapshot.ThrottledRecently,
		snapshot.LastTotalTokens,
		snapshot.LastInputTokens,
		snapshot.LastOutputTokens,
		snapshot.ModelContextWindow,
		snapshot.PrimaryUsedPercent,
		snapshot.SecondaryUsedPercent,
		nullTime(snapshot.PrimaryResetsAt),
		nullTime(snapshot.SecondaryResetsAt),
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
		`SELECT id, account_id, balance, quota_remaining, rpm_remaining, tpm_remaining, health_score,
			recent_error_rate, avg_latency_ms, throttled_recently, last_total_tokens, last_input_tokens,
			last_output_tokens, model_context_window, primary_used_percent, secondary_used_percent,
			primary_resets_at, secondary_resets_at, checked_at
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
		&snapshot.HealthScore,
		&snapshot.RecentErrorRate,
		&snapshot.AvgLatencyMS,
		&snapshot.ThrottledRecently,
		&snapshot.LastTotalTokens,
		&snapshot.LastInputTokens,
		&snapshot.LastOutputTokens,
		&snapshot.ModelContextWindow,
		&snapshot.PrimaryUsedPercent,
		&snapshot.SecondaryUsedPercent,
		nullTimeDest(&snapshot.PrimaryResetsAt),
		nullTimeDest(&snapshot.SecondaryResetsAt),
		&snapshot.CheckedAt,
	)
	if err != nil {
		return Snapshot{}, fmt.Errorf("select latest usage snapshot: %w", err)
	}

	return snapshot, nil
}

func (r *SQLiteRepository) ListLatest() ([]Snapshot, error) {
	rows, err := r.db.Query(
		`SELECT s.id, s.account_id, s.balance, s.quota_remaining, s.rpm_remaining, s.tpm_remaining, s.health_score,
			s.recent_error_rate, s.avg_latency_ms, s.throttled_recently, s.last_total_tokens, s.last_input_tokens,
			s.last_output_tokens, s.model_context_window, s.primary_used_percent, s.secondary_used_percent,
			s.primary_resets_at, s.secondary_resets_at, s.checked_at
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
			&snapshot.HealthScore,
			&snapshot.RecentErrorRate,
			&snapshot.AvgLatencyMS,
			&snapshot.ThrottledRecently,
			&snapshot.LastTotalTokens,
			&snapshot.LastInputTokens,
			&snapshot.LastOutputTokens,
			&snapshot.ModelContextWindow,
			&snapshot.PrimaryUsedPercent,
			&snapshot.SecondaryUsedPercent,
			nullTimeDest(&snapshot.PrimaryResetsAt),
			nullTimeDest(&snapshot.SecondaryResetsAt),
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

func nullTime(value *time.Time) sql.NullTime {
	if value == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: value.UTC(), Valid: true}
}

func nullTimeDest(dest **time.Time) any {
	return &scanTime{dest: dest}
}

type scanTime struct {
	dest **time.Time
}

func (s *scanTime) Scan(src any) error {
	if src == nil {
		*s.dest = nil
		return nil
	}
	var value sql.NullTime
	if err := value.Scan(src); err != nil {
		return err
	}
	if !value.Valid {
		*s.dest = nil
		return nil
	}
	copied := value.Time
	*s.dest = &copied
	return nil
}
