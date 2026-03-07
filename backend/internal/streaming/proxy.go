package streaming

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gcssloop/codex-router/backend/internal/conversations"
	"github.com/gcssloop/codex-router/backend/internal/providers"
	"github.com/gcssloop/codex-router/backend/internal/routing"
)

type RunRecorder interface {
	CreateRun(run conversations.Run) (int64, error)
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

type Attempt struct {
	Candidate          routing.Candidate
	ContinuationPrompt string
	Emit               func(chunk string) error
}

type AttemptFunc func(ctx context.Context, attempt Attempt) error

type Proxy struct {
	recorder RunRecorder
	attempt  AttemptFunc
}

func NewProxy(recorder RunRecorder, attempt AttemptFunc) *Proxy {
	return &Proxy{recorder: recorder, attempt: attempt}
}

func (p *Proxy) Execute(ctx context.Context, conversationID int64, candidates []routing.Candidate, budget routing.TokenBudget) (string, error) {
	scored := routing.ScoreCandidates(candidates)
	accumulated := ""
	var previousRunID *int64

	for _, candidate := range scored {
		if !routing.IsFeasible(budget, candidate.Snapshot) {
			continue
		}

		chunkBuffer := ""
		err := p.attempt(ctx, Attempt{
			Candidate:          candidate,
			ContinuationPrompt: continuationPrompt(accumulated),
			Emit: func(chunk string) error {
				chunkBuffer += chunk
				return nil
			},
		})

		delta := dedupePrefix(chunkBuffer, accumulated)
		accumulated += delta

		status := "completed"
		if err != nil {
			status = runStatusForClass(classify(err))
		}

		runID, recordErr := p.recorder.CreateRun(conversations.Run{
			ConversationID:    conversationID,
			AccountID:         candidate.Account.ID,
			FallbackFromRunID: previousRunID,
			Status:            status,
			StreamOffset:      len(accumulated),
		})
		if recordErr != nil {
			return accumulated, recordErr
		}

		if err == nil {
			return accumulated, nil
		}

		class := classify(err)
		if class == providers.ErrorClassCapacity || class == providers.ErrorClassRateLimit || class == providers.ErrorClassSoft {
			previousRunID = &runID
			continue
		}
		return accumulated, err
	}

	if accumulated != "" {
		return accumulated, errors.New("stream exhausted all candidates")
	}
	return "", errors.New("no candidate succeeded")
}

func continuationPrompt(accumulated string) string {
	if accumulated == "" {
		return ""
	}
	return "Continue from the existing assistant output without repeating it: " + accumulated
}

func dedupePrefix(next, existing string) string {
	if next == "" {
		return ""
	}
	if strings.HasPrefix(next, existing) {
		return strings.TrimPrefix(next, existing)
	}
	return next
}
