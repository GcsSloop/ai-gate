package usage

import (
	"database/sql"
	"fmt"
	"sort"
	"time"
)

type Repository interface {
	Save(snapshot Snapshot) error
	GetLatest(accountID int64) (Snapshot, error)
	ListLatest() ([]Snapshot, error)
	DeleteSnapshotsForAccount(accountID int64) (int64, error)
	CleanupSnapshots(now time.Time) (SnapshotCleanupResult, error)
	SaveEvent(event Event) error
	CompactEvents(now time.Time) error
	ListRecentEvents(filter EventFilter) ([]Event, error)
	SummarizeEvents(filter EventFilter) (EventSummary, error)
	TrendEventsByHour(filter EventFilter) ([]TrendPoint, error)
	ModelDistribution(filter EventFilter) ([]ModelDistributionPoint, error)
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
			account_id, source, confidence, provider_snapshot_json, stale, last_error,
			balance, quota_remaining, rpm_remaining, tpm_remaining, health_score,
			recent_error_rate, avg_latency_ms, throttled_recently, last_total_tokens, last_input_tokens,
			last_output_tokens, model_context_window, primary_used_percent, secondary_used_percent,
			primary_resets_at, secondary_resets_at, checked_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snapshot.AccountID,
		snapshot.Source,
		snapshot.Confidence,
		snapshot.ProviderSnapshotJSON,
		snapshot.Stale,
		snapshot.LastError,
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
		`SELECT id, account_id, source, confidence, provider_snapshot_json, stale, last_error,
			balance, quota_remaining, rpm_remaining, tpm_remaining, health_score,
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
		&snapshot.Source,
		&snapshot.Confidence,
		&snapshot.ProviderSnapshotJSON,
		&snapshot.Stale,
		&snapshot.LastError,
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
		`SELECT s.id, s.account_id, s.source, s.confidence, s.provider_snapshot_json, s.stale, s.last_error,
			s.balance, s.quota_remaining, s.rpm_remaining, s.tpm_remaining, s.health_score,
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
			&snapshot.Source,
			&snapshot.Confidence,
			&snapshot.ProviderSnapshotJSON,
			&snapshot.Stale,
			&snapshot.LastError,
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

func (r *SQLiteRepository) DeleteSnapshotsForAccount(accountID int64) (int64, error) {
	result, err := r.db.Exec(`DELETE FROM account_usage_snapshots WHERE account_id = ?`, accountID)
	if err != nil {
		return 0, fmt.Errorf("delete usage snapshots for account: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read deleted usage snapshot rows: %w", err)
	}
	return deleted, nil
}

func (r *SQLiteRepository) CleanupSnapshots(now time.Time) (SnapshotCleanupResult, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return SnapshotCleanupResult{}, fmt.Errorf("begin usage snapshot cleanup: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var result SnapshotCleanupResult
	orphanResult, err := tx.Exec(
		`DELETE FROM account_usage_snapshots
		 WHERE NOT EXISTS (
			SELECT 1 FROM accounts WHERE accounts.id = account_usage_snapshots.account_id
		 )`,
	)
	if err != nil {
		return SnapshotCleanupResult{}, fmt.Errorf("delete orphan usage snapshots: %w", err)
	}
	result.OrphanDeleted, err = orphanResult.RowsAffected()
	if err != nil {
		return SnapshotCleanupResult{}, fmt.Errorf("read orphan usage snapshot rows: %w", err)
	}

	recentCutoff := now.UTC().AddDate(0, 0, -7)
	midCutoff := now.UTC().AddDate(0, 0, -30)
	compactResult, err := tx.Exec(
		`DELETE FROM account_usage_snapshots
		 WHERE id IN (
			SELECT id FROM (
				SELECT id,
					CASE
						WHEN checked_at >= ? THEN 1
						WHEN checked_at >= ? THEN ROW_NUMBER() OVER (
							PARTITION BY account_id, substr(checked_at, 1, 13)
							ORDER BY checked_at DESC, id DESC
						)
						ELSE ROW_NUMBER() OVER (
							PARTITION BY account_id, substr(checked_at, 1, 10)
							ORDER BY checked_at DESC, id DESC
						)
					END AS keep_rank
				FROM account_usage_snapshots
				WHERE EXISTS (
					SELECT 1 FROM accounts WHERE accounts.id = account_usage_snapshots.account_id
				)
			)
			WHERE keep_rank > 1
		 )`,
		recentCutoff,
		midCutoff,
	)
	if err != nil {
		return SnapshotCleanupResult{}, fmt.Errorf("compact usage snapshots: %w", err)
	}
	result.CompactedDeleted, err = compactResult.RowsAffected()
	if err != nil {
		return SnapshotCleanupResult{}, fmt.Errorf("read compacted usage snapshot rows: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return SnapshotCleanupResult{}, fmt.Errorf("commit usage snapshot cleanup: %w", err)
	}
	return result, nil
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

func (r *SQLiteRepository) CompactEvents(now time.Time) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin compact usage events: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	hourlyCutoff := now.UTC().AddDate(0, 0, -7).Truncate(time.Hour)
	dailyCutoff := addLocalDays(localDayStartUTC(now.UTC(), time.Local), time.Local, -30)

	if err = compactRawEventsToHourly(tx, dailyCutoff, hourlyCutoff); err != nil {
		return err
	}
	if err = compactHourlyRollupsToDaily(tx, dailyCutoff); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit compact usage events: %w", err)
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
	var summary EventSummary
	rows, err := r.loadAggregateRows(filter)
	if err != nil {
		return EventSummary{}, err
	}
	for _, row := range rows {
		summary.RequestCount += row.RequestCount
		summary.SuccessCount += row.SuccessCount
		summary.FailureCount += row.FailureCount
		summary.InputTokens += row.InputTokens
		summary.OutputTokens += row.OutputTokens
		summary.TotalTokens += row.TotalTokens
		summary.BalanceDelta += row.BalanceDelta
		summary.QuotaDelta += row.QuotaDelta
		summary.EstimatedCost += calculateCost(filter, row)
	}
	return summary, nil
}

func (r *SQLiteRepository) TrendEventsByHour(filter EventFilter) ([]TrendPoint, error) {
	rows, err := r.loadAggregateRows(filter)
	if err != nil {
		return nil, err
	}
	bucketSize := filter.BucketSize
	if bucketSize <= 0 {
		bucketSize = time.Hour
	}

	location := trendBucketLocation(filter)
	points, indexByBucket := initializeTrendBuckets(filter, bucketSize)
	for _, row := range rows {
		bucket := row.BucketStart.UTC()
		if bucketSize == time.Hour {
			bucket = bucket.Truncate(time.Hour)
		} else {
			bucket = localDayStartUTC(bucket, location)
		}
		idx, ok := indexByBucket[bucket]
		if !ok {
			points = append(points, TrendPoint{Bucket: bucket})
			idx = len(points) - 1
			indexByBucket[bucket] = idx
		}
		point := &points[idx]
		point.RequestCount += row.RequestCount
		point.InputTokens += row.InputTokens
		point.OutputTokens += row.OutputTokens
		point.TotalTokens += row.TotalTokens
		point.BalanceDelta += row.BalanceDelta
		point.QuotaDelta += row.QuotaDelta
		point.EstimatedCost += calculateCost(filter, row)
	}
	return points, nil
}

func (r *SQLiteRepository) ModelDistribution(filter EventFilter) ([]ModelDistributionPoint, error) {
	rows, err := r.loadAggregateRows(filter)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int64)
	for _, row := range rows {
		counts[row.Model] += row.RequestCount
	}
	points := make([]ModelDistributionPoint, 0, len(counts))
	for model, requestCount := range counts {
		points = append(points, ModelDistributionPoint{Model: model, RequestCount: requestCount})
	}
	sortModelDistribution(points)
	return points, nil
}

func initializeTrendBuckets(filter EventFilter, bucketSize time.Duration) ([]TrendPoint, map[time.Time]int) {
	if !filter.IncludeZeroes || filter.From == nil || filter.To == nil {
		return []TrendPoint{}, map[time.Time]int{}
	}
	points := make([]TrendPoint, 0)
	indexByBucket := make(map[time.Time]int)
	if bucketSize == time.Hour {
		for cursor := filter.From.UTC(); cursor.Before(filter.To.UTC()); cursor = cursor.Add(bucketSize) {
			bucket := cursor.Truncate(time.Hour)
			if _, exists := indexByBucket[bucket]; exists {
				continue
			}
			points = append(points, TrendPoint{Bucket: bucket})
			indexByBucket[bucket] = len(points) - 1
		}
		return points, indexByBucket
	}

	location := trendBucketLocation(filter)
	if filter.BucketLocation == nil {
		for cursor := filter.From.UTC(); cursor.Before(filter.To.UTC()); cursor = cursor.Add(bucketSize) {
			bucket := localDayStartUTC(cursor, location)
			if _, exists := indexByBucket[bucket]; exists {
				continue
			}
			points = append(points, TrendPoint{Bucket: bucket})
			indexByBucket[bucket] = len(points) - 1
		}
		return points, indexByBucket
	}
	for cursor := localDayStartUTC(*filter.From, location); cursor.Before(filter.To.UTC()); cursor = nextLocalDayStartUTC(cursor, location) {
		bucket := cursor
		if _, exists := indexByBucket[bucket]; exists {
			continue
		}
		points = append(points, TrendPoint{Bucket: bucket})
		indexByBucket[bucket] = len(points) - 1
	}
	return points, indexByBucket
}

func trendBucketLocation(filter EventFilter) *time.Location {
	if filter.BucketLocation != nil {
		return filter.BucketLocation
	}
	return time.UTC
}

func localDayStartUTC(value time.Time, location *time.Location) time.Time {
	if location == nil {
		location = time.UTC
	}
	local := value.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location).UTC()
}

func nextLocalDayStartUTC(value time.Time, location *time.Location) time.Time {
	return addLocalDays(value, location, 1)
}

func addLocalDays(value time.Time, location *time.Location, days int) time.Time {
	if location == nil {
		location = time.UTC
	}
	local := value.In(location)
	next := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location).AddDate(0, 0, days)
	return next.UTC()
}

