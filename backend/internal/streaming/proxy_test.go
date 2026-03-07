package streaming_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/conversations"
	"github.com/gcssloop/codex-router/backend/internal/providers"
	"github.com/gcssloop/codex-router/backend/internal/routing"
	"github.com/gcssloop/codex-router/backend/internal/streaming"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

func TestProxyExecuteContinuesAfterMidStreamFailure(t *testing.T) {
	t.Parallel()

	recorder := &runRecorder{}
	var prompts []string

	proxy := streaming.NewProxy(recorder, func(_ context.Context, attempt streaming.Attempt) error {
		prompts = append(prompts, attempt.ContinuationPrompt)

		switch attempt.Candidate.Account.ID {
		case 1:
			if err := attempt.Emit("Hello"); err != nil {
				return err
			}
			return providers.ErrInsufficientQuota
		case 2:
			if err := attempt.Emit("Hello world"); err != nil {
				return err
			}
			return nil
		default:
			return errors.New("unexpected account")
		}
	})

	output, err := proxy.Execute(context.Background(), 55, "gpt-5.4", []routing.Candidate{
		{
			Account: accounts.Account{ID: 1, Status: accounts.StatusActive, Priority: 100},
			Snapshot: usage.Snapshot{
				Balance:        10,
				QuotaRemaining: 10000,
				RPMRemaining:   10,
				TPMRemaining:   10000,
				HealthScore:    0.9,
			},
		},
		{
			Account: accounts.Account{ID: 2, Status: accounts.StatusActive, Priority: 90},
			Snapshot: usage.Snapshot{
				Balance:        10,
				QuotaRemaining: 10000,
				RPMRemaining:   10,
				TPMRemaining:   10000,
				HealthScore:    0.8,
			},
		},
	}, routing.TokenBudget{ProjectedInputTokens: 100, ProjectedOutputTokens: 100, SafetyFactor: 1.2, EstimatedCost: 0.01})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if output != "Hello world" {
		t.Fatalf("output = %q, want %q", output, "Hello world")
	}
	if len(recorder.runs) != 2 {
		t.Fatalf("recorded runs = %d, want 2", len(recorder.runs))
	}
	if recorder.runs[0].Status != "capacity_failed" {
		t.Fatalf("first run status = %q, want %q", recorder.runs[0].Status, "capacity_failed")
	}
	if recorder.runs[0].StreamOffset != 5 {
		t.Fatalf("first run stream offset = %d, want 5", recorder.runs[0].StreamOffset)
	}
	if recorder.runs[1].FallbackFromRunID == nil || *recorder.runs[1].FallbackFromRunID != 1 {
		t.Fatalf("fallback run id = %v, want %d", recorder.runs[1].FallbackFromRunID, 1)
	}
	if prompts[1] == "" {
		t.Fatal("continuation prompt for fallback attempt is empty")
	}
}

func TestProxyExecuteReturnsErrorWhenNoCandidateSucceeds(t *testing.T) {
	t.Parallel()

	recorder := &runRecorder{}
	proxy := streaming.NewProxy(recorder, func(_ context.Context, attempt streaming.Attempt) error {
		_ = attempt.Emit("partial")
		return errors.New("boom")
	})

	_, err := proxy.Execute(context.Background(), 66, "gpt-5.4", []routing.Candidate{
		{
			Account: accounts.Account{ID: 1, Status: accounts.StatusActive, Priority: 100},
			Snapshot: usage.Snapshot{
				Balance:        10,
				QuotaRemaining: 10000,
				RPMRemaining:   10,
				TPMRemaining:   10000,
				HealthScore:    0.9,
			},
		},
	}, routing.TokenBudget{ProjectedInputTokens: 100, ProjectedOutputTokens: 100, SafetyFactor: 1.2, EstimatedCost: 0.01})
	if err == nil {
		t.Fatal("Execute returned nil error, want failure")
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
