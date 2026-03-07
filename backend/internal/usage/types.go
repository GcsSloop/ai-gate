package usage

import "time"

type Snapshot struct {
	ID              int64
	AccountID       int64
	Balance         float64
	QuotaRemaining  float64
	RPMRemaining    float64
	TPMRemaining    float64
	HealthScore     float64
	RecentErrorRate float64
	AvgLatencyMS    float64
	ThrottledRecently bool
	CheckedAt       time.Time
}
