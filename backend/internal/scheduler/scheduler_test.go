package scheduler_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/scheduler"
)

func TestRunCooldownRecoveryRestoresRecoveredAccounts(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	expired := now.Add(-1 * time.Minute)
	repo := &stubAccountRepo{
		accounts: []accounts.Account{
			{ID: 1, AccountName: "recoverable", Status: accounts.StatusActive, CooldownUntil: &expired, CooldownReason: "rate_limited"},
		},
	}

	job := scheduler.NewRecoveryJob(repo, func(_ context.Context, account accounts.Account) error {
		if account.ID != 1 {
			t.Fatalf("probe account id = %d, want 1", account.ID)
		}
		return nil
	}, 5*time.Hour)

	if err := job.Run(context.Background(), now); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if repo.cooldownUpdates[1] != nil {
		t.Fatalf("cooldown update = %v, want nil", repo.cooldownUpdates[1])
	}
	if repo.cooldownReasonUpdates[1] != "" {
		t.Fatalf("cooldown reason update = %q, want empty", repo.cooldownReasonUpdates[1])
	}
}

func TestRunCooldownRecoveryExtendsCooldownOnFailedProbe(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	expired := now.Add(-1 * time.Minute)
	repo := &stubAccountRepo{
		accounts: []accounts.Account{
			{ID: 2, AccountName: "still-cooling", Status: accounts.StatusActive, CooldownUntil: &expired},
		},
	}

	job := scheduler.NewRecoveryJob(repo, func(context.Context, accounts.Account) error {
		return errors.New("still unavailable")
	}, 3*time.Hour)

	if err := job.Run(context.Background(), now); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	got := repo.cooldownUpdates[2]
	want := now.Add(3 * time.Hour)
	if got == nil || !got.Equal(want) {
		t.Fatalf("cooldown update = %v, want %v", got, want)
	}
}

func TestRunCooldownRecoverySkipsDisabledAndInvalid(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	expired := now.Add(-1 * time.Minute)
	repo := &stubAccountRepo{
		accounts: []accounts.Account{
			{ID: 3, Status: accounts.StatusDisabled, CooldownUntil: &expired},
			{ID: 4, Status: accounts.StatusInvalid, CooldownUntil: &expired},
		},
	}

	probed := 0
	job := scheduler.NewRecoveryJob(repo, func(context.Context, accounts.Account) error {
		probed++
		return nil
	}, 1*time.Hour)

	if err := job.Run(context.Background(), now); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if probed != 0 {
		t.Fatalf("probed = %d, want 0", probed)
	}
}

type stubAccountRepo struct {
	accounts              []accounts.Account
	statusUpdates         map[int64]accounts.Status
	cooldownUpdates       map[int64]*time.Time
	cooldownReasonUpdates map[int64]string
}

func (r *stubAccountRepo) List() ([]accounts.Account, error) {
	return r.accounts, nil
}

func (r *stubAccountRepo) UpdateStatus(id int64, status accounts.Status) error {
	if r.statusUpdates == nil {
		r.statusUpdates = make(map[int64]accounts.Status)
	}
	r.statusUpdates[id] = status
	return nil
}

func (r *stubAccountRepo) UpdateCooldown(id int64, until *time.Time) error {
	if r.cooldownUpdates == nil {
		r.cooldownUpdates = make(map[int64]*time.Time)
	}
	r.cooldownUpdates[id] = until
	return nil
}

func (r *stubAccountRepo) UpdateCooldownReason(id int64, reason string) error {
	if r.cooldownReasonUpdates == nil {
		r.cooldownReasonUpdates = make(map[int64]string)
	}
	r.cooldownReasonUpdates[id] = reason
	return nil
}