func calculateCost(filter EventFilter, row RollupPoint) float64 {
	if filter.CostCalculator == nil {
		return row.EstimatedCost
	}
	return filter.CostCalculator(row.AccountID, row.ProviderType, row.Model, row.InputTokens, row.OutputTokens)
}

func sortModelDistribution(points []ModelDistributionPoint) {
	sort.Slice(points, func(i, j int) bool {
		if points[i].RequestCount == points[j].RequestCount {
			return points[i].Model < points[j].Model
		}
		return points[i].RequestCount > points[j].RequestCount
	})
}

func (r *SQLiteRepository) loadAggregateRows(filter EventFilter) ([]RollupPoint, error) {
	rows := make([]RollupPoint, 0)

	rawRows, err := r.queryRawAggregateRows(filter)
	if err != nil {
		return nil, err
	}
	rows = append(rows, rawRows...)

	if filter.From != nil && filter.To != nil {
		hourlyRows, err := r.queryRollupRows("usage_rollups_hourly", filter)
		if err != nil {
			return nil, err
		}
		rows = append(rows, hourlyRows...)

		dailyRows, err := r.queryRollupRows("usage_rollups_daily", filter)
		if err != nil {
			return nil, err
		}
		rows = append(rows, dailyRows...)
	}

	return rows, nil
}

