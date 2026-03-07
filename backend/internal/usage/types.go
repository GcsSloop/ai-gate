package usage

import "time"

type Snapshot struct {
	ID                   int64
	AccountID            int64
	Balance              float64
	QuotaRemaining       float64
	RPMRemaining         float64
	TPMRemaining         float64
	HealthScore          float64
	RecentErrorRate      float64
	AvgLatencyMS         float64
	ThrottledRecently    bool
	LastTotalTokens      float64
	LastInputTokens      float64
	LastOutputTokens     float64
	ModelContextWindow   float64
	PrimaryUsedPercent   float64
	SecondaryUsedPercent float64
	PrimaryResetsAt      *time.Time
	SecondaryResetsAt    *time.Time
	CheckedAt            time.Time
}
