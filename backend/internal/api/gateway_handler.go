package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/auth"
	"github.com/gcssloop/codex-router/backend/internal/conversations"
	gatewayopenai "github.com/gcssloop/codex-router/backend/internal/gateway/openai"
	"github.com/gcssloop/codex-router/backend/internal/providers"
	provideropenai "github.com/gcssloop/codex-router/backend/internal/providers/openai"
	"github.com/gcssloop/codex-router/backend/internal/routing"
	"github.com/gcssloop/codex-router/backend/internal/settings"
	"github.com/gcssloop/codex-router/backend/internal/usage"
	"github.com/gcssloop/codex-router/backend/internal/usage/normalize"
)

type GatewayAccounts interface {
	List() ([]accounts.Account, error)
	Update(account accounts.Account) error
	SetActive(id int64) error
}

type GatewayUsage interface {
	GetLatest(accountID int64) (usage.Snapshot, error)
	Save(snapshot usage.Snapshot) error
	SaveEvent(event usage.Event) error
}

type GatewayRuns interface {
	CreateConversation(conversation conversations.Conversation) (int64, error)
	CreateRun(run conversations.Run) (int64, error)
}

type GatewayRoutingSettings interface {
	GetAppSettings() (settings.AppSettings, error)
	ListFailoverQueue() ([]int64, error)
}

type GatewayHandler struct {
	accounts      GatewayAccounts
	usage         GatewayUsage
	conversations GatewayRuns
	settings      GatewayRoutingSettings
	client        *http.Client
	stateEvents   *StateEventBus
}

type GatewayHandlerOption func(*GatewayHandler)

func WithGatewayHTTPClient(client *http.Client) GatewayHandlerOption {
	return func(handler *GatewayHandler) {
		if client != nil {
			handler.client = client
		}
	}
}

func WithGatewaySettings(repo GatewayRoutingSettings) GatewayHandlerOption {
	return func(handler *GatewayHandler) {
		handler.settings = repo
	}
}

func WithGatewayStateEvents(bus *StateEventBus) GatewayHandlerOption {
	return func(handler *GatewayHandler) {
		handler.stateEvents = bus
	}
}

func NewGatewayHandler(accounts GatewayAccounts, usage GatewayUsage, conversations GatewayRuns, opts ...GatewayHandlerOption) *GatewayHandler {
	handler := &GatewayHandler{
		accounts:      accounts,
		usage:         usage,
		conversations: conversations,
		client:        http.DefaultClient,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(handler)
		}
	}
	return handler
}

