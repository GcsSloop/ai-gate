package refresh_test

import (
	"context"
	"errors"
	"strings"
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
	r.saved = append(r.saved, snapshot)
	r.latest[snapshot.AccountID] = snapshot
	return nil
}

func (r *stubUsageRepo) GetLatest(accountID int64) (usage.Snapshot, error) {
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
	if usageRepo.saved[0].AccountID != 1 || usageRepo.saved[0].QuotaRemaining != 1200 {
		t.Fatalf("first snapshot = %+v, want account 1 quota 1200", usageRepo.saved[0])
	}
	if usageRepo.saved[1].AccountID != 2 || usageRepo.saved[1].Balance != 9.5 {
		t.Fatalf("second snapshot = %+v, want account 2 balance 9.5", usageRepo.saved[1])
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
	stale := usageRepo.saved[0]
	if stale.AccountID != 1 || !stale.Stale {
		t.Fatalf("stale snapshot = %+v, want stale account 1 snapshot", stale)
	}
	if stale.QuotaRemaining != 321 {
		t.Fatalf("stale quota = %v, want 321", stale.QuotaRemaining)
	}
	if stale.LastError != "quota endpoint failed" {
		t.Fatalf("stale last_error = %q, want quota endpoint failed", stale.LastError)
	}
	if usageRepo.saved[1].AccountID != 2 || usageRepo.saved[1].QuotaRemaining != 88 {
		t.Fatalf("healthy snapshot = %+v, want account 2 quota 88", usageRepo.saved[1])
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

func newRegistry(t *testing.T, accountDrivers []accountdrv.AccountDriver, usageDrivers []usagedrv.UsageDriver) *registry.Registry {
	t.Helper()
	reg, err := registry.New(accountDrivers, usageDrivers)
	if err != nil {
		t.Fatalf("registry.New returned error: %v", err)
	}
	return reg
}