func (r *SQLiteRepository) queryRawAggregateRows(filter EventFilter) ([]RollupPoint, error) {
	query := `SELECT created_at, account_id, provider_type, request_kind, model, status,
		input_tokens, output_tokens, total_tokens, estimated_cost,
		balance_before, balance_after, quota_before, quota_after
		FROM usage_events`
	where, args := eventFilterWhere(filter)
	query += where + ` ORDER BY created_at ASC, id ASC`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query raw usage rows: %w", err)
	}
	defer rows.Close()

	points := make([]RollupPoint, 0)
	for rows.Next() {
		var createdAt time.Time
		var point RollupPoint
		var status string
		var balanceBefore *float64
		var balanceAfter *float64
		var quotaBefore *float64
		var quotaAfter *float64
		if err := rows.Scan(
			&createdAt,
			&point.AccountID,
			&point.ProviderType,
			&point.RequestKind,
			&point.Model,
			&status,
			&point.InputTokens,
			&point.OutputTokens,
			&point.TotalTokens,
			&point.EstimatedCost,
			nullFloatDest(&balanceBefore),
			nullFloatDest(&balanceAfter),
			nullFloatDest(&quotaBefore),
			nullFloatDest(&quotaAfter),
		); err != nil {
			return nil, fmt.Errorf("scan raw usage row: %w", err)
		}
		point.BucketStart = createdAt.UTC()
		point.RequestCount = 1
		if status == "completed" {
			point.SuccessCount = 1
		} else {
			point.FailureCount = 1
		}
		if balanceBefore != nil && balanceAfter != nil {
			point.BalanceDelta = *balanceAfter - *balanceBefore
		}
		if quotaBefore != nil && quotaAfter != nil {
			point.QuotaDelta = *quotaAfter - *quotaBefore
		}
		points = append(points, point)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate raw usage rows: %w", err)
	}
	return points, nil
}

