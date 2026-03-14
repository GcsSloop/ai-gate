package usage

import "time"

type Snapshot struct {
	ID                   int64      `json:"id"`
	AccountID            int64      `json:"account_id"`
	Balance              float64    `json:"balance"`
	QuotaRemaining       float64    `json:"quota_remaining"`
	RPMRemaining         float64    `json:"rpm_remaining"`
	TPMRemaining         float64    `json:"tpm_remaining"`
	HealthScore          float64    `json:"health_score"`
	RecentErrorRate      float64    `json:"recent_error_rate"`
	AvgLatencyMS         float64    `json:"avg_latency_ms"`
	ThrottledRecently    bool       `json:"throttled_recently"`
	LastTotalTokens      float64    `json:"last_total_tokens"`
	LastInputTokens      float64    `json:"last_input_tokens"`
	LastOutputTokens     float64    `json:"last_output_tokens"`
	ModelContextWindow   float64    `json:"model_context_window"`
	PrimaryUsedPercent   float64    `json:"primary_used_percent"`
	SecondaryUsedPercent float64    `json:"secondary_used_percent"`
	PrimaryResetsAt      *time.Time `json:"primary_resets_at,omitempty"`
	SecondaryResetsAt    *time.Time `json:"secondary_resets_at,omitempty"`
	CheckedAt            time.Time  `json:"checked_at"`
}

type Event struct {
	ID            int64     `json:"id"`
	AccountID     int64     `json:"account_id"`
	ProviderType  string    `json:"provider_type"`
	RequestKind   string    `json:"request_kind"`
	Model         string    `json:"model"`
	Status        string    `json:"status"`
	InputTokens   int64     `json:"input_tokens"`
	OutputTokens  int64     `json:"output_tokens"`
	TotalTokens   int64     `json:"total_tokens"`
	EstimatedCost float64   `json:"estimated_cost"`
	BalanceBefore *float64  `json:"balance_before,omitempty"`
	BalanceAfter  *float64  `json:"balance_after,omitempty"`
	QuotaBefore   *float64  `json:"quota_before,omitempty"`
	QuotaAfter    *float64  `json:"quota_after,omitempty"`
	LatencyMS     float64   `json:"latency_ms"`
	CreatedAt     time.Time `json:"created_at"`
}

type EventFilter struct {
	From      *time.Time
	To        *time.Time
	AccountID *int64
	Model     string
	Limit     int
}

type EventSummary struct {
	RequestCount  int64   `json:"request_count"`
	SuccessCount  int64   `json:"success_count"`
	FailureCount  int64   `json:"failure_count"`
	InputTokens   int64   `json:"input_tokens"`
	OutputTokens  int64   `json:"output_tokens"`
	TotalTokens   int64   `json:"total_tokens"`
	EstimatedCost float64 `json:"estimated_cost"`
	BalanceDelta  float64 `json:"balance_delta"`
	QuotaDelta    float64 `json:"quota_delta"`
}

type TrendPoint struct {
	Bucket        time.Time `json:"bucket"`
	RequestCount  int64     `json:"request_count"`
	InputTokens   int64     `json:"input_tokens"`
	OutputTokens  int64     `json:"output_tokens"`
	TotalTokens   int64     `json:"total_tokens"`
	EstimatedCost float64   `json:"estimated_cost"`
	BalanceDelta  float64   `json:"balance_delta"`
	QuotaDelta    float64   `json:"quota_delta"`
}
