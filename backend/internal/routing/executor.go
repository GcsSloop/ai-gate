package routing

import (
	"context"
	"errors"
	"fmt"

	"github.com/gcssloop/codex-router/backend/internal/conversations"
	"github.com/gcssloop/codex-router/backend/internal/providers"
)

type RunRecorder interface {
	CreateRun(run conversations.Run) (int64, error)
}

type AttemptFunc func(ctx context.Context, candidate Candidate) error

type Executor struct {
	recorder RunRecorder
	attempt  AttemptFunc
}

func NewExecutor(recorder RunRecorder, attempt AttemptFunc) *Executor {
	return &Executor{recorder: recorder, attempt: attempt}
}

func (e *Executor) ExecuteNonStream(ctx context.Context, conversationID int64, model string, candidates []Candidate, budget TokenBudget) error {
	scored := ScoreCandidates(candidates)

	for _, candidate := range scored {
		if !IsFeasible(budget, candidate.Snapshot) && !candidate.Account.IsActive {
			continue
		}

		err := e.tryCandidate(ctx, conversationID, model, candidate)
		if err == nil {
			return nil
		}

		class := classify(err)
		if class == providers.ErrorClassCapacity || class == providers.ErrorClassRateLimit {
			continue
		}
		if class == providers.ErrorClassHard {
			continue
		}
		return err
	}

	return errors.New("no candidate succeeded")
}

func (e *Executor) tryCandidate(ctx context.Context, conversationID int64, model string, candidate Candidate) error {
	err := e.attempt(ctx, candidate)
	if err == nil {
		_, recordErr := e.recorder.CreateRun(conversations.Run{
			ConversationID: conversationID,
			AccountID:      candidate.Account.ID,
			Model:          model,
			Status:         "completed",
		})
		return recordErr
	}

	class := classify(err)
	status := runStatusForClass(class)
	if _, recordErr := e.recorder.CreateRun(conversations.Run{
		ConversationID: conversationID,
		AccountID:      candidate.Account.ID,
		Model:          model,
		Status:         status,
	}); recordErr != nil {
		return recordErr
	}

	if class == providers.ErrorClassSoft {
		retryErr := e.attempt(ctx, candidate)
		retryStatus := "completed"
		if retryErr != nil {
			retryStatus = runStatusForClass(classify(retryErr))
		}
		if _, recordErr := e.recorder.CreateRun(conversations.Run{
			ConversationID: conversationID,
			AccountID:      candidate.Account.ID,
			Model:          model,
			Status:         retryStatus,
		}); recordErr != nil {
			return recordErr
		}
		if retryErr == nil {
			return nil
		}
		return retryErr
	}

	return err
}

func classify(err error) providers.ErrorClass {
	switch {
	case errors.Is(err, providers.ErrInsufficientQuota):
		return providers.ErrorClassCapacity
	default:
		var httpErr providers.HTTPError
		if errors.As(err, &httpErr) {
			switch {
			case httpErr.StatusCode == 429:
				return providers.ErrorClassRateLimit
			case httpErr.StatusCode == 401 || httpErr.StatusCode == 403:
				return providers.ErrorClassHard
			default:
				return providers.ErrorClassSoft
			}
		}
		return providers.ErrorClassSoft
	}
}

func runStatusForClass(class providers.ErrorClass) string {
	switch class {
	case providers.ErrorClassCapacity:
		return "capacity_failed"
	case providers.ErrorClassRateLimit:
		return "rate_limited"
	case providers.ErrorClassHard:
		return "hard_failed"
	case providers.ErrorClassSoft:
		return "soft_failed"
	default:
		return fmt.Sprintf("failed:%s", class)
	}
}
