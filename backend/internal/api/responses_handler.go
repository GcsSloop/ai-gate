package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/conversations"
	gatewayopenai "github.com/gcssloop/codex-router/backend/internal/gateway/openai"
	"github.com/gcssloop/codex-router/backend/internal/providers"
	providercodex "github.com/gcssloop/codex-router/backend/internal/providers/codex"
	provideropenai "github.com/gcssloop/codex-router/backend/internal/providers/openai"
	"github.com/gcssloop/codex-router/backend/internal/routing"
	"github.com/gcssloop/codex-router/backend/internal/settings"
	"github.com/gcssloop/codex-router/backend/internal/usage"
	"github.com/gcssloop/codex-router/backend/internal/usage/normalize"
)

const officialCodexBaseURL = "https://chatgpt.com/backend-api/codex"
const defaultCodexInstructions = "You are Codex, a coding agent based on GPT-5. You and the user share the same workspace and collaborate to achieve the user's goals. Be pragmatic, concise, and focus on completing the user's task."
const thinResponsesCapacityCooldownWindow = 3 * time.Minute
const thinResponsesRateLimitCooldownWindow = 1 * time.Minute
const activeAccountFailoverRetryAttempts = 3

var errThinGatewayRequiresResponsesAccount = errors.New("thin gateway mode requires an account that supports /responses")
var errThinGatewayActiveAccountUnsupported = errors.New("active account does not support /responses in thin gateway mode")
var errThinGatewayNoHealthyCandidate = errors.New("no healthy responses account available")

type ResponsesAccounts interface {
	List() ([]accounts.Account, error)
	Update(account accounts.Account) error
	SetActive(id int64) error
}

type ResponsesUsage interface {
	GetLatest(accountID int64) (usage.Snapshot, error)
	Save(snapshot usage.Snapshot) error
	SaveEvent(event usage.Event) error
}

type ResponsesRuns interface {
	CreateConversation(conversation conversations.Conversation) (int64, error)
	CreateRun(run conversations.Run) (int64, error)
	AppendMessage(message conversations.Message) error
	ListMessages(conversationID int64) ([]conversations.Message, error)
}

type ResponsesHandler struct {
	accounts      ResponsesAccounts
	usage         ResponsesUsage
	conversations ResponsesRuns
	settings      settings.ReadRepository
	client        *http.Client
	stateEvents   *StateEventBus
	sticky        *routing.StickySelector
}

type ResponsesHandlerOption func(*ResponsesHandler)

func WithResponsesHTTPClient(client *http.Client) ResponsesHandlerOption {
	return func(handler *ResponsesHandler) {
		if client != nil {
			handler.client = client
		}
	}
}

func WithResponsesSettings(repo settings.ReadRepository) ResponsesHandlerOption {
	return func(handler *ResponsesHandler) {
		handler.settings = repo
	}
}

func WithResponsesStateEvents(bus *StateEventBus) ResponsesHandlerOption {
	return func(handler *ResponsesHandler) {
		handler.stateEvents = bus
	}
}

func WithResponsesStickySelector(sticky *routing.StickySelector) ResponsesHandlerOption {
	return func(handler *ResponsesHandler) {
		if sticky != nil {
			handler.sticky = sticky
		}
	}
}

func WithResponsesServerUsers(_ any) ResponsesHandlerOption {
	return func(handler *ResponsesHandler) {
		// Deprecated: server user account pools were removed. Server users now
		// share the global upstream account pool and are used only for auth/usage.
	}
}

type responsesExecutionResult struct {
	Text        string
	Snapshot    usage.Snapshot
	OutputItems []map[string]any
}

func NewResponsesHandler(accounts ResponsesAccounts, usage ResponsesUsage, conversations ResponsesRuns, opts ...ResponsesHandlerOption) *ResponsesHandler {
	handler := &ResponsesHandler{
		accounts:      accounts,
		usage:         usage,
		conversations: conversations,
		client:        http.DefaultClient,
		sticky:        routing.NewStickySelector(time.Minute, time.Now),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(handler)
		}
	}
	return handler
}

