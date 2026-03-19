package refresh_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accountdrv"
	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/usage"
	"github.com/gcssloop/codex-router/backend/internal/usage/refresh"
	"github.com/gcssloop/codex-router/backend/internal/usagedrv"
	"github.com/gcssloop/codex-router/backend/internal/usagedrv/registry"
)

type stubAccountRepo struct {
	accounts []accounts.Account
}

func (r stubAccountRepo) List() ([]accounts.Account, error) {
	return append([]accounts.Account(nil), r.accounts...), nil
}

type stubUsageRepo struct {
	mu     sync.Mutex
	saved  []usage.Snapshot
	latest map[int64]usage.Snapshot
}

func newStubUsageRepo(initial ...usage.Snapshot) *stubUsageRepo {
	repo := &stubUsageRepo{
		latest: make(map[int64]usage.Snapshot, len(initial)),
	}
	for _, snapshot := range initial {
		repo.latest[snapshot.AccountID] = snapshot
	}
	return repo
}

func (r *stubUsageRepo) Save(snapshot usage.Snapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.saved = append(r.saved, snapshot)
	r.latest[snapshot.AccountID] = snapshot
	return nil
}

func (r *stubUsageRepo) GetLatest(accountID int64) (usage.Snapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	snapshot, ok := r.latest[accountID]
	if !ok {
		return usage.Snapshot{}, errors.New("not found")
	}
	return snapshot, nil
}

type stubAccountDriver struct {
	name     string
	supports func(accounts.Account) bool
	resolve  func(context.Context, accounts.Account) (accountdrv.ResolvedCredential, error)
}

func (d stubAccountDriver) Name() string { return d.name }
func (d stubAccountDriver) Supports(account accounts.Account) bool {
	if d.supports == nil {
		return false
	}
	return d.supports(account)
}
func (d stubAccountDriver) Resolve(ctx context.Context, account accounts.Account) (accountdrv.ResolvedCredential, error) {
	return d.resolve(ctx, account)
}

type stubUsageDriver struct {
	name     string
	supports func(accounts.Account) bool
	fetch    func(context.Context, accounts.Account, accountdrv.ResolvedCredential) (usagedrv.RawUsageResult, error)
}

func (d stubUsageDriver) Name() string { return d.name }
func (d stubUsageDriver) Supports(account accounts.Account) bool {
	if d.supports == nil {
		return false
	}
	return d.supports(account)
}
func (d stubUsageDriver) Fetch(ctx context.Context, account accounts.Account, credential accountdrv.ResolvedCredential) (usagedrv.RawUsageResult, error) {
	return d.fetch(ctx, account, credential)
}

func TestOrchestratorRefreshesBuiltInAndLuaAccounts(t *testing.T) {
	t.Parallel()

	reg := newRegistry(t,
		[]accountdrv.AccountDriver{
			stubAccountDriver{
				name: "builtin_api_key",
				supports: func(account accounts.Account) bool {
					return account.AuthMode == accounts.AuthModeAPIKey
				},
				resolve: func(context.Context, accounts.Account) (accountdrv.ResolvedCredential, error) {
					return accountdrv.ResolvedCredential{AccessToken: "token"}, nil
				},
			},
		},
		[]usagedrv.UsageDriver{
			stubUsageDriver{
				name: "builtin_openai_official",
				fetch: func(context.Context, accounts.Account, accountdrv.ResolvedCredential) (usagedrv.RawUsageResult, error) {
					quota := 1200.0
					return usagedrv.RawUsageResult{
						Source:     "remote",
						Confidence: "high",
						Limits: usagedrv.UsageLimits{
							QuotaRemaining: &quota,
						},
					}, nil
				},
			},
			stubUsageDriver{
				name: "lua",
				fetch: func(context.Context, accounts.Account, accountdrv.ResolvedCredential) (usagedrv.RawUsageResult, error) {
					balance := 9.5
					return usagedrv.RawUsageResult{
						Source:     "remote",
						Confidence: "medium",
						Limits: usagedrv.UsageLimits{
							Balance: &balance,
						},
					}, nil
				},
			},
		},
	)

	usageRepo := newStubUsageRepo()
	orchestrator := refresh.NewOrchestrator(stubAccountRepo{accounts: []accounts.Account{
		{ID: 1, AccountName: "official", AuthMode: accounts.AuthModeAPIKey, UsageDriver: "builtin_openai_official"},
		{ID: 2, AccountName: "lua", AuthMode: accounts.AuthModeAPIKey, UsageDriver: "lua"},
	}}, usageRepo, reg)

	now := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)
	if err := orchestrator.Run(context.Background(), now); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(usageRepo.saved) != 2 {
		t.Fatalf("saved snapshots = %d, want 2", len(usageRepo.saved))
	}
	savedByAccount := snapshotsByAccount(usageRepo.saved)
	if savedByAccount[1].QuotaRemaining != 1200 {
		t.Fatalf("account 1 snapshot = %+v, want quota 1200", savedByAccount[1])
	}
	if savedByAccount[2].Balance != 9.5 {
		t.Fatalf("account 2 snapshot = %+v, want balance 9.5", savedByAccount[2])
	}
}

