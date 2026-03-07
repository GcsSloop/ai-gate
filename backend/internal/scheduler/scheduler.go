package scheduler

import (
	"context"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/routing"
)

type AccountRepository interface {
	List() ([]accounts.Account, error)
	UpdateStatus(id int64, status accounts.Status) error
	UpdateCooldown(id int64, until *time.Time) error
}

type ProbeFunc func(ctx context.Context, account accounts.Account) error

type RecoveryJob struct {
	repo           AccountRepository
	probe          ProbeFunc
	retryWindow    time.Duration
}

func NewRecoveryJob(repo AccountRepository, probe ProbeFunc, retryWindow time.Duration) *RecoveryJob {
	return &RecoveryJob{
		repo:        repo,
		probe:       probe,
		retryWindow: retryWindow,
	}
}

func (j *RecoveryJob) Run(ctx context.Context, now time.Time) error {
	accountList, err := j.repo.List()
	if err != nil {
		return err
	}

	for _, account := range accountList {
		if !routing.ShouldProbeRecovery(account, now) {
			continue
		}

		if err := j.probe(ctx, account); err != nil {
			next := now.UTC().Add(j.retryWindow)
			if updateErr := j.repo.UpdateCooldown(account.ID, &next); updateErr != nil {
				return updateErr
			}
			continue
		}

		if err := j.repo.UpdateStatus(account.ID, accounts.StatusActive); err != nil {
			return err
		}
		if err := j.repo.UpdateCooldown(account.ID, nil); err != nil {
			return err
		}
	}

	return nil
}
