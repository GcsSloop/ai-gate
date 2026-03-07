package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/conversations"
	gatewayopenai "github.com/gcssloop/codex-router/backend/internal/gateway/openai"
	"github.com/gcssloop/codex-router/backend/internal/providers"
	provideropenai "github.com/gcssloop/codex-router/backend/internal/providers/openai"
	"github.com/gcssloop/codex-router/backend/internal/routing"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

type GatewayAccounts interface {
	List() ([]accounts.Account, error)
}

type GatewayUsage interface {
	GetLatest(accountID int64) (usage.Snapshot, error)
}

type GatewayRuns interface {
	CreateConversation(conversation conversations.Conversation) (int64, error)
	CreateRun(run conversations.Run) (int64, error)
}

type GatewayHandler struct {
	accounts      GatewayAccounts
	usage         GatewayUsage
	conversations GatewayRuns
	client        *http.Client
}

func NewGatewayHandler(accounts GatewayAccounts, usage GatewayUsage, conversations GatewayRuns) *GatewayHandler {
	return &GatewayHandler{
		accounts:      accounts,
		usage:         usage,
		conversations: conversations,
		client:        http.DefaultClient,
	}
}

func (h *GatewayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
		http.NotFound(w, r)
		return
	}

	req, err := gatewayopenai.ParseChatCompletionRequest(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	body, err := json.Marshal(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	accountList, err := h.accounts.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	candidates := make([]routing.Candidate, 0, len(accountList))
	for _, account := range accountList {
		snapshot, err := h.usage.GetLatest(account.ID)
		if err != nil {
			snapshot = usage.Snapshot{
				AccountID:       account.ID,
				Balance:         1,
				QuotaRemaining:  1_000_000,
				RPMRemaining:    100,
				TPMRemaining:    1_000_000,
				HealthScore:     0.5,
				RecentErrorRate: 0,
			}
		}
		candidates = append(candidates, routing.Candidate{Account: account, Snapshot: snapshot})
	}

	conversationID, err := h.conversations.CreateConversation(conversations.Conversation{
		ClientID:             r.RemoteAddr,
		TargetProviderFamily: "openai",
		DefaultModel:         req.Model,
		State:                "active",
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var upstreamResponse []byte
	executor := routing.NewExecutor(h.conversations, func(ctx context.Context, candidate routing.Candidate) error {
		adapter := provideropenai.NewAdapter(candidate.Account.BaseURL)
		upstreamReq, err := adapter.BuildRequest(ctx, providers.Request{
			Path:   "/chat/completions",
			Method: http.MethodPost,
			APIKey: candidate.Account.CredentialRef,
			Body:   body,
		})
		if err != nil {
			return err
		}

		resp, err := h.client.Do(upstreamReq)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			return providers.HTTPError{StatusCode: resp.StatusCode}
		}

		upstreamResponse, err = io.ReadAll(resp.Body)
		return err
	})

	err = executor.ExecuteNonStream(r.Context(), conversationID, candidates, routing.TokenBudget{
		ProjectedInputTokens:  float64(len(req.Messages) * 500),
		ProjectedOutputTokens: 1500,
		SafetyFactor:          1.3,
		EstimatedCost:         0.01,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, bytes.NewReader(upstreamResponse))
}