func (r *SQLiteRepository) queryRollupRows(table string, filter EventFilter) ([]RollupPoint, error) {
	query := fmt.Sprintf(`SELECT bucket_start, account_id, provider_type, request_kind, model,
		request_count, success_count, failure_count, input_tokens, output_tokens, total_tokens,
		balance_delta, quota_delta
		FROM %s`, table)
	where, args := rollupFilterWhere(filter)
	query += where + ` ORDER BY bucket_start ASC, id ASC`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query %s rows: %w", table, err)
	}
	defer rows.Close()

	points := make([]RollupPoint, 0)
	for rows.Next() {
		var point RollupPoint
		if err := rows.Scan(
			&point.BucketStart,
			&point.AccountID,
			&point.ProviderType,
			&point.RequestKind,
			&point.Model,
			&point.RequestCount,
			&point.SuccessCount,
			&point.FailureCount,
			&point.InputTokens,
			&point.OutputTokens,
			&point.TotalTokens,
			&point.BalanceDelta,
			&point.QuotaDelta,
		); err != nil {
			return nil, fmt.Errorf("scan %s row: %w", table, err)
		}
		points = append(points, point)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s rows: %w", table, err)
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
		clauses = append(clauses, "created_at < ?")
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

func rollupFilterWhere(filter EventFilter) (string, []any) {
	clauses := make([]string, 0, 4)
	args := make([]any, 0, 4)
	if filter.From != nil {
		clauses = append(clauses, "bucket_start >= ?")
		args = append(args, filter.From.UTC())
	}
	if filter.To != nil {
		clauses = append(clauses, "bucket_start < ?")
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

func compactRawEventsToHourly(tx *sql.Tx, from time.Time, to time.Time) error {
	if !from.Before(to) {
		return nil
	}
	rows, err := tx.Query(
		`SELECT created_at, account_id, provider_type, request_kind, model, status,
			input_tokens, output_tokens, total_tokens,
			balance_before, balance_after, quota_before, quota_after
		FROM usage_events
		WHERE created_at >= ? AND created_at < ?`,
		from.UTC(),
		to.UTC(),
	)
	if err != nil {
		return fmt.Errorf("query raw events for compaction: %w", err)
	}
	defer rows.Close()

	buckets := make(map[string]RollupPoint)
	for rows.Next() {
		var createdAt time.Time
		var accountID int64
		var providerType string
		var requestKind string
		var model string
		var status string
		var inputTokens int64
		var outputTokens int64
		var totalTokens int64
		var balanceBefore *float64
		var balanceAfter *float64
		var quotaBefore *float64
		var quotaAfter *float64
		if err := rows.Scan(
			&createdAt,
			&accountID,
			&providerType,
			&requestKind,
			&model,
			&status,
			&inputTokens,
			&outputTokens,
			&totalTokens,
			nullFloatDest(&balanceBefore),
			nullFloatDest(&balanceAfter),
			nullFloatDest(&quotaBefore),
			nullFloatDest(&quotaAfter),
		); err != nil {
			return fmt.Errorf("scan raw events for compaction: %w", err)
		}
		bucket := createdAt.UTC().Truncate(time.Hour)
		key := aggregateKey(bucket, accountID, providerType, requestKind, model)
		point := buckets[key]
		point.BucketStart = bucket
		point.AccountID = accountID
		point.ProviderType = providerType
		point.RequestKind = requestKind
		point.Model = model
		point.RequestCount++
		if status == "completed" {
			point.SuccessCount++
		} else {
			point.FailureCount++
		}
		point.InputTokens += inputTokens
		point.OutputTokens += outputTokens
		point.TotalTokens += totalTokens
		if balanceBefore != nil && balanceAfter != nil {
			point.BalanceDelta += *balanceAfter - *balanceBefore
		}
		if quotaBefore != nil && quotaAfter != nil {
			point.QuotaDelta += *quotaAfter - *quotaBefore
		}
		buckets[key] = point
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate raw events for compaction: %w", err)
	}

	for _, point := range buckets {
		if err := upsertRollup(tx, "usage_rollups_hourly", point); err != nil {
			return fmt.Errorf("compact raw events to hourly: %w", err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM usage_events WHERE created_at >= ? AND created_at < ?`, from.UTC(), to.UTC()); err != nil {
		return fmt.Errorf("delete compacted raw events: %w", err)
	}
	return nil
}

func compactHourlyRollupsToDaily(tx *sql.Tx, cutoff time.Time) error {
	rows, err := tx.Query(
		`SELECT bucket_start, account_id, provider_type, request_kind, model,
			request_count, success_count, failure_count, input_tokens, output_tokens, total_tokens,
			balance_delta, quota_delta
		FROM usage_rollups_hourly
		WHERE bucket_start < ?`,
		cutoff.UTC(),
	)
	if err != nil {
		return fmt.Errorf("query hourly rollups for compaction: %w", err)
	}
	defer rows.Close()

	buckets := make(map[string]RollupPoint)
	for rows.Next() {
		var point RollupPoint
		if err := rows.Scan(
			&point.BucketStart,
			&point.AccountID,
			&point.ProviderType,
			&point.RequestKind,
			&point.Model,
			&point.RequestCount,
			&point.SuccessCount,
			&point.FailureCount,
			&point.InputTokens,
			&point.OutputTokens,
			&point.TotalTokens,
			&point.BalanceDelta,
			&point.QuotaDelta,
		); err != nil {
			return fmt.Errorf("scan hourly rollups for compaction: %w", err)
		}
		bucket := localDayStartUTC(point.BucketStart, time.Local)
		key := aggregateKey(bucket, point.AccountID, point.ProviderType, point.RequestKind, point.Model)
		current := buckets[key]
		current.BucketStart = bucket
		current.AccountID = point.AccountID
		current.ProviderType = point.ProviderType
		current.RequestKind = point.RequestKind
		current.Model = point.Model
		current.RequestCount += point.RequestCount
		current.SuccessCount += point.SuccessCount
		current.FailureCount += point.FailureCount
		current.InputTokens += point.InputTokens
		current.OutputTokens += point.OutputTokens
		current.TotalTokens += point.TotalTokens
		current.BalanceDelta += point.BalanceDelta
		current.QuotaDelta += point.QuotaDelta
		buckets[key] = current
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate hourly rollups for compaction: %w", err)
	}

	for _, point := range buckets {
		if err := upsertRollup(tx, "usage_rollups_daily", point); err != nil {
			return fmt.Errorf("compact hourly rollups to daily: %w", err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM usage_rollups_hourly WHERE bucket_start < ?`, cutoff.UTC()); err != nil {
		return fmt.Errorf("delete compacted hourly rollups: %w", err)
	}
	return nil
}

func upsertRollup(tx *sql.Tx, table string, point RollupPoint) error {
	query := fmt.Sprintf(`INSERT INTO %s (
		bucket_start, account_id, provider_type, request_kind, model,
		request_count, success_count, failure_count, input_tokens, output_tokens, total_tokens,
		balance_delta, quota_delta
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(bucket_start, account_id, provider_type, request_kind, model) DO UPDATE SET
		request_count = %s.request_count + excluded.request_count,
		success_count = %s.success_count + excluded.success_count,
		failure_count = %s.failure_count + excluded.failure_count,
		input_tokens = %s.input_tokens + excluded.input_tokens,
		output_tokens = %s.output_tokens + excluded.output_tokens,
		total_tokens = %s.total_tokens + excluded.total_tokens,
		balance_delta = %s.balance_delta + excluded.balance_delta,
		quota_delta = %s.quota_delta + excluded.quota_delta`, table, table, table, table, table, table, table, table, table)

	_, err := tx.Exec(
		query,
		point.BucketStart.UTC(),
		point.AccountID,
		point.ProviderType,
		point.RequestKind,
		point.Model,
		point.RequestCount,
		point.SuccessCount,
		point.FailureCount,
		point.InputTokens,
		point.OutputTokens,
		point.TotalTokens,
		point.BalanceDelta,
		point.QuotaDelta,
	)
	return err
}

func aggregateKey(bucket time.Time, accountID int64, providerType string, requestKind string, model string) string {
	return fmt.Sprintf("%s|%d|%s|%s|%s", bucket.UTC().Format(time.RFC3339), accountID, providerType, requestKind, model)
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
