package refresh

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/usage"
	"github.com/gcssloop/codex-router/backend/internal/usage/normalize"
	"github.com/gcssloop/codex-router/backend/internal/usagedrv/registry"
)

type accountLister interface {
	List() ([]accounts.Account, error)
}

type snapshotRepository interface {
	Save(snapshot usage.Snapshot) error
	GetLatest(accountID int64) (usage.Snapshot, error)
}

type Orchestrator struct {
	accounts accountLister
	usage    snapshotRepository
	registry *registry.Registry
	now      func() time.Time
}

func NewOrchestrator(accountsRepo accountLister, usageRepo snapshotRepository, driverRegistry *registry.Registry) *Orchestrator {
	return &Orchestrator{
		accounts: accountsRepo,
		usage:    usageRepo,
		registry: driverRegistry,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func (o *Orchestrator) Run(ctx context.Context, runAt time.Time) error {
	if o == nil || o.accounts == nil || o.usage == nil || o.registry == nil {
		return nil
	}
	accountsList, err := o.accounts.List()
	if err != nil {
		return fmt.Errorf("list accounts for usage refresh: %w", err)
	}
	if runAt.IsZero() {
		runAt = o.now()
	}
	runAt = runAt.UTC()

	var errs []error
	for _, account := range accountsList {
		if err := o.refreshOne(ctx, account, runAt); err != nil {
			errs = append(errs, fmt.Errorf("account %d (%s): %w", account.ID, account.AccountName, err))
		}
	}
	return errors.Join(errs...)
}

func (o *Orchestrator) refreshOne(ctx context.Context, account accounts.Account, runAt time.Time) error {
	accountDriver, err := o.registry.AccountDriverFor(account)
	if err != nil {
		return o.markFailure(account.ID, runAt, err)
	}
	credential, err := accountDriver.Resolve(ctx, account)
	if err != nil {
		return o.markFailure(account.ID, runAt, err)
	}
	usageDriver, err := o.registry.UsageDriverFor(account)
	if err != nil {
		return o.markFailure(account.ID, runAt, err)
	}
	raw, err := usageDriver.Fetch(ctx, account, credential)
	if err != nil {
		return o.markFailure(account.ID, runAt, err)
	}
	if err := o.usage.Save(normalize.FromRaw(account.ID, raw, runAt)); err != nil {
		return fmt.Errorf("save fresh snapshot: %w", err)
	}
	return nil
}

func (o *Orchestrator) markFailure(accountID int64, runAt time.Time, cause error) error {
	snapshot, err := o.usage.GetLatest(accountID)
	if err != nil {
		snapshot = normalize.DefaultFallbackSnapshot(accountID)
	}
	snapshot.ID = 0
	snapshot.AccountID = accountID
	snapshot.CheckedAt = runAt.UTC()
	snapshot.Stale = true
	snapshot.LastError = cause.Error()
	if snapshot.Source == "" {
		snapshot.Source = "inferred"
	}
	if snapshot.Confidence == "" {
		snapshot.Confidence = "low"
	}
	if err := o.usage.Save(snapshot); err != nil {
		return fmt.Errorf("save stale snapshot: %w", err)
	}
	return cause
}