func TestOrchestratorContinuesAfterFailureAndPreservesStaleSnapshot(t *testing.T) {
	t.Parallel()

	reg := newRegistry(t,
		[]accountdrv.AccountDriver{
			stubAccountDriver{
				name: "builtin_api_key",
				supports: func(account accounts.Account) bool {
					return account.AuthMode == accounts.AuthModeAPIKey
				},
				resolve: func(context.Context, accounts.Account) (accountdrv.ResolvedCredential, error) {
					return accountdrv.ResolvedCredential{AccessToken: "token"}, nil
				},
			},
		},
		[]usagedrv.UsageDriver{
			stubUsageDriver{
				name: "builtin_ppchat",
				fetch: func(_ context.Context, account accounts.Account, _ accountdrv.ResolvedCredential) (usagedrv.RawUsageResult, error) {
					if account.ID == 1 {
						return usagedrv.RawUsageResult{}, errors.New("quota endpoint failed")
					}
					quota := 88.0
					return usagedrv.RawUsageResult{
						Source:     "remote",
						Confidence: "high",
						Limits: usagedrv.UsageLimits{
							QuotaRemaining: &quota,
						},
					}, nil
				},
			},
		},
	)

	previous := usage.Snapshot{
		AccountID:      1,
		Source:         "remote",
		Confidence:     "high",
		QuotaRemaining: 321,
		CheckedAt:      time.Date(2026, 3, 16, 10, 0, 0, 0, time.UTC),
	}
	usageRepo := newStubUsageRepo(previous)
	orchestrator := refresh.NewOrchestrator(stubAccountRepo{accounts: []accounts.Account{
		{ID: 1, AccountName: "broken", AuthMode: accounts.AuthModeAPIKey, UsageDriver: "builtin_ppchat"},
		{ID: 2, AccountName: "healthy", AuthMode: accounts.AuthModeAPIKey, UsageDriver: "builtin_ppchat"},
	}}, usageRepo, reg)

	now := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)
	err := orchestrator.Run(context.Background(), now)
	if err == nil {
		t.Fatal("Run returned nil error, want joined error")
	}
	if !strings.Contains(err.Error(), "quota endpoint failed") {
		t.Fatalf("error = %q, want quota endpoint failed", err.Error())
	}
	if len(usageRepo.saved) != 2 {
		t.Fatalf("saved snapshots = %d, want 2", len(usageRepo.saved))
	}
	savedByAccount := snapshotsByAccount(usageRepo.saved)
	stale := savedByAccount[1]
	if stale.AccountID != 1 || !stale.Stale {
		t.Fatalf("stale snapshot = %+v, want stale account 1 snapshot", stale)
	}
	if stale.QuotaRemaining != 321 {
		t.Fatalf("stale quota = %v, want 321", stale.QuotaRemaining)
	}
	if stale.LastError != "quota endpoint failed" {
		t.Fatalf("stale last_error = %q, want quota endpoint failed", stale.LastError)
	}
	if savedByAccount[2].AccountID != 2 || savedByAccount[2].QuotaRemaining != 88 {
		t.Fatalf("healthy snapshot = %+v, want account 2 quota 88", savedByAccount[2])
	}
}

