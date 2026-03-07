package routing_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/conversations"
	"github.com/gcssloop/codex-router/backend/internal/providers"
	"github.com/gcssloop/codex-router/backend/internal/routing"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

func TestExecuteNonStreamRetriesSoftFailureThenSucceeds(t *testing.T) {
	t.Parallel()

	var attempts int
	recorder := &runRecorder{}
	executor := routing.NewExecutor(recorder, func(_ context.Context, candidate routing.Candidate) error {
		attempts++
		if attempts == 1 {
			return providers.HTTPError{StatusCode: 502}
		}
		return nil
	})

	err := executor.ExecuteNonStream(context.Background(), 99, []routing.Candidate{
		{
			Account: accounts.Account{ID: 1, AccountName: "primary", Status: accounts.StatusActive, Priority: 100},
			Snapshot: usage.Snapshot{HealthScore: 0.9, RPMRemaining: 10, TPMRemaining: 10000, Balance: 10, QuotaRemaining: 10000},
		},
	}, routing.TokenBudget{ProjectedInputTokens: 100, ProjectedOutputTokens: 100, SafetyFactor: 1.2, EstimatedCost: 1})
	if err != nil {
		t.Fatalf("ExecuteNonStream returned error: %v", err)
	}

	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if len(recorder.runs) != 2 {
		t.Fatalf("recorded runs = %d, want 2", len(recorder.runs))
	}
	if recorder.runs[1].Status != "completed" {
		t.Fatalf("final run status = %q, want %q", recorder.runs[1].Status, "completed")
	}
}

func TestExecuteNonStreamFailsOverOnCapacityFailure(t *testing.T) {
	t.Parallel()

	recorder := &runRecorder{}
	executor := routing.NewExecutor(recorder, func(_ context.Context, candidate routing.Candidate) error {
		if candidate.Account.ID == 1 {
			return providers.ErrInsufficientQuota
		}
		return nil
	})

	err := executor.ExecuteNonStream(context.Background(), 88, []routing.Candidate{
		{
			Account: accounts.Account{ID: 1, AccountName: "quota-exhausted", Status: accounts.StatusActive, Priority: 100},
			Snapshot: usage.Snapshot{HealthScore: 0.95, RPMRemaining: 10, TPMRemaining: 10000, Balance: 10, QuotaRemaining: 10000},
		},
		{
			Account: accounts.Account{ID: 2, AccountName: "fallback", Status: accounts.StatusActive, Priority: 90},
			Snapshot: usage.Snapshot{HealthScore: 0.9, RPMRemaining: 10, TPMRemaining: 10000, Balance: 10, QuotaRemaining: 10000},
		},
	}, routing.TokenBudget{ProjectedInputTokens: 100, ProjectedOutputTokens: 100, SafetyFactor: 1.2, EstimatedCost: 1})
	if err != nil {
		t.Fatalf("ExecuteNonStream returned error: %v", err)
	}

	if len(recorder.runs) != 2 {
		t.Fatalf("recorded runs = %d, want 2", len(recorder.runs))
	}
	if recorder.runs[0].Status != "capacity_failed" {
		t.Fatalf("first run status = %q, want %q", recorder.runs[0].Status, "capacity_failed")
	}
	if recorder.runs[1].AccountID != 2 {
		t.Fatalf("second run account = %d, want %d", recorder.runs[1].AccountID, 2)
	}
}

func TestExecuteNonStreamReturnsErrorWhenNoCandidateCanSucceed(t *testing.T) {
	t.Parallel()

	recorder := &runRecorder{}
	executor := routing.NewExecutor(recorder, func(_ context.Context, _ routing.Candidate) error {
		return errors.New("boom")
	})

	err := executor.ExecuteNonStream(context.Background(), 77, []routing.Candidate{
		{
			Account: accounts.Account{ID: 1, AccountName: "only", Status: accounts.StatusActive, Priority: 100},
			Snapshot: usage.Snapshot{HealthScore: 0.9, RPMRemaining: 10, TPMRemaining: 10000, Balance: 10, QuotaRemaining: 10000},
		},
	}, routing.TokenBudget{ProjectedInputTokens: 100, ProjectedOutputTokens: 100, SafetyFactor: 1.2, EstimatedCost: 1})
	if err == nil {
		t.Fatal("ExecuteNonStream returned nil error, want failure")
	}
}

type runRecorder struct {
	runs []conversations.Run
}

func (r *runRecorder) CreateRun(run conversations.Run) (int64, error) {
	run.ID = int64(len(r.runs) + 1)
	r.runs = append(r.runs, run)
	return run.ID, nil
}
