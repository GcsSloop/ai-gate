package refresh

import (
	"context"
	"errors"
	"fmt"
	"sync"
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
	timeout  time.Duration
	workers  int
}

const defaultPerAccountTimeout = 5 * time.Second
const defaultRefreshWorkers = 4

func NewOrchestrator(accountsRepo accountLister, usageRepo snapshotRepository, driverRegistry *registry.Registry) *Orchestrator {
	return &Orchestrator{
		accounts: accountsRepo,
		usage:    usageRepo,
		registry: driverRegistry,
		now:      func() time.Time { return time.Now().UTC() },
		timeout:  defaultPerAccountTimeout,
		workers:  defaultRefreshWorkers,
	}
}

func NewOrchestratorWithTimeout(accountsRepo accountLister, usageRepo snapshotRepository, driverRegistry *registry.Registry, timeout time.Duration) *Orchestrator {
	orchestrator := NewOrchestrator(accountsRepo, usageRepo, driverRegistry)
	if timeout > 0 {
		orchestrator.timeout = timeout
	}
	return orchestrator
}

func NewOrchestratorWithOptions(accountsRepo accountLister, usageRepo snapshotRepository, driverRegistry *registry.Registry, timeout time.Duration, workers int) *Orchestrator {
	orchestrator := NewOrchestratorWithTimeout(accountsRepo, usageRepo, driverRegistry, timeout)
	if workers > 0 {
		orchestrator.workers = workers
	}
	return orchestrator
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

	if len(accountsList) == 0 {
		return nil
	}
	workers := o.workers
	if workers <= 0 {
		workers = 1
	}
	if workers > len(accountsList) {
		workers = len(accountsList)
	}

	type job struct {
		index   int
		account accounts.Account
	}
	type result struct {
		index int
		err   error
	}

	jobs := make(chan job)
	results := make(chan result, len(accountsList))
	var group sync.WaitGroup
	for i := 0; i < workers; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for work := range jobs {
				err := o.refreshOne(ctx, work.account, runAt)
				if err != nil {
					err = fmt.Errorf("account %d (%s): %w", work.account.ID, work.account.AccountName, err)
				}
				results <- result{index: work.index, err: err}
			}
		}()
	}

	go func() {
		for index, account := range accountsList {
			jobs <- job{index: index, account: account}
		}
		close(jobs)
		group.Wait()
		close(results)
	}()

	errs := make([]error, len(accountsList))
	for result := range results {
		errs[result.index] = result.err
	}

	ordered := make([]error, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			ordered = append(ordered, err)
		}
	}
	return errors.Join(ordered...)
}

func (o *Orchestrator) refreshOne(ctx context.Context, account accounts.Account, runAt time.Time) error {
	if o.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.timeout)
		defer cancel()
	}
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
