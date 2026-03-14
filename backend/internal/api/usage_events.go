package api

import (
	"strings"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/conversations"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

type usageEventStore interface {
	GetLatest(accountID int64) (usage.Snapshot, error)
	Save(snapshot usage.Snapshot) error
	SaveEvent(event usage.Event) error
}

type noopRunRecorder struct{}

func (noopRunRecorder) CreateRun(_ conversations.Run) (int64, error) {
	return 0, nil
}

func latestSnapshotOrEmpty(repo interface {
	GetLatest(accountID int64) (usage.Snapshot, error)
}, accountID int64) usage.Snapshot {
	snapshot, err := repo.GetLatest(accountID)
	if err != nil {
		return usage.Snapshot{AccountID: accountID}
	}
	return snapshot
}

func persistUsageEvent(repo usageEventStore, account accounts.Account, requestKind string, model string, status string, snapshot usage.Snapshot, startedAt time.Time) {
	if account.ID == 0 {
		return
	}

	before := latestSnapshotOrEmpty(repo, account.ID)
	snapshot.AccountID = account.ID
	if snapshot.CheckedAt.IsZero() {
		snapshot.CheckedAt = time.Now().UTC()
	}
	after := mergeUsageSnapshotWithLatest(repo, snapshot)
	if status == "completed" && hasUsageDelta(after) {
		_ = repo.Save(after)
	}

	event := usage.Event{
		AccountID:     account.ID,
		ProviderType:  string(account.ProviderType),
		RequestKind:   requestKind,
		Model:         model,
		Status:        status,
		InputTokens:   int64(after.LastInputTokens),
		OutputTokens:  int64(after.LastOutputTokens),
		TotalTokens:   int64(after.LastTotalTokens),
		EstimatedCost: estimateModelCostUSD(model, after.LastInputTokens, after.LastOutputTokens),
		LatencyMS:     time.Since(startedAt).Seconds() * 1000,
		CreatedAt:     time.Now().UTC(),
	}
	if before.Balance != 0 || after.Balance != 0 {
		event.BalanceBefore = float64Ptr(before.Balance)
		event.BalanceAfter = float64Ptr(after.Balance)
	}
	if before.QuotaRemaining != 0 || after.QuotaRemaining != 0 {
		event.QuotaBefore = float64Ptr(before.QuotaRemaining)
		event.QuotaAfter = float64Ptr(after.QuotaRemaining)
	}
	_ = repo.SaveEvent(event)
}

func mergeUsageSnapshotWithLatest(repo interface {
	GetLatest(accountID int64) (usage.Snapshot, error)
}, snapshot usage.Snapshot) usage.Snapshot {
	snapshot.CheckedAt = time.Now().UTC()
	if snapshot.AccountID == 0 {
		return snapshot
	}
	latest, err := repo.GetLatest(snapshot.AccountID)
	if err != nil {
		if snapshot.HealthScore == 0 {
			snapshot.HealthScore = 1
		}
		return snapshot
	}
	if snapshot.Balance == 0 {
		snapshot.Balance = latest.Balance
	}
	if snapshot.QuotaRemaining == 0 {
		snapshot.QuotaRemaining = latest.QuotaRemaining
	}
	if snapshot.RPMRemaining == 0 {
		snapshot.RPMRemaining = latest.RPMRemaining
	}
	if snapshot.TPMRemaining == 0 {
		snapshot.TPMRemaining = latest.TPMRemaining
	}
	if snapshot.HealthScore == 0 {
		snapshot.HealthScore = latest.HealthScore
		if snapshot.HealthScore == 0 {
			snapshot.HealthScore = 1
		}
	}
	if snapshot.ModelContextWindow == 0 {
		snapshot.ModelContextWindow = latest.ModelContextWindow
	}
	if snapshot.PrimaryUsedPercent == 0 {
		snapshot.PrimaryUsedPercent = latest.PrimaryUsedPercent
	}
	if snapshot.SecondaryUsedPercent == 0 {
		snapshot.SecondaryUsedPercent = latest.SecondaryUsedPercent
	}
	if snapshot.PrimaryResetsAt == nil {
		snapshot.PrimaryResetsAt = latest.PrimaryResetsAt
	}
	if snapshot.SecondaryResetsAt == nil {
		snapshot.SecondaryResetsAt = latest.SecondaryResetsAt
	}
	return snapshot
}

func hasUsageDelta(snapshot usage.Snapshot) bool {
	return snapshot.LastInputTokens != 0 || snapshot.LastOutputTokens != 0 || snapshot.LastTotalTokens != 0
}

func float64Ptr(value float64) *float64 {
	copied := value
	return &copied
}

type modelRate struct {
	inputPerMillion  float64
	outputPerMillion float64
}

var defaultModelRate = modelRate{inputPerMillion: 2, outputPerMillion: 8}

var modelRates = map[string]modelRate{
	"gpt-5.4":       {inputPerMillion: 5, outputPerMillion: 15},
	"gpt-5.3-codex": {inputPerMillion: 3, outputPerMillion: 12},
	"gpt-5.2":       {inputPerMillion: 2, outputPerMillion: 8},
	"gpt-5.2-codex": {inputPerMillion: 2, outputPerMillion: 8},
	"gpt-5.1-codex": {inputPerMillion: 2, outputPerMillion: 8},
	"gpt-4.1":       {inputPerMillion: 2, outputPerMillion: 8},
	"claude-sonnet": {inputPerMillion: 3, outputPerMillion: 15},
	"claude-opus":   {inputPerMillion: 15, outputPerMillion: 75},
	"claude-code":   {inputPerMillion: 3, outputPerMillion: 15},
}

func estimateModelCostUSD(model string, inputTokens float64, outputTokens float64) float64 {
	rate := defaultModelRate
	normalized := strings.ToLower(strings.TrimSpace(model))
	for prefix, candidate := range modelRates {
		if strings.HasPrefix(normalized, prefix) {
			rate = candidate
			break
		}
	}
	return (inputTokens/1_000_000)*rate.inputPerMillion + (outputTokens/1_000_000)*rate.outputPerMillion
}
