package usagedrv

import (
	"context"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/accountdrv"
)

type UsageLimits struct {
	Balance              *float64
	QuotaRemaining       *float64
	RPMRemaining         *float64
	TPMRemaining         *float64
	DailyRemaining       *float64
	MonthlyRemaining     *float64
	PrimaryUsedPercent   *float64
	SecondaryUsedPercent *float64
	PrimaryResetsAt      *time.Time
	SecondaryResetsAt    *time.Time
}

type RawUsageResult struct {
	Source     string
	Confidence string
	Payload    map[string]any
	Limits     UsageLimits
	Meta       map[string]any
}

type UsageDriver interface {
	Name() string
	Supports(account accounts.Account) bool
	Fetch(ctx context.Context, account accounts.Account, credential accountdrv.ResolvedCredential) (RawUsageResult, error)
}