func (h *GatewayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || (r.URL.Path != "/v1/chat/completions" && r.URL.Path != "/chat/completions") {
		http.NotFound(w, r)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	req, err := gatewayopenai.ParseChatCompletionRequest(bytes.NewReader(body))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	logRequestSummary("gateway", r.URL.Path, r.Method, req.Model, r.RemoteAddr, summarizeChatRequestLog(req.Messages, req.Stream))
	accountList, err := h.accounts.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	candidates := make([]routing.Candidate, 0, len(accountList))
	for _, account := range accountList {
		snapshot, err := h.usage.GetLatest(account.ID)
		if err != nil {
			snapshot = normalize.DefaultFallbackSnapshot(account.ID)
		}
		candidates = append(candidates, routing.Candidate{Account: account, Snapshot: snapshot})
	}

	conversationID := int64(0)

	if req.Stream {
		h.serveStream(r.Context(), w, req, body, candidates, conversationID)
		return
	}

	var upstreamResponse []byte
	orderedCandidates, err := h.orderedCandidates(candidates)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	executor := routing.NewExecutor(noopRunRecorder{}, func(ctx context.Context, candidate routing.Candidate) error {
		account := candidate.Account
		startedAt := time.Now()
		logUpstreamSummary("gateway", conversationID, account, "/chat/completions", req.Model)
		if err := ensureOfficialAccountSession(ctx, h.client, h.accounts, &account); err != nil {
			logFailureSummary("gateway", conversationID, account.ID, account.AccountName, "ensure_session", startedAt, err)
			return err
		}
		credential, err := resolveCredential(account)
		if err != nil {
			logFailureSummary("gateway", conversationID, account.ID, account.AccountName, "resolve_credential", startedAt, err)
			return err
		}

		adapter := provideropenai.NewAdapter(resolveAccountBaseURL(account))
		upstreamReq, err := adapter.BuildRequest(ctx, providers.Request{
			Path:   "/chat/completions",
			Method: http.MethodPost,
			APIKey: credential,
			Body:   body,
		})
		if err != nil {
			logFailureSummary("gateway", conversationID, account.ID, account.AccountName, "build_request", startedAt, err)
			return err
		}

		resp, err := h.client.Do(upstreamReq)
		if err != nil {
			logFailureSummary("gateway", conversationID, account.ID, account.AccountName, "upstream_request", startedAt, err)
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			responseBody, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				logFailureSummary("gateway", conversationID, account.ID, account.AccountName, "read_response", startedAt, readErr)
				return readErr
			}
			upstreamErr := buildUpstreamStatusError(resp.StatusCode, responseBody)
			logFailureSummary("gateway", conversationID, account.ID, account.AccountName, "upstream_status", startedAt, upstreamErr)
			return upstreamErr
		}

		upstreamResponse, err = io.ReadAll(resp.Body)
		if err != nil {
			logFailureSummary("gateway", conversationID, account.ID, account.AccountName, "read_response", startedAt, err)
		} else {
			logResultSummary("gateway", conversationID, account.ID, resp.StatusCode, startedAt, string(upstreamResponse))
			persistUsageEvent(h.usage, account, "chat_completions", req.Model, "completed", parseChatCompletionsUsage(upstreamResponse, account.ID), startedAt)
			if changed, err := syncActiveAccount(h.accounts, account); err == nil && changed {
				h.publishAccountRoutingStateChanged()
			}
		}
		return err
	})

	err = executor.ExecuteNonStream(r.Context(), conversationID, req.Model, orderedCandidates, routing.TokenBudget{
		ProjectedInputTokens:  float64(len(req.Messages) * 500),
		ProjectedOutputTokens: 1500,
		SafetyFactor:          1.3,
		EstimatedCost:         0.01,
	})
	if err != nil {
		if len(orderedCandidates) > 0 {
			persistUsageEvent(h.usage, orderedCandidates[0].Account, "chat_completions", req.Model, "failed", usage.Snapshot{AccountID: orderedCandidates[0].Account.ID}, time.Now().UTC())
		}
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, bytes.NewReader(upstreamResponse))
}