func TestOrchestratorUsesFallbackSnapshotWhenNoPreviousSnapshotExists(t *testing.T) {
	t.Parallel()

	reg := newRegistry(t,
		[]accountdrv.AccountDriver{
			stubAccountDriver{
				name: "builtin_api_key",
				supports: func(account accounts.Account) bool {
					return account.AuthMode == accounts.AuthModeAPIKey
				},
				resolve: func(context.Context, accounts.Account) (accountdrv.ResolvedCredential, error) {
					return accountdrv.ResolvedCredential{}, errors.New("missing token")
				},
			},
		},
		[]usagedrv.UsageDriver{
			stubUsageDriver{name: "lua"},
		},
	)

	usageRepo := newStubUsageRepo()
	orchestrator := refresh.NewOrchestrator(stubAccountRepo{accounts: []accounts.Account{
		{ID: 7, AccountName: "first-run", AuthMode: accounts.AuthModeAPIKey, UsageDriver: "lua"},
	}}, usageRepo, reg)

	now := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)
	err := orchestrator.Run(context.Background(), now)
	if err == nil {
		t.Fatal("Run returned nil error, want account resolution error")
	}
	if len(usageRepo.saved) != 1 {
		t.Fatalf("saved snapshots = %d, want 1", len(usageRepo.saved))
	}
	snapshot := usageRepo.saved[0]
	if snapshot.AccountID != 7 || !snapshot.Stale {
		t.Fatalf("fallback snapshot = %+v, want stale account 7 snapshot", snapshot)
	}
	if snapshot.LastError != "missing token" {
		t.Fatalf("fallback last_error = %q, want missing token", snapshot.LastError)
	}
	if snapshot.Source != "inferred" || snapshot.Confidence != "low" {
		t.Fatalf("fallback metadata = source=%q confidence=%q, want inferred/low", snapshot.Source, snapshot.Confidence)
	}
}

func TestOrchestratorTimesOutSingleAccountAndContinues(t *testing.T) {
	t.Parallel()

	reg := newRegistry(t,
		[]accountdrv.AccountDriver{
			stubAccountDriver{
				name: "builtin_api_key",
				supports: func(account accounts.Account) bool {
					return account.AuthMode == accounts.AuthModeAPIKey
				},
				resolve: func(context.Context, accounts.Account) (accountdrv.ResolvedCredential, error) {
					return accountdrv.ResolvedCredential{AccessToken: "token"}, nil
				},
			},
		},
		[]usagedrv.UsageDriver{
			stubUsageDriver{
				name: "builtin_ppchat",
				fetch: func(ctx context.Context, account accounts.Account, _ accountdrv.ResolvedCredential) (usagedrv.RawUsageResult, error) {
					if account.ID == 1 {
						<-ctx.Done()
						return usagedrv.RawUsageResult{}, fmt.Errorf("usage fetch timeout: %w", ctx.Err())
					}
					quota := 42.0
					return usagedrv.RawUsageResult{
						Source:     "remote",
						Confidence: "high",
						Limits: usagedrv.UsageLimits{
							QuotaRemaining: &quota,
						},
					}, nil
				},
			},
		},
	)

	usageRepo := newStubUsageRepo()
	orchestrator := refresh.NewOrchestratorWithTimeout(stubAccountRepo{accounts: []accounts.Account{
		{ID: 1, AccountName: "hung", AuthMode: accounts.AuthModeAPIKey, UsageDriver: "builtin_ppchat"},
		{ID: 2, AccountName: "healthy", AuthMode: accounts.AuthModeAPIKey, UsageDriver: "builtin_ppchat"},
	}}, usageRepo, reg, 20*time.Millisecond)

	now := time.Date(2026, 3, 19, 9, 0, 0, 0, time.UTC)
	start := time.Now()
	err := orchestrator.Run(context.Background(), now)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("Run returned nil error, want timeout error")
	}
	if !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("error = %q, want deadline exceeded", err.Error())
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("Run took %s, want under 250ms", elapsed)
	}
	if len(usageRepo.saved) != 2 {
		t.Fatalf("saved snapshots = %d, want 2", len(usageRepo.saved))
	}
	savedByAccount := snapshotsByAccount(usageRepo.saved)
	if !savedByAccount[1].Stale || savedByAccount[1].AccountID != 1 {
		t.Fatalf("account 1 snapshot = %+v, want stale account 1 snapshot", savedByAccount[1])
	}
	if savedByAccount[2].AccountID != 2 || savedByAccount[2].QuotaRemaining != 42 {
		t.Fatalf("account 2 snapshot = %+v, want healthy account 2 quota 42", savedByAccount[2])
	}
}