func (h *ResponsesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && (r.URL.Path == "/v1/responses" || r.URL.Path == "/responses"):
		h.handleResponses(w, r)
	case r.Method == http.MethodPost && isTransparentResponsesSubpath(r.URL.Path):
		h.handleResponsesTransparentSubpath(w, r)
	case r.Method == http.MethodGet && (r.URL.Path == "/v1/models" || r.URL.Path == "/models"):
		h.handleModels(w, r)
	case r.Method == http.MethodGet && isModelDetailPath(r.URL.Path):
		h.handleModelDetail(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *ResponsesHandler) handleResponses(w http.ResponseWriter, r *http.Request) {
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req, err := gatewayopenai.ParseResponsesRequest(bytes.NewReader(rawBody))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	logRequestSummary("responses", r.URL.Path, r.Method, req.Model, r.RemoteAddr, summarizeResponsesRequestLog(req.Input, req.Tools, req.PreviousResponseID, req.Stream))
	h.handleResponsesThin(w, r, req, rawBody)
}

func (h *ResponsesHandler) handleResponsesTransparentSubpath(w http.ResponseWriter, r *http.Request) {
	account, err := h.selectThinGatewayAccount(r.Context())
	if err != nil {
		if errors.Is(err, errThinGatewayRequiresResponsesAccount) || errors.Is(err, errThinGatewayActiveAccountUnsupported) {
			writeThinGatewayUnsupported(w, err.Error())
			return
		}
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	if err := ensureOfficialAccountSession(r.Context(), h.client, h.accounts, &account); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	credential, err := resolveCredential(account)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	upstreamReq, err := h.buildThinResponsesProxySubpathRequest(r.Context(), account, credential, normalizedResponsesSubpath(r.URL.Path), rawBody)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	resp, err := doAccountRequest(h.client, upstreamReq, account)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	copyResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (h *ResponsesHandler) handleResponsesThin(w http.ResponseWriter, r *http.Request, req gatewayopenai.ResponsesRequest, rawBody []byte) {
	candidates, err := h.orderedThinGatewayCandidates(r.Context())
	if err != nil {
		if errors.Is(err, errThinGatewayRequiresResponsesAccount) || errors.Is(err, errThinGatewayActiveAccountUnsupported) {
			writeThinGatewayUnsupported(w, err.Error())
			return
		}
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	stickyScope := gatewayStickyScope(r.Context(), "responses")
	serverMode := serverUserFromContextExists(r.Context())
	conversationID := int64(0)
	var lastErr error
	var fallbackStatusCode int
	var fallbackHeaders http.Header
	var fallbackBody []byte

	for index, candidate := range candidates {
		maxAttempts := 1
		if candidate.Account.IsActive && candidate.Account.RoutingCooldownActive(time.Now().UTC()) {
			maxAttempts = activeAccountFailoverRetryAttempts
		}
		for attemptIndex := 0; attemptIndex < maxAttempts; attemptIndex++ {
			account := candidate.Account
			if !account.NativeResponsesCapable() {
				logThinGatewayCandidate(account, "skip", "supports_responses=false")
				break
			}
			if reason, ok := h.skipReasonForThinCandidate(candidate); ok {
				logThinGatewayCandidate(account, "skip", reason)
				h.invalidateSticky(stickyScope, account.ID)
				cooldownUntil := computeThinCandidateCooldownUntil(candidate.Snapshot, reason)
				if shouldCooldownThinCandidate(candidate, reason) {
					changed, err := h.markThinCandidateCooldown(account, candidate.Snapshot, reason)
					if err != nil {
						lastErr = err
						break
					}
					if changed {
						h.publishAccountRoutingStateChanged()
					}
				}
				if next, ok := h.nextThinFailoverTarget(candidates, index); ok {
					logResponsesFailover(conversationID, account, next.Account, reason, "preemptive", req.Model, cooldownUntil)
				}
				lastErr = errThinGatewayNoHealthyCandidate
				break
			}

			if err := ensureOfficialAccountSession(r.Context(), h.client, h.accounts, &account); err != nil {
				startedAt := time.Now().UTC()
				persistUsageEvent(r.Context(), h.usage, account, "responses", req.Model, runStatusForErrorClass(classifyRunError(err)), usage.Snapshot{AccountID: account.ID}, startedAt)
				logFailureSummary("responses", conversationID, account.ID, account.AccountName, "ensure_session", startedAt, err)
				lastErr = err
				if shouldFailoverOnThinError(err) && attemptIndex+1 < maxAttempts {
					h.invalidateStickyOnThinError(stickyScope, account.ID, err)
					continue
				}
				if shouldFailoverOnThinError(err) {
					h.invalidateStickyOnThinError(stickyScope, account.ID, err)
					break
				}
				writeThinGatewayFailure(w, req.Stream, http.StatusBadGateway, err)
				return
			}

			credential, err := resolveCredential(account)
			if err != nil {
				startedAt := time.Now().UTC()
				persistUsageEvent(r.Context(), h.usage, account, "responses", req.Model, runStatusForErrorClass(classifyRunError(err)), usage.Snapshot{AccountID: account.ID}, startedAt)
				logFailureSummary("responses", conversationID, account.ID, account.AccountName, "resolve_credential", startedAt, err)
				lastErr = err
				if shouldFailoverOnThinError(err) && attemptIndex+1 < maxAttempts {
					h.invalidateStickyOnThinError(stickyScope, account.ID, err)
					continue
				}
				if shouldFailoverOnThinError(err) {
					h.invalidateStickyOnThinError(stickyScope, account.ID, err)
					break
				}
				writeThinGatewayFailure(w, req.Stream, http.StatusBadGateway, err)
				return
			}

			startedAt := time.Now().UTC()
			logUpstreamSummary("responses", conversationID, account, "/responses", req.Model)
			resp, err := h.executeThinResponsesUpstreamRequest(r.Context(), account, credential, rawBody, req.Stream, conversationID, req.Model, startedAt)
			if err != nil {
				persistUsageEvent(r.Context(), h.usage, account, "responses", req.Model, runStatusForErrorClass(classifyRunError(err)), usage.Snapshot{AccountID: account.ID}, startedAt)
				logFailureSummary("responses", conversationID, account.ID, account.AccountName, "upstream_request", startedAt, err)
				lastErr = err
				if shouldFailoverOnThinError(err) {
					h.invalidateStickyOnThinError(stickyScope, account.ID, err)
					continue
				}
				writeThinGatewayFailure(w, req.Stream, http.StatusBadGateway, err)
				return
			}

			if resp.StatusCode >= 400 {
				responseBody, readErr := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if readErr != nil {
					persistUsageEvent(r.Context(), h.usage, account, "responses", req.Model, runStatusForErrorClass(classifyRunError(readErr)), usage.Snapshot{AccountID: account.ID}, startedAt)
					logFailureSummary("responses", conversationID, account.ID, account.AccountName, "read_response", startedAt, readErr)
					lastErr = readErr
					if shouldFailoverOnThinError(readErr) && attemptIndex+1 < maxAttempts {
						h.invalidateStickyOnThinError(stickyScope, account.ID, readErr)
						continue
					}
					if shouldFailoverOnThinError(readErr) {
						h.invalidateStickyOnThinError(stickyScope, account.ID, readErr)
						break
					}
					writeThinGatewayFailure(w, req.Stream, http.StatusBadGateway, readErr)
					return
				}

				runStatus := classifyThinResponseStatus(account, resp, responseBody)
				upstreamErr := buildUpstreamStatusError(resp.StatusCode, responseBody)
				logFailureSummary("responses", conversationID, account.ID, account.AccountName, "upstream_status", startedAt, upstreamErr)
				persistUsageEvent(r.Context(), h.usage, account, "responses", req.Model, runStatus, usage.Snapshot{AccountID: account.ID}, startedAt)

				if shouldFailoverOnThinStatus(runStatus) {
					h.invalidateStickyOnThinStatus(stickyScope, account.ID, runStatus)
					cooldownUntil := computeThinCandidateCooldownUntil(candidate.Snapshot, runStatus)
					if shouldCooldownForRunStatus(runStatus) && !serverMode {
						changed, err := h.markThinCandidateCooldown(account, candidate.Snapshot, runStatus)
						if err != nil {
							lastErr = err
							break
						}
						if changed {
							h.publishAccountRoutingStateChanged()
						}
					}
					if next, ok := h.nextThinFailoverTarget(candidates, index); ok {
						logResponsesFailover(conversationID, account, next.Account, runStatus, strconv.Itoa(resp.StatusCode), req.Model, cooldownUntil)
					}
					fallbackStatusCode = resp.StatusCode
					fallbackHeaders = resp.Header.Clone()
					fallbackBody = append(fallbackBody[:0], responseBody...)
					lastErr = upstreamErr
					if attemptIndex+1 < maxAttempts {
						continue
					}
					break
				}

				copyResponseHeaders(w.Header(), resp.Header)
				w.Header().Set("OpenAI-Model", req.Model)
				w.WriteHeader(resp.StatusCode)
				_, _ = w.Write(responseBody)
				return
			}

			runStatus := "completed"
			copyResponseHeaders(w.Header(), resp.Header)
			w.Header().Set("OpenAI-Model", req.Model)
			// Some upstream official /responses streams arrive as SSE payloads without a
			// reliable text/event-stream content type. When the caller requested
			// stream=true, prefer the SSE path so response.completed usage is not lost.
			streamResponse := req.Stream || isEventStreamResponse(resp.Header)
			if streamResponse {
				collector := newResponsesUsageCollector(account.ID)
				w.WriteHeader(resp.StatusCode)
				if err := copyResponseStreamWithObserver(w, resp.Body, collector.Observe); err != nil {
					runStatus = runStatusForErrorClass(classifyRunError(err))
					logFailureSummary("responses", conversationID, account.ID, account.AccountName, "read_stream", startedAt, err)
					persistUsageEvent(r.Context(), h.usage, account, "responses", req.Model, runStatus, usage.Snapshot{AccountID: account.ID}, startedAt)
					_ = resp.Body.Close()
					h.invalidateStickyOnThinStatus(stickyScope, account.ID, runStatus)
					writeThinGatewayFailure(w, true, http.StatusBadGateway, err)
					return
				}
				_ = resp.Body.Close()
				logResultSummary("responses", conversationID, account.ID, resp.StatusCode, startedAt, "")
				collector.Save(h.usage)
				persistUsageEvent(r.Context(), h.usage, account, "responses", req.Model, runStatus, collector.snapshotOrDefault(), startedAt)
				h.recordSuccessfulThinRoute(r.Context(), stickyScope, account)
				return
			}

			responseBody, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				persistUsageEvent(r.Context(), h.usage, account, "responses", req.Model, runStatusForErrorClass(classifyRunError(err)), usage.Snapshot{AccountID: account.ID}, startedAt)
				logFailureSummary("responses", conversationID, account.ID, account.AccountName, "read_response", startedAt, err)
				lastErr = err
				if shouldFailoverOnThinError(err) && attemptIndex+1 < maxAttempts {
					continue
				}
				if shouldFailoverOnThinError(err) {
					break
				}
				writeThinGatewayFailure(w, false, http.StatusBadGateway, err)
				return
			}

			result := parseResponsesJSONResponse(responseBody, account.ID)
			logResultSummary("responses", conversationID, account.ID, resp.StatusCode, startedAt, result.Text)
			persistUsageEvent(r.Context(), h.usage, account, "responses", req.Model, runStatus, result.Snapshot, startedAt)
			h.recordSuccessfulThinRoute(r.Context(), stickyScope, account)
			w.WriteHeader(resp.StatusCode)
			_, _ = w.Write(responseBody)
			return
		}
	}

	if lastErr == nil {
		lastErr = errThinGatewayNoHealthyCandidate
	}
	if fallbackStatusCode > 0 {
		copyResponseHeaders(w.Header(), fallbackHeaders)
		w.Header().Set("OpenAI-Model", req.Model)
		w.WriteHeader(fallbackStatusCode)
		_, _ = w.Write(fallbackBody)
		return
	}
	if errors.Is(lastErr, errThinGatewayNoHealthyCandidate) || errors.Is(lastErr, errThinGatewayRequiresResponsesAccount) {
		writeThinGatewayUnsupported(w, lastErr.Error())
		return
	}
	writeThinGatewayFailure(w, req.Stream, http.StatusBadGateway, lastErr)
}

func (h *ResponsesHandler) executeThinResponsesUpstreamRequest(ctx context.Context, account accounts.Account, credential string, rawBody []byte, stream bool, conversationID int64, model string, startedAt time.Time) (*http.Response, error) {
	attempts := 1
	if usesOfficialCodexAdapter(account) {
		attempts = 2
	}
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		upstreamReq, err := h.buildThinResponsesProxyRequest(ctx, account, credential, rawBody, stream)
		if err != nil {
			return nil, err
		}
		resp, err := doAccountRequest(h.client, upstreamReq, account)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if attempt == attempts || !shouldRetryOfficialResponsesTransportError(account, err) {
			break
		}
		log.Printf(
			"responses retry conversation_id=%d account_id=%d provider=%s endpoint=/responses model=%s attempt=%d reason=transport_eof",
			conversationID,
			account.ID,
			account.ProviderType,
			model,
			attempt+1,
		)
	}
	return nil, lastErr
}

func (h *ResponsesHandler) buildThinResponsesProxyRequest(ctx context.Context, account accounts.Account, credential string, rawBody []byte, stream bool) (*http.Request, error) {
	if usesOfficialCodexAdapter(account) {
		accountID, err := resolveLocalAccountID(account)
		if err != nil {
			return nil, err
		}
		return providercodex.NewAdapter(resolveAccountBaseURL(account)).BuildResponsesRequest(ctx, credential, accountID, rawBody, stream)
	}
	return provideropenai.NewAdapter(resolveAccountBaseURL(account)).BuildRequest(ctx, providers.Request{
		Path:   "/responses",
		Method: http.MethodPost,
		APIKey: credential,
		Body:   rawBody,
	})
}

func (h *ResponsesHandler) buildThinResponsesProxySubpathRequest(ctx context.Context, account accounts.Account, credential string, endpointPath string, rawBody []byte) (*http.Request, error) {
	if usesOfficialCodexAdapter(account) {
		accountID, err := resolveLocalAccountID(account)
		if err != nil {
			return nil, err
		}
		return providercodex.NewAdapter(resolveAccountBaseURL(account)).BuildResponsesEndpointRequest(ctx, credential, accountID, endpointPath, rawBody, false)
	}
	return provideropenai.NewAdapter(resolveAccountBaseURL(account)).BuildRequest(ctx, providers.Request{
		Path:   endpointPath,
		Method: http.MethodPost,
		APIKey: credential,
		Body:   rawBody,
	})
}

func (h *ResponsesHandler) buildThinModelsProxyRequest(ctx context.Context, account accounts.Account, credential string, endpointPath string) (*http.Request, error) {
	if usesOfficialCodexAdapter(account) {
		accountID, err := resolveLocalAccountID(account)
		if err != nil {
			return nil, err
		}
		return providercodex.NewAdapter(resolveAccountBaseURL(account)).BuildModelsRequest(ctx, credential, accountID, endpointPath)
	}
	return provideropenai.NewAdapter(resolveAccountBaseURL(account)).BuildRequest(ctx, providers.Request{
		Path:   endpointPath,
		Method: http.MethodGet,
		APIKey: credential,
	})
}

func shouldRetryOfficialResponsesTransportError(account accounts.Account, err error) bool {
	return usesOfficialCodexAdapter(account) && errors.Is(err, io.EOF)
}

func (h *ResponsesHandler) selectThinGatewayAccount(ctx context.Context) (accounts.Account, error) {
	accountList, err := h.accounts.List()
	if err != nil {
		return accounts.Account{}, err
	}
	if len(accountList) == 0 {
		return accounts.Account{}, errThinGatewayRequiresResponsesAccount
	}
	if serverUserFromContextExists(ctx) {
		for _, candidate := range orderServerUserCandidates(ctx, h.sticky, "responses", h.buildCandidates(accountList)) {
			if !routing.CanAttemptCandidate(candidate) {
				logThinGatewayCandidate(candidate.Account, "skip", "account_locked")
				continue
			}
			if candidate.Account.NativeResponsesCapable() {
				logThinGatewayCandidate(candidate.Account, "select", "server_sticky_or_scored")
				return candidate.Account, nil
			}
			logThinGatewayCandidate(candidate.Account, "skip", "supports_responses=false")
		}
		return accounts.Account{}, errThinGatewayRequiresResponsesAccount
	}
	if h.autoFailoverEnabled() {
		ordered, err := settings.OrderCandidates(h.settings, h.buildCandidates(accountList))
		if err != nil {
			return accounts.Account{}, err
		}
		for _, candidate := range ordered {
			if !routing.CanAttemptCandidate(candidate) {
				logThinGatewayCandidate(candidate.Account, "skip", "account_locked")
				continue
			}
			if candidate.Account.NativeResponsesCapable() {
				logThinGatewayCandidate(candidate.Account, "select", "explicit_queue")
				return candidate.Account, nil
			}
			logThinGatewayCandidate(candidate.Account, "skip", "supports_responses=false")
		}
		return accounts.Account{}, errThinGatewayRequiresResponsesAccount
	}
	for _, account := range accountList {
		if !account.IsActive {
			continue
		}
		if !account.NativeResponsesCapable() {
			logThinGatewayCandidate(account, "reject", "active_account_missing_responses_capability")
			return accounts.Account{}, errThinGatewayActiveAccountUnsupported
		}
		logThinGatewayCandidate(account, "select", "active_account")
		return account, nil
	}
	for _, candidate := range routing.ScoreCandidates(h.buildCandidates(accountList)) {
		if !routing.CanAttemptCandidate(candidate) {
			logThinGatewayCandidate(candidate.Account, "skip", "account_locked")
			continue
		}
		if candidate.Account.NativeResponsesCapable() {
			logThinGatewayCandidate(candidate.Account, "select", "scored_candidate")
			return candidate.Account, nil
		}
		logThinGatewayCandidate(candidate.Account, "skip", "supports_responses=false")
	}
	return accounts.Account{}, errThinGatewayRequiresResponsesAccount
}

func (h *ResponsesHandler) autoFailoverEnabled() bool {
	return autoFailoverEnabled(h.settings)
}

func (h *ResponsesHandler) orderedThinGatewayCandidates(ctx context.Context) ([]routing.Candidate, error) {
	accountList, err := h.accounts.List()
	if err != nil {
		return nil, err
	}
	if len(accountList) == 0 {
		return nil, errThinGatewayRequiresResponsesAccount
	}
	candidates := h.buildCandidates(accountList)
	if serverUserFromContextExists(ctx) {
		return orderServerUserCandidates(ctx, h.sticky, "responses", candidates), nil
	}
	if !h.autoFailoverEnabled() {
		for _, candidate := range candidates {
			if !candidate.Account.IsActive {
				continue
			}
			if !candidate.Account.NativeResponsesCapable() {
				logThinGatewayCandidate(candidate.Account, "reject", "active_account_missing_responses_capability")
				return nil, errThinGatewayActiveAccountUnsupported
			}
			logThinGatewayCandidate(candidate.Account, "select", "active_account")
			return []routing.Candidate{candidate}, nil
		}
	}

	if candidate, ok := activeCandidate(candidates); ok {
		if !candidate.Account.NativeResponsesCapable() {
			logThinGatewayCandidate(candidate.Account, "reject", "active_account_missing_responses_capability")
			return nil, errThinGatewayActiveAccountUnsupported
		}
	}
	return orderCandidatesActiveFirst(candidates), nil
}

func (h *ResponsesHandler) nextThinFailoverTarget(candidates []routing.Candidate, currentIndex int) (routing.Candidate, bool) {
	for i := currentIndex + 1; i < len(candidates); i++ {
		candidate := candidates[i]
		if !routing.CanAttemptCandidate(candidate) {
			continue
		}
		if !candidate.Account.NativeResponsesCapable() {
			continue
		}
		if _, skip := h.skipReasonForThinCandidate(candidate); skip {
			continue
		}
		return candidate, true
	}
	return routing.Candidate{}, false
}

func copyResponseHeaders(dst, src http.Header) {
	for key, values := range src {
		dst.Del(key)
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func copyResponseStream(w http.ResponseWriter, body io.Reader) error {
	flusher, _ := w.(http.Flusher)
	buffer := make([]byte, 32*1024)
	for {
		n, err := body.Read(buffer)
		if n > 0 {
			if _, writeErr := w.Write(buffer[:n]); writeErr != nil {
				return writeErr
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func copyResponseStreamWithObserver(w http.ResponseWriter, body io.Reader, observe func(map[string]any)) error {
	flusher, _ := w.(http.Flusher)
	reader := bufio.NewReader(body)
	var dataLines []string

	flush := func() {
		if len(dataLines) == 0 {
			return
		}
		payload := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]

		if payload != "" && payload != "[DONE]" && observe != nil {
			var frame map[string]any
			if err := json.Unmarshal([]byte(payload), &frame); err == nil {
				observe(frame)
			}
		}
	}

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}

		if len(line) > 0 {
			if _, writeErr := w.Write(line); writeErr != nil {
				return writeErr
			}
			if flusher != nil {
				flusher.Flush()
			}
		}

		trimmed := strings.TrimRight(string(line), "\r\n")

		if trimmed == "" {
			flush()
		} else if strings.HasPrefix(trimmed, "data:") {
			payload := strings.TrimPrefix(trimmed, "data:")
			if strings.HasPrefix(payload, " ") {
				payload = payload[1:]
			}
			dataLines = append(dataLines, payload)
		}

		if errors.Is(err, io.EOF) {
			flush()
			return nil
		}
	}
}

func isEventStreamResponse(headers http.Header) bool {
	return strings.Contains(strings.ToLower(headers.Get("Content-Type")), "text/event-stream")
}

func isTransparentResponsesSubpath(path string) bool {
	return path == "/responses/compact" || path == "/v1/responses/compact"
}

func normalizedResponsesSubpath(path string) string {
	if strings.HasPrefix(path, "/v1/") {
		return strings.TrimPrefix(path, "/v1")
	}
	return path
}

func normalizedModelsPath(value *url.URL) string {
	path := value.Path
	if strings.HasPrefix(path, "/v1/") {
		path = strings.TrimPrefix(path, "/v1")
	}
	if value.RawQuery != "" {
		return path + "?" + value.RawQuery
	}
	return path
}

func (h *ResponsesHandler) startThinAudit(r *http.Request, req gatewayopenai.ResponsesRequest, accountID int64, inputItems []gatewayopenai.ResponsesInputItem) (int64, int) {
	conversationID, err := h.conversations.CreateConversation(conversations.Conversation{
		ClientID:             r.RemoteAddr,
		TargetProviderFamily: "official-thin-gateway",
		DefaultModel:         req.Model,
		CurrentAccountID:     &accountID,
		State:                "active",
	})
	if err != nil {
		return 0, 0
	}
	sequence := 0
	for _, item := range inputItems {
		message := conversations.Message{
			ConversationID: conversationID,
			Role:           normalizeRole(item.Role),
			Content:        item.Content,
			ItemType:       responseInputItemType(item.Raw),
			SequenceNo:     sequence,
		}
		if rawJSON, ok := marshalRawItem(item.Raw); ok {
			message.RawItemJSON = rawJSON
		}
		if err := h.conversations.AppendMessage(message); err != nil {
			return conversationID, sequence
		}
		sequence++
	}
	return conversationID, sequence
}

func (h *ResponsesHandler) appendThinAuditOutput(conversationID int64, sequence int, result responsesExecutionResult) {
	if conversationID == 0 {
		return
	}
	outputItems := result.OutputItems
	if len(outputItems) == 0 && strings.TrimSpace(result.Text) != "" {
		outputItems = []map[string]any{buildOutputItem(newResponseItemID(), result.Text, "completed")}
	}
	for _, outputItem := range outputItems {
		message := conversations.Message{
			ConversationID: conversationID,
			Role:           "assistant",
			Content:        outputItemText(outputItem),
			ItemType:       responseOutputItemType(outputItem),
			SequenceNo:     sequence,
		}
		if rawJSON, ok := marshalRawItem(outputItem); ok {
			message.RawItemJSON = rawJSON
		}
		if strings.TrimSpace(message.Content) == "" {
			message.Content = result.Text
		}
		if err := h.conversations.AppendMessage(message); err != nil {
			return
		}
		sequence++
	}
}

func (h *ResponsesHandler) recordThinAuditRun(conversationID, accountID int64, model string, status string) {
	if conversationID == 0 {
		return
	}
	_, _ = h.conversations.CreateRun(conversations.Run{
		ConversationID: conversationID,
		AccountID:      accountID,
		Model:          model,
		Status:         status,
	})
}

func (h *ResponsesHandler) handleModels(w http.ResponseWriter, r *http.Request) {
	h.handleModelsPassthrough(w, r, normalizedModelsPath(r.URL))
}

func (h *ResponsesHandler) handleModelDetail(w http.ResponseWriter, r *http.Request) {
	h.handleModelsPassthrough(w, r, normalizedModelsPath(r.URL))
}

func (h *ResponsesHandler) handleModelsPassthrough(w http.ResponseWriter, r *http.Request, endpointPath string) {
	account, err := h.selectThinGatewayAccount(r.Context())
	if err != nil {
		if errors.Is(err, errThinGatewayRequiresResponsesAccount) || errors.Is(err, errThinGatewayActiveAccountUnsupported) {
			writeThinGatewayUnsupported(w, err.Error())
			return
		}
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	if err := ensureOfficialAccountSession(r.Context(), h.client, h.accounts, &account); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	credential, err := resolveCredential(account)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	upstreamReq, err := h.buildThinModelsProxyRequest(r.Context(), account, credential, endpointPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	resp, err := doAccountRequest(h.client, upstreamReq, account)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	copyResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func writeThinGatewayFailure(w http.ResponseWriter, stream bool, statusCode int, err error) {
	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		payload, marshalErr := json.Marshal(map[string]any{
			"type": "error",
			"error": map[string]any{
				"type":    "server_error",
				"code":    "thin_gateway_upstream_error",
				"message": err.Error(),
			},
		})
		if marshalErr == nil {
			_, _ = w.Write([]byte("data: " + string(payload) + "\n\n"))
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		return
	}
	writeJSON(w, statusCode, map[string]any{
		"error": map[string]any{
			"type":    "server_error",
			"code":    "thin_gateway_upstream_error",
			"message": err.Error(),
		},
	})
}

func writeThinGatewayUnsupported(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"error": map[string]any{
			"type":    "invalid_request_error",
			"code":    "responses_unsupported",
			"message": message,
		},
	})
}

func (h *ResponsesHandler) buildCandidates(accountList []accounts.Account) []routing.Candidate {
	candidates := make([]routing.Candidate, 0, len(accountList))
	for _, account := range accountList {
		snapshot, err := h.usage.GetLatest(account.ID)
		if err != nil {
			snapshot = normalize.DefaultFallbackSnapshot(account.ID)
		}
		candidates = append(candidates, routing.Candidate{Account: account, Snapshot: snapshot})
	}
	return candidates
}

func (h *ResponsesHandler) skipReasonForThinCandidate(candidate routing.Candidate) (string, bool) {
	now := time.Now().UTC()
	switch candidate.Account.Status {
	case accounts.StatusDisabled:
		return "status=disabled", true
	case accounts.StatusInvalid:
		return "status=invalid", true
	}
	if !routing.CanAttemptCandidate(candidate) {
		return "account_locked", true
	}
	if candidate.Account.RoutingCooldownActive(now) && !candidate.Account.IsActive {
		return "routing_cooldown", true
	}
	return "", false
}

func shouldCooldownThinCandidate(candidate routing.Candidate, reason string) bool {
	if reason == "routing_cooldown" {
		return false
	}
	if candidate.Account.IsActive {
		return false
	}
	return false
}

func shouldCooldownForRunStatus(status string) bool {
	switch status {
	case "capacity_failed", "rate_limited":
		return true
	default:
		return false
	}
}

func shouldFailoverOnThinError(err error) bool {
	class := classifyRunError(err)
	return class == providers.ErrorClassCapacity || class == providers.ErrorClassRateLimit || class == providers.ErrorClassSoft
}

func shouldFailoverOnThinStatus(status string) bool {
	switch status {
	case "usage_limited", "capacity_failed", "rate_limited":
		return true
	default:
		return false
	}
}

func (h *ResponsesHandler) markThinCandidateCooldown(account accounts.Account, snapshot usage.Snapshot, reason string) (bool, error) {
	until := computeThinCandidateCooldownUntil(snapshot, reason)
	if account.CooldownUntil != nil && until != nil && account.CooldownUntil.Equal(*until) && account.CooldownReason == reason {
		return false, nil
	}
	account.CooldownUntil = until
	account.CooldownReason = reason
	if err := h.accounts.Update(account); err != nil {
		return false, err
	}
	return true, nil
}

func (h *ResponsesHandler) publishAccountRoutingStateChanged() {
	if h.stateEvents != nil {
		h.stateEvents.Publish(AccountRoutingStateChangedTopic)
	}
}

func (h *ResponsesHandler) recordSuccessfulThinRoute(ctx context.Context, scope string, account accounts.Account) {
	if serverUserFromContextExists(ctx) {
		h.rememberSticky(scope, account.ID)
		return
	}
	if changed, err := clearRoutingCooldownIfNeeded(h.accounts, account); err == nil && changed {
		h.publishAccountRoutingStateChanged()
	}
	if changed, err := syncActiveAccount(h.accounts, account); err == nil && changed {
		h.publishAccountRoutingStateChanged()
	}
}

func (h *ResponsesHandler) rememberSticky(scope string, accountID int64) {
	if scope != "" {
		h.sticky.Remember(scope, accountID)
	}
}

func (h *ResponsesHandler) invalidateSticky(scope string, accountID int64) {
	if scope != "" {
		h.sticky.Invalidate(scope, accountID)
	}
}

func (h *ResponsesHandler) invalidateStickyOnThinError(scope string, accountID int64, err error) {
	if scope == "" || !shouldFailoverOnThinError(err) {
		return
	}
	h.sticky.Invalidate(scope, accountID)
}

func (h *ResponsesHandler) invalidateStickyOnThinStatus(scope string, accountID int64, status string) {
	if scope == "" || !shouldFailoverOnThinStatus(status) {
		return
	}
	h.sticky.Invalidate(scope, accountID)
}

func computeThinCandidateCooldownUntil(snapshot usage.Snapshot, reason string) *time.Time {
	now := time.Now().UTC()
	switch reason {
	case "usage_limited", "capacity_failed":
		resetAt := relevantOfficialResetAt(snapshot)
		until := routing.ComputeCooldownUntil(now, routing.CooldownReasonCapacity, resetAt, thinResponsesCapacityCooldownWindow)
		return &until
	case "rate_limited":
		until := routing.ComputeCooldownUntil(now, routing.CooldownReasonRateLimit, nil, thinResponsesRateLimitCooldownWindow)
		return &until
	case "routing_cooldown":
		resetAt := latestRelevantResetAt(snapshot)
		if resetAt == nil {
			return nil
		}
		until := resetAt.UTC()
		return &until
	default:
		return nil
	}
}

func latestRelevantResetAt(snapshot usage.Snapshot) *time.Time {
	return latestResetAtFor(snapshot.PrimaryResetsAt, snapshot.SecondaryResetsAt)
}

func relevantOfficialResetAt(snapshot usage.Snapshot) *time.Time {
	return latestRelevantResetAt(snapshot)
}

func latestResetAtFor(candidates ...*time.Time) *time.Time {
	var latest *time.Time
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		if latest == nil || candidate.After(*latest) {
			value := candidate.UTC()
			latest = &value
		}
	}
	return latest
}

type requestedToolSummary struct {
	Count  int
	Types  []string
	HasMCP bool
}

func (s requestedToolSummary) String() string {
	return fmt.Sprintf("count=%d types=%s has_mcp=%t", s.Count, strings.Join(s.Types, ","), s.HasMCP)
}

func summarizeRequestedTools(raw json.RawMessage) requestedToolSummary {
	summary := requestedToolSummary{}
	decoded, ok := decodeRawJSON(raw)
	if !ok {
		return summary
	}
	items, ok := decoded.([]any)
	if !ok {
		return summary
	}
	seen := map[string]struct{}{}
	for _, entry := range items {
		tool, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		summary.Count++
		toolType, _ := tool["type"].(string)
		toolType = strings.TrimSpace(strings.ToLower(toolType))
		if toolType == "" {
			continue
		}
		if _, exists := seen[toolType]; !exists {
			seen[toolType] = struct{}{}
			summary.Types = append(summary.Types, toolType)
		}
		if toolType == "mcp" {
			summary.HasMCP = true
		}
	}
	sort.Strings(summary.Types)
	return summary
}

func logResponsesDebug(format string, args ...any) {
	if !responsesDebugEnabled() {
		return
	}
	log.Printf("responses-debug: "+format, args...)
}

func responsesDebugEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("AIGATE_DEBUG_RESPONSES")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func buildResponsesInput(messages []conversations.Message) []map[string]any {
	items := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		if rawItem, ok := unmarshalRawItem(message.RawItemJSON); ok {
			items = append(items, rawItem)
			continue
		}
		items = append(items, map[string]any{
			"role": message.Role,
			"content": []map[string]any{
				{
					"type": "input_text",
					"text": message.Content,
				},
			},
		})
	}
	return items
}

func buildOfficialResponsesBody(req gatewayopenai.ResponsesRequest, messages []conversations.Message, stream bool, instructions string) map[string]any {
	body := map[string]any{
		"model":        req.Model,
		"stream":       stream,
		"store":        false,
		"instructions": instructions,
		"input":        buildResponsesInput(messages),
	}
	if value, ok := decodeRawJSON(req.Tools); ok {
		body["tools"] = value
	}
	if value, ok := decodeRawJSON(req.ToolChoice); ok {
		body["tool_choice"] = value
	}
	if req.ParallelToolCalls != nil {
		body["parallel_tool_calls"] = *req.ParallelToolCalls
	}
	if value, ok := decodeRawJSON(req.Reasoning); ok {
		body["reasoning"] = value
	}
	if value, ok := decodeRawJSON(req.Include); ok {
		body["include"] = value
	}
	if value, ok := decodeRawJSON(req.Metadata); ok {
		body["metadata"] = value
	}
	if req.MaxOutputTokens != nil {
		body["max_output_tokens"] = *req.MaxOutputTokens
	}
	return body
}

func buildOutputItem(id string, text string, status string) map[string]any {
	return map[string]any{
		"id":     id,
		"type":   "message",
		"status": status,
		"role":   "assistant",
		"content": []map[string]any{
			buildOutputTextPart(text),
		},
	}
}

func buildOutputTextPart(text string) map[string]any {
	return map[string]any{
		"type":        "output_text",
		"text":        text,
		"annotations": []any{},
	}
}

func buildResponsesUsagePayload(snapshot usage.Snapshot) map[string]any {
	if snapshot.AccountID == 0 && snapshot.LastTotalTokens == 0 && snapshot.LastInputTokens == 0 && snapshot.LastOutputTokens == 0 {
		snapshot = emptyResponsesUsageSnapshot()
	}
	return map[string]any{
		"input_tokens": snapshot.LastInputTokens,
		"input_tokens_details": map[string]any{
			"cached_tokens": 0,
		},
		"output_tokens": snapshot.LastOutputTokens,
		"output_tokens_details": map[string]any{
			"reasoning_tokens": 0,
		},
		"total_tokens": snapshot.LastTotalTokens,
	}
}

func emptyResponsesUsageSnapshot() usage.Snapshot {
	return usage.Snapshot{
		LastInputTokens:  0,
		LastOutputTokens: 0,
		LastTotalTokens:  0,
	}
}

func newResponseItemID() string {
	return "msg_" + strconv.FormatInt(time.Now().UnixNano(), 10)
}

func normalizeRole(role string) string {
	switch role {
	case "assistant", "system", "developer":
		return role
	default:
		return "user"
	}
}

func effectiveCodexInstructions(value string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return defaultCodexInstructions
}

func usesOfficialCodexAdapter(account accounts.Account) bool {
	return account.AuthMode == accounts.AuthModeLocalImport || account.ProviderType == accounts.ProviderOpenAIOfficial
}

func classifyHTTPError(resp *http.Response) error {
	if resp.StatusCode < 400 {
		return nil
	}
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests {
		return providers.ErrInsufficientQuota
	}
	return buildUpstreamStatusError(resp.StatusCode, raw)
}

func compactErrorText(value string, limit int) string {
	compact := strings.Join(strings.Fields(value), " ")
	if limit <= 0 || len(compact) <= limit {
		return compact
	}
	return compact[:limit] + "..."
}

func isStreamClosedBeforeCompleted(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "stream closed before response.completed")
}

func classifyRunError(err error) providers.ErrorClass {
	switch {
	case providers.IsInsufficientQuotaError(err):
		return providers.ErrorClassCapacity
	default:
		var httpErr providers.HTTPError
		if errors.As(err, &httpErr) {
			switch {
			case httpErr.StatusCode == http.StatusTooManyRequests:
				return providers.ErrorClassRateLimit
			case httpErr.StatusCode == http.StatusUnauthorized || httpErr.StatusCode == http.StatusForbidden:
				return providers.ErrorClassHard
			default:
				return providers.ErrorClassSoft
			}
		}
		return providers.ErrorClassSoft
	}
}

func runStatusForErrorClass(class providers.ErrorClass) string {
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

func consumeResponsesStream(body io.Reader, emit func(string) error, observe func(map[string]any)) error {
	// Parse SSE per spec: events are separated by a blank line; multiple `data:` lines
	// are concatenated with '\n'. Codex (codex-rs) treats EOF before `response.completed`
	// as an error ("stream closed before response.completed").
	reader := bufio.NewReader(body)

	var (
		dataLines    []string
		sawCompleted bool
		frameCount   int
		lastType     string
	)

	flush := func() (bool, error) {
		if len(dataLines) == 0 {
			return false, nil
		}
		payload := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]
		payload = strings.TrimSpace(payload)
		if payload == "" {
			return false, nil
		}
		if payload == "[DONE]" {
			sawCompleted = true
			return true, nil
		}

		var frame map[string]any
		if err := json.Unmarshal([]byte(payload), &frame); err != nil {
			// Upstreams sometimes send non-JSON keepalives; ignore parse errors to
			// match codex-rs behavior (it logs and continues).
			return false, nil
		}
		if observe != nil {
			observe(frame)
		}
		frameCount++
		lastType, _ = frame["type"].(string)
		logResponsesDebug("stream frame index=%d type=%q", frameCount, lastType)

		switch frame["type"] {
		case "response.output_text.delta":
			if delta, ok := frame["delta"].(string); ok {
				if err := emit(delta); err != nil {
					return false, err
				}
			}
		case "response.failed", "error":
			// Fail fast so the router can rotate to the next candidate.
			return false, errors.New("response.failed event received")
		case "response.completed":
			sawCompleted = true
			return true, nil
		}
		return false, nil
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			done, flushErr := flush()
			if flushErr != nil {
				return flushErr
			}
			if done {
				return nil
			}
		} else if strings.HasPrefix(line, "data:") {
			payload := strings.TrimPrefix(line, "data:")
			if strings.HasPrefix(payload, " ") {
				payload = payload[1:]
			}
			dataLines = append(dataLines, payload)
		}

		if errors.Is(err, io.EOF) {
			// EOF: flush any buffered event without a trailing blank line.
			done, flushErr := flush()
			if flushErr != nil {
				return flushErr
			}
			if done || sawCompleted {
				return nil
			}
			return fmt.Errorf("stream closed before response.completed (frames=%d last_type=%q)", frameCount, lastType)
		}
	}
}

func parseChatCompletionsUsage(raw []byte, accountID int64) usage.Snapshot {
	var payload struct {
		Usage struct {
			PromptTokens     float64 `json:"prompt_tokens"`
			CompletionTokens float64 `json:"completion_tokens"`
			TotalTokens      float64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return emptyResponsesUsageSnapshot()
	}
	return usage.Snapshot{
		AccountID:        accountID,
		LastInputTokens:  payload.Usage.PromptTokens,
		LastOutputTokens: payload.Usage.CompletionTokens,
		LastTotalTokens:  payload.Usage.TotalTokens,
	}
}

func parseResponsesJSONResponse(raw []byte, accountID int64) responsesExecutionResult {
	var payload struct {
		OutputText string           `json:"output_text"`
		Output     []map[string]any `json:"output"`
		Usage      struct {
			InputTokens  float64 `json:"input_tokens"`
			OutputTokens float64 `json:"output_tokens"`
			TotalTokens  float64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return responsesExecutionResult{Snapshot: emptyResponsesUsageSnapshot()}
	}
	text := strings.TrimSpace(payload.OutputText)
	if text == "" {
		text = outputItemsText(payload.Output)
	}
	return responsesExecutionResult{
		Text: text,
		Snapshot: usage.Snapshot{
			AccountID:        accountID,
			LastInputTokens:  payload.Usage.InputTokens,
			LastOutputTokens: payload.Usage.OutputTokens,
			LastTotalTokens:  payload.Usage.TotalTokens,
		},
		OutputItems: payload.Output,
	}
}

func decodeRawJSON(raw json.RawMessage) (any, bool) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" || strings.TrimSpace(string(raw)) == "null" {
		return nil, false
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, false
	}
	return value, true
}

func isModelDetailPath(path string) bool {
	return strings.HasPrefix(path, "/v1/models/") || strings.HasPrefix(path, "/models/")
}

func responseInputItemType(raw map[string]any) string {
	if raw == nil {
		return "message"
	}
	itemType, _ := raw["type"].(string)
	if strings.TrimSpace(itemType) == "" {
		return "message"
	}
	return itemType
}

func marshalRawItem(raw map[string]any) (string, bool) {
	if raw == nil {
		return "", false
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return "", false
	}
	return string(data), true
}

func unmarshalRawItem(value string) (map[string]any, bool) {
	if strings.TrimSpace(value) == "" {
		return nil, false
	}
	var item map[string]any
	if err := json.Unmarshal([]byte(value), &item); err != nil {
		return nil, false
	}
	return item, true
}

func responseOutputItemType(raw map[string]any) string {
	if raw == nil {
		return "message"
	}
	itemType, _ := raw["type"].(string)
	if strings.TrimSpace(itemType) == "" {
		return "message"
	}
	return itemType
}

func outputItemText(item map[string]any) string {
	content, ok := item["content"].([]any)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(content))
	for _, rawPart := range content {
		part, ok := rawPart.(map[string]any)
		if !ok {
			continue
		}
		if text, _ := part["text"].(string); strings.TrimSpace(text) != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func outputItemsText(items []map[string]any) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if text := strings.TrimSpace(outputItemText(item)); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func asInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return int(parsed)
		}
	}
	return 0
}

func buildResponseItemID(responseID string) string {
	return "msg_" + responseID
}
