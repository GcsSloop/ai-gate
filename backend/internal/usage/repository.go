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
	SaveEvent(event Event) error
	ListRecentEvents(filter EventFilter) ([]Event, error)
	SummarizeEvents(filter EventFilter) (EventSummary, error)
	TrendEventsByHour(filter EventFilter) ([]TrendPoint, error)
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

func (r *SQLiteRepository) SaveEvent(event Event) error {
	_, err := r.db.Exec(
		`INSERT INTO usage_events (
			account_id, provider_type, request_kind, model, status,
			input_tokens, output_tokens, total_tokens, estimated_cost,
			balance_before, balance_after, quota_before, quota_after,
			latency_ms, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.AccountID,
		event.ProviderType,
		event.RequestKind,
		event.Model,
		event.Status,
		event.InputTokens,
		event.OutputTokens,
		event.TotalTokens,
		event.EstimatedCost,
		nullFloat64(event.BalanceBefore),
		nullFloat64(event.BalanceAfter),
		nullFloat64(event.QuotaBefore),
		nullFloat64(event.QuotaAfter),
		event.LatencyMS,
		event.CreatedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert usage event: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) ListRecentEvents(filter EventFilter) ([]Event, error) {
	query := `SELECT id, account_id, provider_type, request_kind, model, status,
		input_tokens, output_tokens, total_tokens, estimated_cost,
		balance_before, balance_after, quota_before, quota_after,
		latency_ms, created_at
		FROM usage_events`
	where, args := eventFilterWhere(filter)
	query += where + ` ORDER BY created_at DESC, id DESC LIMIT ?`
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	args = append(args, limit)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query recent usage events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var event Event
		if err := rows.Scan(
			&event.ID,
			&event.AccountID,
			&event.ProviderType,
			&event.RequestKind,
			&event.Model,
			&event.Status,
			&event.InputTokens,
			&event.OutputTokens,
			&event.TotalTokens,
			&event.EstimatedCost,
			nullFloatDest(&event.BalanceBefore),
			nullFloatDest(&event.BalanceAfter),
			nullFloatDest(&event.QuotaBefore),
			nullFloatDest(&event.QuotaAfter),
			&event.LatencyMS,
			&event.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan usage event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate usage events: %w", err)
	}
	return events, nil
}

func (r *SQLiteRepository) SummarizeEvents(filter EventFilter) (EventSummary, error) {
	query := `SELECT
		COUNT(*),
		SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END),
		SUM(CASE WHEN status <> 'completed' THEN 1 ELSE 0 END),
		COALESCE(SUM(input_tokens), 0),
		COALESCE(SUM(output_tokens), 0),
		COALESCE(SUM(total_tokens), 0),
		COALESCE(SUM(estimated_cost), 0),
		COALESCE(SUM(CASE
			WHEN balance_before IS NOT NULL AND balance_after IS NOT NULL THEN balance_after - balance_before
			ELSE 0
		END), 0),
		COALESCE(SUM(CASE
			WHEN quota_before IS NOT NULL AND quota_after IS NOT NULL THEN quota_after - quota_before
			ELSE 0
		END), 0)
		FROM usage_events`
	where, args := eventFilterWhere(filter)

	var summary EventSummary
	err := r.db.QueryRow(query+where, args...).Scan(
		&summary.RequestCount,
		&summary.SuccessCount,
		&summary.FailureCount,
		&summary.InputTokens,
		&summary.OutputTokens,
		&summary.TotalTokens,
		&summary.EstimatedCost,
		&summary.BalanceDelta,
		&summary.QuotaDelta,
	)
	if err != nil {
		return EventSummary{}, fmt.Errorf("summarize usage events: %w", err)
	}
	return summary, nil
}

func (r *SQLiteRepository) TrendEventsByHour(filter EventFilter) ([]TrendPoint, error) {
	query := `SELECT id, account_id, provider_type, request_kind, model, status,
		input_tokens, output_tokens, total_tokens, estimated_cost,
		balance_before, balance_after, quota_before, quota_after,
		latency_ms, created_at
		FROM usage_events`
	where, args := eventFilterWhere(filter)
	query += where + ` ORDER BY created_at ASC, id ASC`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query usage trends by hour: %w", err)
	}
	defer rows.Close()

	var points []TrendPoint
	indexByBucket := make(map[time.Time]int)
	for rows.Next() {
		var event Event
		if err := rows.Scan(
			&event.ID,
			&event.AccountID,
			&event.ProviderType,
			&event.RequestKind,
			&event.Model,
			&event.Status,
			&event.InputTokens,
			&event.OutputTokens,
			&event.TotalTokens,
			&event.EstimatedCost,
			nullFloatDest(&event.BalanceBefore),
			nullFloatDest(&event.BalanceAfter),
			nullFloatDest(&event.QuotaBefore),
			nullFloatDest(&event.QuotaAfter),
			&event.LatencyMS,
			&event.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan usage trend point: %w", err)
		}
		bucket := event.CreatedAt.UTC().Truncate(time.Hour)
		idx, ok := indexByBucket[bucket]
		if !ok {
			points = append(points, TrendPoint{Bucket: bucket})
			idx = len(points) - 1
			indexByBucket[bucket] = idx
		}
		point := &points[idx]
		point.RequestCount++
		point.InputTokens += event.InputTokens
		point.OutputTokens += event.OutputTokens
		point.TotalTokens += event.TotalTokens
		point.EstimatedCost += event.EstimatedCost
		if event.BalanceBefore != nil && event.BalanceAfter != nil {
			point.BalanceDelta += *event.BalanceAfter - *event.BalanceBefore
		}
		if event.QuotaBefore != nil && event.QuotaAfter != nil {
			point.QuotaDelta += *event.QuotaAfter - *event.QuotaBefore
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate usage trends by hour: %w", err)
	}
	return points, nil
}

func eventFilterWhere(filter EventFilter) (string, []any) {
	clauses := make([]string, 0, 4)
	args := make([]any, 0, 4)
	if filter.From != nil {
		clauses = append(clauses, "created_at >= ?")
		args = append(args, filter.From.UTC())
	}
	if filter.To != nil {
		clauses = append(clauses, "created_at <= ?")
		args = append(args, filter.To.UTC())
	}
	if filter.AccountID != nil {
		clauses = append(clauses, "account_id = ?")
		args = append(args, *filter.AccountID)
	}
	if filter.Model != "" {
		clauses = append(clauses, "model = ?")
		args = append(args, filter.Model)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + joinClauses(clauses), args
}

func joinClauses(clauses []string) string {
	if len(clauses) == 0 {
		return ""
	}
	result := clauses[0]
	for _, clause := range clauses[1:] {
		result += " AND " + clause
	}
	return result
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

func nullFloat64(value *float64) sql.NullFloat64 {
	if value == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *value, Valid: true}
}

func nullFloatDest(dest **float64) any {
	return &scanFloat64{dest: dest}
}

type scanFloat64 struct {
	dest **float64
}

func (s *scanFloat64) Scan(src any) error {
	if src == nil {
		*s.dest = nil
		return nil
	}
	var value sql.NullFloat64
	if err := value.Scan(src); err != nil {
		return err
	}
	if !value.Valid {
		*s.dest = nil
		return nil
	}
	copied := value.Float64
	*s.dest = &copied
	return nil
}