func TestOrchestratorLimitsConcurrentRefreshes(t *testing.T) {
	t.Parallel()

	var current atomic.Int32
	var maxSeen atomic.Int32
	started := make(chan int64, 3)
	release := make(chan struct{})

	reg := newRegistry(t,
		[]accountdrv.AccountDriver{
			stubAccountDriver{
				name: "builtin_api_key",
				supports: func(account accounts.Account) bool {
					return account.AuthMode == accounts.AuthModeAPIKey
				},
				resolve: func(context.Context, accounts.Account) (accountdrv.ResolvedCredential, error) {
					return accountdrv.ResolvedCredential{AccessToken: "token"}, nil
				},
			},
		},
		[]usagedrv.UsageDriver{
			stubUsageDriver{
				name: "builtin_ppchat",
				fetch: func(_ context.Context, account accounts.Account, _ accountdrv.ResolvedCredential) (usagedrv.RawUsageResult, error) {
					active := current.Add(1)
					for {
						existing := maxSeen.Load()
						if active <= existing || maxSeen.CompareAndSwap(existing, active) {
							break
						}
					}
					started <- account.ID
					<-release
					current.Add(-1)
					quota := float64(account.ID)
					return usagedrv.RawUsageResult{
						Source:     "remote",
						Confidence: "high",
						Limits: usagedrv.UsageLimits{
							QuotaRemaining: &quota,
						},
					}, nil
				},
			},
		},
	)

	usageRepo := newStubUsageRepo()
	orchestrator := refresh.NewOrchestratorWithOptions(stubAccountRepo{accounts: []accounts.Account{
		{ID: 1, AccountName: "one", AuthMode: accounts.AuthModeAPIKey, UsageDriver: "builtin_ppchat"},
		{ID: 2, AccountName: "two", AuthMode: accounts.AuthModeAPIKey, UsageDriver: "builtin_ppchat"},
		{ID: 3, AccountName: "three", AuthMode: accounts.AuthModeAPIKey, UsageDriver: "builtin_ppchat"},
	}}, usageRepo, reg, 100*time.Millisecond, 2)

	done := make(chan error, 1)
	go func() {
		done <- orchestrator.Run(context.Background(), time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC))
	}()

	seen := map[int64]bool{}
	for len(seen) < 2 {
		select {
		case id := <-started:
			seen[id] = true
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timed out waiting for first two refreshes to start")
		}
	}

	select {
	case id := <-started:
		t.Fatalf("third refresh started before slot released: account=%d", id)
	case <-time.After(40 * time.Millisecond):
	}

	close(release)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Run did not complete after releasing workers")
	}

	if maxSeen.Load() != 2 {
		t.Fatalf("max concurrent refreshes = %d, want 2", maxSeen.Load())
	}
}

func snapshotsByAccount(saved []usage.Snapshot) map[int64]usage.Snapshot {
	indexed := make(map[int64]usage.Snapshot, len(saved))
	for _, snapshot := range saved {
		indexed[snapshot.AccountID] = snapshot
	}
	return indexed
}

func newRegistry(t *testing.T, accountDrivers []accountdrv.AccountDriver, usageDrivers []usagedrv.UsageDriver) *registry.Registry {
	t.Helper()
	reg, err := registry.New(accountDrivers, usageDrivers)
	if err != nil {
		t.Fatalf("registry.New returned error: %v", err)
	}
	return reg
}
