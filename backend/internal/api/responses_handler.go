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
)

const officialCodexBaseURL = "https://chatgpt.com/backend-api/codex"
const defaultCodexInstructions = "You are Codex, a coding agent based on GPT-5. You and the user share the same workspace and collaborate to achieve the user's goals. Be pragmatic, concise, and focus on completing the user's task."

var errThinGatewayRequiresResponsesAccount = errors.New("thin gateway mode requires an account that supports /responses")
var errThinGatewayActiveAccountUnsupported = errors.New("active account does not support /responses in thin gateway mode")

type ResponsesAccounts interface {
	List() ([]accounts.Account, error)
	Update(account accounts.Account) error
}

type ResponsesUsage interface {
	GetLatest(accountID int64) (usage.Snapshot, error)
	Save(snapshot usage.Snapshot) error
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
}

type ResponsesHandlerOption func(*ResponsesHandler)

func WithResponsesSettings(repo settings.ReadRepository) ResponsesHandlerOption {
	return func(handler *ResponsesHandler) {
		handler.settings = repo
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

func (h *ResponsesHandler) handleResponsesThin(w http.ResponseWriter, r *http.Request, req gatewayopenai.ResponsesRequest, rawBody []byte) {
	account, err := h.selectThinGatewayAccount()
	if err != nil {
		if errors.Is(err, errThinGatewayRequiresResponsesAccount) || errors.Is(err, errThinGatewayActiveAccountUnsupported) {
			writeThinGatewayUnsupported(w, err.Error())
			return
		}
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	inputItems, _ := gatewayopenai.ExtractResponsesInputItems(req.Input)
	conversationID, nextSequence := h.startThinAudit(r, req, account.ID, inputItems)
	if err := ensureOfficialAccountSession(r.Context(), h.client, h.accounts, &account); err != nil {
		h.recordThinAuditRun(conversationID, account.ID, req.Model, runStatusForErrorClass(classifyRunError(err)))
		writeThinGatewayFailure(w, req.Stream, http.StatusBadGateway, err)
		return
	}
	credential, err := resolveCredential(account)
	if err != nil {
		h.recordThinAuditRun(conversationID, account.ID, req.Model, runStatusForErrorClass(classifyRunError(err)))
		writeThinGatewayFailure(w, req.Stream, http.StatusBadGateway, err)
		return
	}
	startedAt := time.Now()
	logUpstreamSummary("responses", conversationID, account, "/responses", req.Model)
	resp, err := h.executeThinResponsesUpstreamRequest(r.Context(), account, credential, rawBody, req.Stream, conversationID, req.Model, startedAt)
	if err != nil {
		h.recordThinAuditRun(conversationID, account.ID, req.Model, runStatusForErrorClass(classifyRunError(err)))
		logFailureSummary("responses", conversationID, account.ID, "upstream_request", startedAt, err)
		writeThinGatewayFailure(w, req.Stream, http.StatusBadGateway, err)
		return
	}
	defer resp.Body.Close()

	runStatus := "completed"
	if resp.StatusCode >= 400 {
		runStatus = runStatusForErrorClass(classifyRunError(providers.HTTPError{StatusCode: resp.StatusCode}))
	}
	copyResponseHeaders(w.Header(), resp.Header)
	if isEventStreamResponse(resp.Header) {
		w.Header().Set("OpenAI-Model", req.Model)
		w.WriteHeader(resp.StatusCode)
		if err := copyResponseStream(w, resp.Body); err != nil {
			runStatus = runStatusForErrorClass(classifyRunError(err))
			logFailureSummary("responses", conversationID, account.ID, "read_stream", startedAt, err)
			writeThinGatewayFailure(w, true, http.StatusBadGateway, err)
		} else {
			logResultSummary("responses", conversationID, account.ID, resp.StatusCode, startedAt, "")
		}
		h.recordThinAuditRun(conversationID, account.ID, req.Model, runStatus)
		return
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		h.recordThinAuditRun(conversationID, account.ID, req.Model, runStatusForErrorClass(classifyRunError(err)))
		logFailureSummary("responses", conversationID, account.ID, "read_response", startedAt, err)
		writeThinGatewayFailure(w, false, http.StatusBadGateway, err)
		return
	}
	if resp.StatusCode < 400 {
		result := parseResponsesJSONResponse(responseBody, account.ID)
		h.appendThinAuditOutput(conversationID, nextSequence, result)
		logResultSummary("responses", conversationID, account.ID, resp.StatusCode, startedAt, result.Text)
	} else {
		logFailureSummary("responses", conversationID, account.ID, "upstream_status", startedAt, providers.HTTPError{StatusCode: resp.StatusCode})
	}
	h.recordThinAuditRun(conversationID, account.ID, req.Model, runStatus)
	w.Header().Set("OpenAI-Model", req.Model)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(responseBody)
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
		resp, err := h.client.Do(upstreamReq)
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

func shouldRetryOfficialResponsesTransportError(account accounts.Account, err error) bool {
	return usesOfficialCodexAdapter(account) && errors.Is(err, io.EOF)
}

func (h *ResponsesHandler) selectThinGatewayAccount() (accounts.Account, error) {
	accountList, err := h.accounts.List()
	if err != nil {
		return accounts.Account{}, err
	}
	if h.autoFailoverEnabled() {
		ordered, err := settings.OrderCandidates(h.settings, h.buildCandidates(accountList))
		if err != nil {
			return accounts.Account{}, err
		}
		for _, candidate := range ordered {
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
		if candidate.Account.NativeResponsesCapable() {
			logThinGatewayCandidate(candidate.Account, "select", "scored_candidate")
			return candidate.Account, nil
		}
		logThinGatewayCandidate(candidate.Account, "skip", "supports_responses=false")
	}
	return accounts.Account{}, errThinGatewayRequiresResponsesAccount
}

func (h *ResponsesHandler) autoFailoverEnabled() bool {
	if h.settings == nil {
		return false
	}
	appSettings, err := h.settings.GetAppSettings()
	if err != nil || !appSettings.AutoFailoverEnabled {
		return false
	}
	queue, err := h.settings.ListFailoverQueue()
	if err != nil {
		return false
	}
	return len(queue) > 0
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

func isEventStreamResponse(headers http.Header) bool {
	return strings.Contains(strings.ToLower(headers.Get("Content-Type")), "text/event-stream")
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

func (h *ResponsesHandler) handleModels(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   listModels(),
	})
}

func (h *ResponsesHandler) handleModelDetail(w http.ResponseWriter, r *http.Request) {
	modelID := pathBase(r.URL.Path)
	for _, model := range listModels() {
		if model["id"] == modelID {
			writeJSON(w, http.StatusOK, model)
			return
		}
	}
	http.NotFound(w, r)
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
	return candidates
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
	if resp.StatusCode == http.StatusTooManyRequests {
		return providers.ErrInsufficientQuota
	}
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		detail := strings.TrimSpace(string(raw))
		if detail != "" {
			detail = compactErrorText(detail, 512)
			return fmt.Errorf("http status %d: %s: %w", resp.StatusCode, detail, providers.HTTPError{StatusCode: resp.StatusCode})
		}
		return providers.HTTPError{StatusCode: resp.StatusCode}
	}
	return nil
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
	case errors.Is(err, providers.ErrInsufficientQuota):
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

func listModels() []map[string]any {
	return []map[string]any{
		buildModel("gpt-5.4", 272000, 32000),
		buildModel("gpt-5.2-codex", 272000, 32000),
		buildModel("gpt-5.1-codex-max", 272000, 32000),
		buildModel("gpt-4.1", 128000, 16000),
	}
}

func buildModel(id string, contextWindow int, maxOutputTokens int) map[string]any {
	return map[string]any{
		"id":                 id,
		"object":             "model",
		"owned_by":           "codex-router",
		"context_window":     contextWindow,
		"max_output_tokens":  maxOutputTokens,
		"supports_responses": true,
		"supports_streaming": true,
		"supports_tools":     true,
		"supports_reasoning": true,
		"supports_vision":    false,
		"default_endpoint":   "/v1/responses",
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

func pathBase(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
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

func (h *ResponsesHandler) mergeUsageSnapshot(snapshot usage.Snapshot) usage.Snapshot {
	snapshot.CheckedAt = time.Now().UTC()
	if snapshot.AccountID == 0 {
		return snapshot
	}
	latest, err := h.usage.GetLatest(snapshot.AccountID)
	if err != nil {
		if snapshot.HealthScore == 0 {
			snapshot.HealthScore = 1
		}
		return snapshot
	}
	if snapshot.Balance == 0 {
		snapshot.Balance = latest.Balance
	}
	if snapshot.QuotaRemaining == 0 {
		snapshot.QuotaRemaining = latest.QuotaRemaining
	}
	if snapshot.RPMRemaining == 0 {
		snapshot.RPMRemaining = latest.RPMRemaining
	}
	if snapshot.TPMRemaining == 0 {
		snapshot.TPMRemaining = latest.TPMRemaining
	}
	if snapshot.HealthScore == 0 {
		snapshot.HealthScore = latest.HealthScore
		if snapshot.HealthScore == 0 {
			snapshot.HealthScore = 1
		}
	}
	if snapshot.ModelContextWindow == 0 {
		snapshot.ModelContextWindow = latest.ModelContextWindow
	}
	if snapshot.PrimaryUsedPercent == 0 {
		snapshot.PrimaryUsedPercent = latest.PrimaryUsedPercent
	}
	if snapshot.SecondaryUsedPercent == 0 {
		snapshot.SecondaryUsedPercent = latest.SecondaryUsedPercent
	}
	if snapshot.PrimaryResetsAt == nil {
		snapshot.PrimaryResetsAt = latest.PrimaryResetsAt
	}
	if snapshot.SecondaryResetsAt == nil {
		snapshot.SecondaryResetsAt = latest.SecondaryResetsAt
	}
	return snapshot
}