func (h *GatewayHandler) serveStream(ctx context.Context, w http.ResponseWriter, req gatewayopenai.ChatCompletionRequest, body []byte, candidates []routing.Candidate, conversationID int64) {
	orderedCandidates, err := h.orderedCandidates(candidates)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var lastErr error

	for _, candidate := range orderedCandidates {
		if !routing.IsFeasible(routing.TokenBudget{
			ProjectedInputTokens:  float64(len(req.Messages) * 500),
			ProjectedOutputTokens: 1500,
			SafetyFactor:          1.3,
			EstimatedCost:         0.01,
		}, candidate.Snapshot) && !candidate.Account.IsActive {
			continue
		}

		account := candidate.Account
		startedAt := time.Now()
		logUpstreamSummary("gateway", conversationID, account, "/chat/completions", req.Model)
		if err := ensureOfficialAccountSession(ctx, h.client, h.accounts, &account); err != nil {
			logFailureSummary("gateway", conversationID, account.ID, account.AccountName, "ensure_session", startedAt, err)
			persistUsageEvent(h.usage, account, "chat_completions", req.Model, runStatusForErrorClass(classifyRunError(err)), usage.Snapshot{AccountID: account.ID}, startedAt)
			lastErr = err
			if shouldFailoverOnGatewayStreamError(err) {
				continue
			}
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		credential, err := resolveCredential(account)
		if err != nil {
			logFailureSummary("gateway", conversationID, account.ID, account.AccountName, "resolve_credential", startedAt, err)
			persistUsageEvent(h.usage, account, "chat_completions", req.Model, runStatusForErrorClass(classifyRunError(err)), usage.Snapshot{AccountID: account.ID}, startedAt)
			lastErr = err
			if shouldFailoverOnGatewayStreamError(err) {
				continue
			}
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		adapter := provideropenai.NewAdapter(resolveAccountBaseURL(account))
		upstreamReq, err := adapter.BuildRequest(ctx, providers.Request{
			Path:   "/chat/completions",
			Method: http.MethodPost,
			APIKey: credential,
			Body:   body,
		})
		if err != nil {
			logFailureSummary("gateway", conversationID, account.ID, account.AccountName, "build_request", startedAt, err)
			persistUsageEvent(h.usage, account, "chat_completions", req.Model, runStatusForErrorClass(classifyRunError(err)), usage.Snapshot{AccountID: account.ID}, startedAt)
			lastErr = err
			if shouldFailoverOnGatewayStreamError(err) {
				continue
			}
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		resp, err := h.client.Do(upstreamReq)
		if err != nil {
			logFailureSummary("gateway", conversationID, account.ID, account.AccountName, "upstream_request", startedAt, err)
			persistUsageEvent(h.usage, account, "chat_completions", req.Model, runStatusForErrorClass(classifyRunError(err)), usage.Snapshot{AccountID: account.ID}, startedAt)
			lastErr = err
			if shouldFailoverOnGatewayStreamError(err) {
				continue
			}
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		if resp.StatusCode >= 400 {
			responseBody, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				logFailureSummary("gateway", conversationID, account.ID, account.AccountName, "read_response", startedAt, readErr)
				persistUsageEvent(h.usage, account, "chat_completions", req.Model, runStatusForErrorClass(classifyRunError(readErr)), usage.Snapshot{AccountID: account.ID}, startedAt)
				lastErr = readErr
				if shouldFailoverOnGatewayStreamError(readErr) {
					continue
				}
				http.Error(w, readErr.Error(), http.StatusBadGateway)
				return
			}
			err = buildUpstreamStatusError(resp.StatusCode, responseBody)
			persistUsageEvent(h.usage, account, "chat_completions", req.Model, runStatusForErrorClass(classifyRunError(err)), usage.Snapshot{AccountID: account.ID}, startedAt)
			logFailureSummary("gateway", conversationID, account.ID, account.AccountName, "upstream_status", startedAt, err)
			lastErr = err
			if shouldFailoverOnGatewayStreamError(err) {
				continue
			}
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		copyResponseHeaders(w.Header(), resp.Header)
		collector := newChatCompletionsUsageCollector(account.ID)
		w.WriteHeader(resp.StatusCode)
		err = copyResponseStreamWithObserver(w, resp.Body, collector.Observe)
		_ = resp.Body.Close()
		if err != nil {
			logFailureSummary("gateway", conversationID, account.ID, account.AccountName, "read_stream", startedAt, err)
			persistUsageEvent(h.usage, account, "chat_completions", req.Model, runStatusForErrorClass(classifyRunError(err)), usage.Snapshot{AccountID: account.ID}, startedAt)
			return
		}
		logResultSummary("gateway", conversationID, account.ID, resp.StatusCode, startedAt, "")
		persistUsageEvent(h.usage, account, "chat_completions", req.Model, "completed", collector.snapshotOrDefault(), startedAt)
		if changed, err := syncActiveAccount(h.accounts, account); err == nil && changed {
			h.publishAccountRoutingStateChanged()
		}
		return
	}

	if lastErr == nil {
		lastErr = errors.New("no candidate succeeded")
	}
	http.Error(w, lastErr.Error(), http.StatusBadGateway)
}

func shouldFailoverOnGatewayStreamError(err error) bool {
	class := classifyRunError(err)
	return class == providers.ErrorClassCapacity || class == providers.ErrorClassRateLimit || class == providers.ErrorClassSoft
}

func (h *GatewayHandler) publishAccountRoutingStateChanged() {
	if h.stateEvents != nil {
		h.stateEvents.Publish(AccountRoutingStateChangedTopic)
	}
}

func (h *GatewayHandler) orderedCandidates(candidates []routing.Candidate) ([]routing.Candidate, error) {
	if !autoFailoverEnabled(h.settings) {
		if candidate, ok := activeCandidate(candidates); ok {
			return []routing.Candidate{candidate}, nil
		}
		return orderCandidatesByPriority(candidates), nil
	}
	return orderCandidatesActiveFirst(candidates), nil
}

func resolveCredential(account accounts.Account) (string, error) {
	if account.AuthMode != accounts.AuthModeLocalImport {
		return account.CredentialRef, nil
	}

	file, err := auth.LoadLocalAuthFileContent([]byte(account.CredentialRef))
	if err != nil {
		return "", err
	}
	if file.Tokens.AccessToken != "" {
		return file.Tokens.AccessToken, nil
	}
	return file.Tokens.IDToken, nil
}

func resolveLocalAccountID(account accounts.Account) (string, error) {
	if account.AuthMode != accounts.AuthModeLocalImport {
		return "", nil
	}

	file, err := auth.LoadLocalAuthFileContent([]byte(account.CredentialRef))
	if err != nil {
		return "", err
	}
	if file.Tokens.AccountID != "" {
		return file.Tokens.AccountID, nil
	}
	return "", errors.New("local auth file missing account_id")
}
