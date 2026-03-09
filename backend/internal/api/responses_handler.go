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
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

const officialCodexBaseURL = "https://chatgpt.com/backend-api/codex"
const defaultCodexInstructions = "You are Codex, a coding agent based on GPT-5. You and the user share the same workspace and collaborate to achieve the user's goals. Be pragmatic, concise, and focus on completing the user's task."

var errThinGatewayRequiresOfficialAccount = errors.New("thin gateway mode requires an official OpenAI account")

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
	accounts        ResponsesAccounts
	usage           ResponsesUsage
	conversations   ResponsesRuns
	client          *http.Client
	thinGatewayMode bool
}

type ResponsesHandlerOption func(*ResponsesHandler)

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

func WithThinGatewayMode(enabled bool) ResponsesHandlerOption {
	return func(handler *ResponsesHandler) {
		handler.thinGatewayMode = enabled
	}
}

func (h *ResponsesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && (r.URL.Path == "/v1/responses" || r.URL.Path == "/responses"):
		h.handleResponses(w, r)
	case r.Method == http.MethodPost && isResponsesCompactPath(r.URL.Path):
		h.handleResponsesCompact(w, r)
	case r.Method == http.MethodPost && isResponsesInputTokensPath(r.URL.Path):
		h.handleResponsesInputTokens(w, r)
	case r.Method == http.MethodGet && (r.URL.Path == "/v1/models" || r.URL.Path == "/models"):
		h.handleModels(w, r)
	case r.Method == http.MethodGet && isResponseInputItemsPath(r.URL.Path):
		h.handleResponseInputItems(w, r)
	case r.Method == http.MethodGet && isResponseDetailPath(r.URL.Path):
		h.handleResponseDetail(w, r)
	case r.Method == http.MethodPost && isResponseCancelPath(r.URL.Path):
		h.handleCancelResponse(w, r)
	case r.Method == http.MethodDelete && isResponseDetailPath(r.URL.Path):
		h.handleDeleteResponse(w, r)
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
	if h.thinGatewayMode {
		h.handleResponsesThin(w, r, req, rawBody)
		return
	}
	if req.Store != nil && *req.Store {
		http.Error(w, "store is not supported", http.StatusBadRequest)
		return
	}
	log.Printf("responses: received request path=%s model=%q stream=%t remote=%q", r.URL.Path, req.Model, req.Stream, r.RemoteAddr)
	w.Header().Set("OpenAI-Model", req.Model)

	inputItems, err := gatewayopenai.ExtractResponsesInputItems(req.Input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(inputItems) == 0 {
		http.Error(w, "input is required", http.StatusBadRequest)
		return
	}

	accountList, err := h.accounts.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	candidates := h.buildCandidates(accountList)

	conversationID, existingMessages, err := h.resolveConversation(r, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if conversationID == 0 {
		conversationID, err = h.conversations.CreateConversation(conversations.Conversation{
			ClientID:             r.RemoteAddr,
			TargetProviderFamily: "codex-router",
			DefaultModel:         req.Model,
			State:                "active",
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	log.Printf("responses: using conversation conversation_id=%d model=%q", conversationID, req.Model)

	allMessages := append([]conversations.Message{}, existingMessages...)
	sequence := len(existingMessages)
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		allMessages = append(allMessages, message)
		sequence++
	}

	responseID := newRouterResponseIDForSequence(conversationID, sequence)
	if req.Stream {
		h.streamResponses(r.Context(), w, req, candidates, conversationID, responseID, allMessages, sequence)
		return
	}

	result := responsesExecutionResult{Snapshot: emptyResponsesUsageSnapshot()}
	executor := routing.NewExecutor(h.conversations, func(ctx context.Context, candidate routing.Candidate) error {
		executionResult, err := h.executeResponsesRequest(ctx, candidate.Account, req, allMessages)
		if err != nil {
			return err
		}
		result = executionResult
		return nil
	})

	err = executor.ExecuteNonStream(r.Context(), conversationID, req.Model, candidates, routing.TokenBudget{
		ProjectedInputTokens:  float64(len(allMessages) * 500),
		ProjectedOutputTokens: 1500,
		SafetyFactor:          1.3,
		EstimatedCost:         0.01,
	})
	if err != nil {
		log.Printf("responses: request failed conversation_id=%d model=%q err=%v", conversationID, req.Model, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	assistantMessage := conversations.Message{
		ConversationID: conversationID,
		Role:           "assistant",
		Content:        "",
		ItemType:       responseOutputItemType(firstOutputItem(result.OutputItems)),
		SequenceNo:     sequence,
	}
	if rawJSON, ok := marshalRawItem(firstOutputItem(result.OutputItems)); ok {
		assistantMessage.RawItemJSON = rawJSON
	}
	if len(result.OutputItems) <= 1 {
		if err := h.conversations.AppendMessage(assistantMessage); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		for index, outputItem := range result.OutputItems {
			message := assistantMessage
			message.Content = outputItemText(outputItem)
			if strings.TrimSpace(message.Content) == "" && index == 0 && responseOutputItemType(outputItem) == "message" {
				message.Content = result.Text
			}
			message.ItemType = responseOutputItemType(outputItem)
			if rawJSON, ok := marshalRawItem(outputItem); ok {
				message.RawItemJSON = rawJSON
			}
			if err := h.conversations.AppendMessage(message); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}
	if result.Snapshot.AccountID != 0 || result.Snapshot.LastTotalTokens > 0 {
		result.Snapshot = h.mergeUsageSnapshot(result.Snapshot)
		_ = h.usage.Save(result.Snapshot)
	}

	writeJSON(w, http.StatusOK, buildResponsesResponse(responseID, req.Model, result.Text, "completed", newResponseItemID(), result.Snapshot, result.OutputItems))
}

func (h *ResponsesHandler) handleResponsesThin(w http.ResponseWriter, r *http.Request, req gatewayopenai.ResponsesRequest, rawBody []byte) {
	account, err := h.selectThinGatewayAccount()
	if err != nil {
		if errors.Is(err, errThinGatewayRequiresOfficialAccount) {
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
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	credential, err := resolveCredential(account)
	if err != nil {
		h.recordThinAuditRun(conversationID, account.ID, req.Model, runStatusForErrorClass(classifyRunError(err)))
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	accountID, err := resolveLocalAccountID(account)
	if err != nil {
		h.recordThinAuditRun(conversationID, account.ID, req.Model, runStatusForErrorClass(classifyRunError(err)))
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	adapter := providercodex.NewAdapter(resolveAccountBaseURL(account))
	upstreamReq, err := adapter.BuildResponsesRequest(r.Context(), credential, accountID, rawBody, req.Stream)
	if err != nil {
		h.recordThinAuditRun(conversationID, account.ID, req.Model, runStatusForErrorClass(classifyRunError(err)))
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	resp, err := h.client.Do(upstreamReq)
	if err != nil {
		h.recordThinAuditRun(conversationID, account.ID, req.Model, runStatusForErrorClass(classifyRunError(err)))
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	runStatus := "completed"
	if resp.StatusCode >= 400 {
		runStatus = runStatusForErrorClass(classifyRunError(providers.HTTPError{StatusCode: resp.StatusCode}))
	}
	copyResponseHeaders(w.Header(), resp.Header)
	w.Header().Set("OpenAI-Model", req.Model)
	w.WriteHeader(resp.StatusCode)
	if isEventStreamResponse(resp.Header) {
		if err := copyResponseStream(w, resp.Body); err != nil {
			runStatus = runStatusForErrorClass(classifyRunError(err))
		}
		h.recordThinAuditRun(conversationID, account.ID, req.Model, runStatus)
		return
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		h.recordThinAuditRun(conversationID, account.ID, req.Model, runStatusForErrorClass(classifyRunError(err)))
		return
	}
	if resp.StatusCode < 400 {
		result := parseResponsesJSONResponse(responseBody, account.ID)
		h.appendThinAuditOutput(conversationID, nextSequence, result)
	}
	h.recordThinAuditRun(conversationID, account.ID, req.Model, runStatus)
	_, _ = w.Write(responseBody)
}

func (h *ResponsesHandler) selectThinGatewayAccount() (accounts.Account, error) {
	accountList, err := h.accounts.List()
	if err != nil {
		return accounts.Account{}, err
	}
	for _, candidate := range routing.ScoreCandidates(h.buildCandidates(accountList)) {
		if usesOfficialCodexAdapter(candidate.Account) {
			return candidate.Account, nil
		}
	}
	return accounts.Account{}, errThinGatewayRequiresOfficialAccount
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

func (h *ResponsesHandler) handleResponseDetail(w http.ResponseWriter, r *http.Request) {
	if h.thinGatewayMode {
		writeThinGatewayUnsupported(w, "response retrieval is unavailable in thin gateway mode")
		return
	}
	responseID := pathBase(r.URL.Path)
	responsePayload, err := h.buildStoredResponse(responseID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, responsePayload)
}

func (h *ResponsesHandler) handleResponseInputItems(w http.ResponseWriter, r *http.Request) {
	if h.thinGatewayMode {
		writeThinGatewayUnsupported(w, "response input_items are unavailable in thin gateway mode")
		return
	}
	responseID := pathBase(pathDir(r.URL.Path))
	items, err := h.buildStoredInputItems(responseID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	page := paginateInputItems(items, r.URL.Query())
	writeJSON(w, http.StatusOK, map[string]any{
		"object":   "list",
		"data":     page.Items,
		"first_id": page.FirstID,
		"last_id":  page.LastID,
		"has_more": page.HasMore,
	})
}

func (h *ResponsesHandler) handleDeleteResponse(w http.ResponseWriter, r *http.Request) {
	if h.thinGatewayMode {
		writeThinGatewayUnsupported(w, "response deletion is unavailable in thin gateway mode")
		return
	}
	responseID := pathBase(r.URL.Path)
	writeJSON(w, http.StatusOK, map[string]any{
		"id":      responseID,
		"object":  "response.deleted",
		"deleted": true,
	})
}

func (h *ResponsesHandler) handleCancelResponse(w http.ResponseWriter, r *http.Request) {
	if h.thinGatewayMode {
		writeThinGatewayUnsupported(w, "response cancellation is unavailable in thin gateway mode")
		return
	}
	responseID := pathBase(pathDir(r.URL.Path))
	writeJSON(w, http.StatusOK, map[string]any{
		"id":                 responseID,
		"object":             "response",
		"status":             "cancelled",
		"error":              nil,
		"incomplete_details": nil,
	})
}

func (h *ResponsesHandler) handleResponsesInputTokens(w http.ResponseWriter, r *http.Request) {
	req, err := gatewayopenai.ParseResponsesRequest(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	inputItems, err := gatewayopenai.ExtractResponsesInputItems(req.Input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	totalChars := 0
	for _, item := range inputItems {
		totalChars += len([]rune(item.Content))
	}
	estimated := totalChars/4 + len(inputItems)*4
	if estimated <= 0 {
		estimated = 1
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"input_tokens": estimated,
		"model":        req.Model,
	})
}

func (h *ResponsesHandler) handleResponsesCompact(w http.ResponseWriter, r *http.Request) {
	if h.thinGatewayMode {
		writeThinGatewayUnsupported(w, "responses compact is unavailable in thin gateway mode")
		return
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rawItems, ok := body["input"].([]any)
	if !ok || len(rawItems) == 0 {
		http.Error(w, "input must be a list", http.StatusBadRequest)
		return
	}

	// Minimal compaction: pass-through the provided history so Codex can proceed
	// even when upstream does not implement `/responses/compact`.
	output := make([]any, 0, len(rawItems))
	for _, item := range rawItems {
		output = append(output, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"output": output,
	})
}

func writeThinGatewayUnsupported(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusNotImplemented, map[string]any{
		"error": map[string]any{
			"type":    "invalid_request_error",
			"code":    "thin_gateway_unsupported",
			"message": message,
		},
	})
}

func (h *ResponsesHandler) streamResponses(ctx context.Context, w http.ResponseWriter, req gatewayopenai.ResponsesRequest, candidates []routing.Candidate, conversationID int64, responseID string, messages []conversations.Message, sequence int) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("OpenAI-Model", req.Model)
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	itemID := newResponseItemID()
	sequenceNumber := 0

	flusher, _ := w.(http.Flusher)
	writeEvent := func(event any) error {
		payload, ok := event.(map[string]any)
		if ok {
			if _, exists := payload["response_id"]; !exists {
				payload["response_id"] = responseID
			}
			if _, exists := payload["sequence_number"]; !exists {
				payload["sequence_number"] = sequenceNumber
			}
			sequenceNumber++
		}
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte("data: " + string(data) + "\n\n")); err != nil {
			return err
		}
		if flusher != nil {
			flusher.Flush()
		}
		return nil
	}

	_ = writeEvent(map[string]any{
		"type":     "response.created",
		"response": buildResponsesResponse(responseID, req.Model, "", "in_progress", itemID, emptyResponsesUsageSnapshot(), nil),
	})
	_ = writeEvent(map[string]any{
		"type":     "response.in_progress",
		"response": buildResponsesResponse(responseID, req.Model, "", "in_progress", itemID, emptyResponsesUsageSnapshot(), nil),
	})

	var assistant strings.Builder
	resultSnapshot := emptyResponsesUsageSnapshot()
	lastRunErr := errors.New("no candidate succeeded")
	textItemStarted := false
	ensureTextItemStarted := func() error {
		if textItemStarted {
			return nil
		}
		textItemStarted = true
		if err := writeEvent(map[string]any{
			"type":         "response.output_item.added",
			"output_index": 0,
			"item":         buildOutputItem(itemID, "", "in_progress"),
		}); err != nil {
			return err
		}
		return writeEvent(map[string]any{
			"type":          "response.content_part.added",
			"item_id":       itemID,
			"output_index":  0,
			"content_index": 0,
			"part":          buildOutputTextPart(""),
		})
	}
	for _, candidate := range routing.ScoreCandidates(candidates) {
		if !routing.IsFeasible(routing.TokenBudget{
			ProjectedInputTokens:  float64(len(messages) * 500),
			ProjectedOutputTokens: 1500,
			SafetyFactor:          1.3,
			EstimatedCost:         0.01,
		}, candidate.Snapshot) && !candidate.Account.IsActive {
			continue
		}
		existingOutput := assistant.String()
		candidateRaw := ""
		emittedLen := 0
		result, err := h.executeResponsesStreamRequest(ctx, candidate.Account, req, messages, func(delta string) error {
			candidateRaw += delta
			deduped := dedupePrefix(candidateRaw, existingOutput)
			if len(deduped) <= emittedLen {
				return nil
			}
			increment := deduped[emittedLen:]
			emittedLen = len(deduped)
			if strings.TrimSpace(increment) == "" {
				assistant.WriteString(increment)
				return nil
			}
			if err := ensureTextItemStarted(); err != nil {
				return err
			}
			assistant.WriteString(increment)
			return writeEvent(map[string]any{
				"type":          "response.output_text.delta",
				"delta":         increment,
				"item_id":       itemID,
				"output_index":  0,
				"content_index": 0,
			})
		})
		if err == nil {
			resultSnapshot = result.Snapshot
			outputItems := result.OutputItems
			if len(outputItems) == 0 {
				outputItems = []map[string]any{buildOutputItem(itemID, assistant.String(), "completed")}
			}
			outputItem := firstOutputItem(outputItems)
			if assistant.Len() == 0 {
				if syntheticText := strings.TrimSpace(outputItemText(outputItem)); syntheticText != "" && !isFunctionCallOutputItem(outputItem) {
					if err := ensureTextItemStarted(); err == nil {
						assistant.WriteString(syntheticText)
						_ = writeEvent(map[string]any{
							"type":          "response.output_text.delta",
							"delta":         syntheticText,
							"item_id":       itemID,
							"output_index":  0,
							"content_index": 0,
						})
					}
				}
			}
			finalItemID := itemID
			if currentItemID, _ := outputItem["id"].(string); strings.TrimSpace(currentItemID) != "" {
				finalItemID = currentItemID
			}
			_, _ = h.conversations.CreateRun(conversations.Run{
				ConversationID: conversationID,
				AccountID:      candidate.Account.ID,
				Model:          req.Model,
				Status:         "completed",
			})
			assistantMessage := conversations.Message{
				ConversationID: conversationID,
				Role:           "assistant",
				Content:        "",
				ItemType:       responseOutputItemType(outputItem),
				SequenceNo:     sequence,
			}
			for index, currentOutput := range outputItems {
				message := assistantMessage
				message.Content = outputItemText(currentOutput)
				if strings.TrimSpace(message.Content) == "" && index == 0 && responseOutputItemType(currentOutput) == "message" {
					message.Content = assistant.String()
				}
				message.ItemType = responseOutputItemType(currentOutput)
				if rawJSON, ok := marshalRawItem(currentOutput); ok {
					message.RawItemJSON = rawJSON
				}
				_ = h.conversations.AppendMessage(message)
			}
			if !isFunctionCallOutputItem(outputItem) {
				if err := ensureTextItemStarted(); err != nil {
					break
				}
				_ = writeEvent(map[string]any{
					"type":          "response.output_text.done",
					"text":          assistant.String(),
					"item_id":       finalItemID,
					"output_index":  0,
					"content_index": 0,
				})
				_ = writeEvent(map[string]any{
					"type":          "response.content_part.done",
					"item_id":       finalItemID,
					"output_index":  0,
					"content_index": 0,
					"part":          buildOutputTextPart(assistant.String()),
				})
			} else {
				_ = writeEvent(map[string]any{
					"type":         "response.output_item.added",
					"output_index": 0,
					"item":         cloneOutputItemWithStatus(outputItem, "in_progress"),
				})
				if arguments, _ := outputItem["arguments"].(string); arguments != "" {
					_ = writeEvent(map[string]any{
						"type":         "response.function_call_arguments.delta",
						"item_id":      finalItemID,
						"output_index": 0,
						"delta":        arguments,
					})
				}
				_ = writeEvent(map[string]any{
					"type":         "response.function_call_arguments.done",
					"item_id":      finalItemID,
					"output_index": 0,
					"arguments":    outputItem["arguments"],
					"name":         outputItem["name"],
					"call_id":      outputItem["call_id"],
				})
			}
			_ = writeEvent(map[string]any{
				"type":         "response.output_item.done",
				"output_index": 0,
				"item":         outputItem,
			})
			for outputIndex := 1; outputIndex < len(outputItems); outputIndex++ {
				currentOutput := outputItems[outputIndex]
				currentItemID, _ := currentOutput["id"].(string)
				if strings.TrimSpace(currentItemID) == "" {
					currentItemID = newResponseItemID()
					currentOutput["id"] = currentItemID
				}
				_ = writeEvent(map[string]any{
					"type":         "response.output_item.added",
					"output_index": outputIndex,
					"item":         cloneOutputItemWithStatus(currentOutput, "in_progress"),
				})
				if isFunctionCallOutputItem(currentOutput) {
					if arguments, _ := currentOutput["arguments"].(string); arguments != "" {
						_ = writeEvent(map[string]any{
							"type":         "response.function_call_arguments.delta",
							"item_id":      currentItemID,
							"output_index": outputIndex,
							"delta":        arguments,
						})
					}
					_ = writeEvent(map[string]any{
						"type":         "response.function_call_arguments.done",
						"item_id":      currentItemID,
						"output_index": outputIndex,
						"arguments":    currentOutput["arguments"],
						"name":         currentOutput["name"],
						"call_id":      currentOutput["call_id"],
					})
				} else {
					text := outputItemText(currentOutput)
					_ = writeEvent(map[string]any{
						"type":          "response.content_part.added",
						"item_id":       currentItemID,
						"output_index":  outputIndex,
						"content_index": 0,
						"part":          buildOutputTextPart(text),
					})
					_ = writeEvent(map[string]any{
						"type":          "response.output_text.done",
						"text":          text,
						"item_id":       currentItemID,
						"output_index":  outputIndex,
						"content_index": 0,
					})
					_ = writeEvent(map[string]any{
						"type":          "response.content_part.done",
						"item_id":       currentItemID,
						"output_index":  outputIndex,
						"content_index": 0,
						"part":          buildOutputTextPart(text),
					})
				}
				_ = writeEvent(map[string]any{
					"type":         "response.output_item.done",
					"output_index": outputIndex,
					"item":         currentOutput,
				})
			}
			_ = writeEvent(map[string]any{
				"type":     "response.completed",
				"response": buildResponsesResponse(responseID, req.Model, assistant.String(), "completed", itemID, resultSnapshot, outputItems),
			})
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			if flusher != nil {
				flusher.Flush()
			}
			return
		}
		_, _ = h.conversations.CreateRun(conversations.Run{
			ConversationID: conversationID,
			AccountID:      candidate.Account.ID,
			Model:          req.Model,
			Status:         runStatusForErrorClass(classifyRunError(err)),
		})
		lastRunErr = err
	}

	message := "no candidate succeeded"
	if lastRunErr != nil {
		message = lastRunErr.Error()
	}
	if strings.TrimSpace(assistant.String()) == "" && strings.TrimSpace(message) != "" {
		if err := ensureTextItemStarted(); err == nil {
			assistant.WriteString(message)
			_ = writeEvent(map[string]any{
				"type":          "response.output_text.delta",
				"delta":         message,
				"item_id":       itemID,
				"output_index":  0,
				"content_index": 0,
			})
		}
	}
	_ = writeEvent(map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    "server_error",
			"code":    "upstream_error",
			"message": message,
		},
	})
	terminal := buildResponsesResponse(responseID, req.Model, assistant.String(), "failed", itemID, resultSnapshot, nil)
	terminal["error"] = map[string]any{
		"type":    "server_error",
		"code":    "upstream_error",
		"message": message,
	}
	terminal["incomplete_details"] = map[string]any{
		"reason": "upstream_error",
	}
	_ = writeEvent(map[string]any{
		"type":     "response.completed",
		"response": terminal,
	})
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
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

func (h *ResponsesHandler) resolveConversation(r *http.Request, req gatewayopenai.ResponsesRequest) (int64, []conversations.Message, error) {
	if strings.TrimSpace(req.PreviousResponseID) == "" {
		return 0, nil, nil
	}

	conversationID, err := conversationIDFromResponseID(req.PreviousResponseID)
	if err != nil {
		return 0, nil, err
	}
	messages, err := h.conversations.ListMessages(conversationID)
	if err != nil {
		return 0, nil, err
	}
	return conversationID, messages, nil
}

func (h *ResponsesHandler) executeResponsesRequest(ctx context.Context, account accounts.Account, req gatewayopenai.ResponsesRequest, messages []conversations.Message) (responsesExecutionResult, error) {
	log.Printf("responses: forwarding request model=%q stream=false account_id=%d account_name=%q base_url=%q auth_mode=%q",
		req.Model, account.ID, account.AccountName, account.BaseURL, account.AuthMode)
	if err := ensureOfficialAccountSession(ctx, h.client, h.accounts, &account); err != nil {
		log.Printf("responses: forward failed account_id=%d err=%v", account.ID, err)
		return responsesExecutionResult{}, err
	}
	credential, err := resolveCredential(account)
	if err != nil {
		log.Printf("responses: forward failed account_id=%d err=%v", account.ID, err)
		return responsesExecutionResult{}, err
	}

	if usesOfficialCodexAdapter(account) {
		accountID, err := resolveLocalAccountID(account)
		if err != nil {
			return responsesExecutionResult{}, err
		}
		instructions := effectiveCodexInstructions(req.Instructions)
		body, err := json.Marshal(buildOfficialResponsesBody(req, messages, true, instructions))
		if err != nil {
			return responsesExecutionResult{}, err
		}
		adapter := providercodex.NewAdapter(resolveAccountBaseURL(account))
		httpReq, err := adapter.BuildResponsesRequest(ctx, credential, accountID, body, true)
		if err != nil {
			log.Printf("responses: forward failed account_id=%d err=%v", account.ID, err)
			return responsesExecutionResult{}, err
		}
		resp, err := h.client.Do(httpReq)
		if err != nil {
			log.Printf("responses: forward failed account_id=%d err=%v", account.ID, err)
			return responsesExecutionResult{}, err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			log.Printf("responses: upstream error account_id=%d status=%d", account.ID, resp.StatusCode)
			return responsesExecutionResult{}, classifyHTTPError(resp)
		}
		var builder strings.Builder
		collector := newResponsesUsageCollector(account.ID)
		if err := consumeResponsesStream(resp.Body, func(delta string) error {
			builder.WriteString(delta)
			return nil
		}, collector.Observe); err != nil {
			return responsesExecutionResult{}, err
		}
		collector.Save(h.usage)
		text := builder.String()
		if strings.TrimSpace(text) == "" {
			text = collector.outputText()
		}
		return responsesExecutionResult{
			Text:        text,
			Snapshot:    collector.snapshotOrDefault(),
			OutputItems: collector.outputItems(),
		}, nil
	}

	toolSummary := summarizeRequestedTools(req.Tools)
	if compatibleResult, fallback, fallbackReason, err := h.tryExecuteCompatibleResponsesRequest(ctx, account, req, messages, credential); !fallback {
		return compatibleResult, err
	} else {
		logResponsesDebug("non-stream fallback account_id=%d account_name=%q reason=%q tools=%s", account.ID, account.AccountName, fallbackReason, toolSummary.String())
	}
	if !account.AllowChatFallback {
		return responsesExecutionResult{}, errors.New("responses unsupported for this account and chat fallback is disabled")
	}
	if toolSummary.HasMCP {
		return responsesExecutionResult{}, errors.New("chat fallback does not support mcp tools; use /responses-compatible upstream or disable chat fallback")
	}

	body, err := json.Marshal(map[string]any{
		"model": req.Model,
	})
	if err != nil {
		log.Printf("responses: forward failed account_id=%d err=%v", account.ID, err)
		return responsesExecutionResult{}, err
	}
	body, err = json.Marshal(buildChatCompletionsBody(req, messages, false))
	if err != nil {
		return responsesExecutionResult{}, err
	}
	adapter := provideropenai.NewAdapter(resolveAccountBaseURL(account))
	httpReq, err := adapter.BuildRequest(ctx, providers.Request{
		Path:   "/chat/completions",
		Method: http.MethodPost,
		APIKey: credential,
		Body:   body,
	})
	if err != nil {
		log.Printf("responses: forward failed account_id=%d err=%v", account.ID, err)
		return responsesExecutionResult{}, err
	}
	resp, err := h.client.Do(httpReq)
	if err != nil {
		log.Printf("responses: forward failed account_id=%d err=%v", account.ID, err)
		return responsesExecutionResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.Printf("responses: upstream error account_id=%d status=%d", account.ID, resp.StatusCode)
		return responsesExecutionResult{}, classifyHTTPError(resp)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("responses: read upstream failed account_id=%d err=%v", account.ID, err)
		return responsesExecutionResult{}, err
	}
	text, outputItems := parseChatCompletionsResponse(raw)
	return responsesExecutionResult{
		Text:        text,
		Snapshot:    parseChatCompletionsUsage(raw, account.ID),
		OutputItems: outputItems,
	}, nil
}

func (h *ResponsesHandler) executeResponsesStreamRequest(ctx context.Context, account accounts.Account, req gatewayopenai.ResponsesRequest, messages []conversations.Message, emit func(string) error) (responsesExecutionResult, error) {
	log.Printf("responses: forwarding request model=%q stream=true account_id=%d account_name=%q base_url=%q auth_mode=%q",
		req.Model, account.ID, account.AccountName, account.BaseURL, account.AuthMode)
	if err := ensureOfficialAccountSession(ctx, h.client, h.accounts, &account); err != nil {
		log.Printf("responses: stream forward failed account_id=%d err=%v", account.ID, err)
		return responsesExecutionResult{}, err
	}
	credential, err := resolveCredential(account)
	if err != nil {
		log.Printf("responses: stream forward failed account_id=%d err=%v", account.ID, err)
		return responsesExecutionResult{}, err
	}

	if usesOfficialCodexAdapter(account) {
		accountID, err := resolveLocalAccountID(account)
		if err != nil {
			return responsesExecutionResult{}, err
		}
		instructions := effectiveCodexInstructions(req.Instructions)
		body, err := json.Marshal(buildOfficialResponsesBody(req, messages, true, instructions))
		if err != nil {
			return responsesExecutionResult{}, err
		}
		adapter := providercodex.NewAdapter(resolveAccountBaseURL(account))
		httpReq, err := adapter.BuildResponsesRequest(ctx, credential, accountID, body, true)
		if err != nil {
			log.Printf("responses: stream forward failed account_id=%d err=%v", account.ID, err)
			return responsesExecutionResult{}, err
		}
		resp, err := h.client.Do(httpReq)
		if err != nil {
			log.Printf("responses: stream forward failed account_id=%d err=%v", account.ID, err)
			return responsesExecutionResult{}, err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			log.Printf("responses: stream upstream error account_id=%d status=%d", account.ID, resp.StatusCode)
			return responsesExecutionResult{}, classifyHTTPError(resp)
		}
		collector := newResponsesUsageCollector(account.ID)
		err = consumeResponsesStream(resp.Body, emit, collector.Observe)
		if err == nil {
			collector.Save(h.usage)
		}
		return responsesExecutionResult{
			Snapshot:    collector.snapshotOrDefault(),
			OutputItems: collector.outputItems(),
		}, err
	}

	toolSummary := summarizeRequestedTools(req.Tools)
	if compatibleResult, fallback, fallbackReason, err := h.tryExecuteCompatibleResponsesStreamRequest(ctx, account, req, messages, emit, credential); !fallback {
		return compatibleResult, err
	} else {
		logResponsesDebug("stream fallback account_id=%d account_name=%q reason=%q tools=%s", account.ID, account.AccountName, fallbackReason, toolSummary.String())
	}
	if !account.AllowChatFallback {
		return responsesExecutionResult{}, errors.New("responses unsupported for this account and chat fallback is disabled")
	}
	if toolSummary.HasMCP {
		return responsesExecutionResult{}, errors.New("chat fallback does not support mcp tools; use /responses-compatible upstream or disable chat fallback")
	}

	body, err := json.Marshal(buildChatCompletionsBody(req, messages, true))
	if err != nil {
		log.Printf("responses: stream forward failed account_id=%d err=%v", account.ID, err)
		return responsesExecutionResult{}, err
	}
	adapter := provideropenai.NewAdapter(resolveAccountBaseURL(account))
	httpReq, err := adapter.BuildRequest(ctx, providers.Request{
		Path:   "/chat/completions",
		Method: http.MethodPost,
		APIKey: credential,
		Body:   body,
	})
	if err != nil {
		log.Printf("responses: stream forward failed account_id=%d err=%v", account.ID, err)
		return responsesExecutionResult{}, err
	}
	resp, err := h.client.Do(httpReq)
	if err != nil {
		log.Printf("responses: stream forward failed account_id=%d err=%v", account.ID, err)
		return responsesExecutionResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.Printf("responses: stream upstream error account_id=%d status=%d", account.ID, resp.StatusCode)
		return responsesExecutionResult{}, classifyHTTPError(resp)
	}
	outputItems, snapshot, err := consumeChatCompletionsStream(resp.Body, emit, account.ID)
	return responsesExecutionResult{
		Snapshot:    snapshot,
		OutputItems: outputItems,
	}, err
}

func (h *ResponsesHandler) tryExecuteCompatibleResponsesRequest(ctx context.Context, account accounts.Account, req gatewayopenai.ResponsesRequest, messages []conversations.Message, credential string) (responsesExecutionResult, bool, string, error) {
	body, err := json.Marshal(buildOfficialResponsesBody(req, messages, false, effectiveCodexInstructions(req.Instructions)))
	if err != nil {
		return responsesExecutionResult{}, false, "", err
	}
	adapter := provideropenai.NewAdapter(resolveAccountBaseURL(account))
	httpReq, err := adapter.BuildRequest(ctx, providers.Request{
		Path:   "/responses",
		Method: http.MethodPost,
		APIKey: credential,
		Body:   body,
	})
	if err != nil {
		return responsesExecutionResult{}, false, "", err
	}
	resp, err := h.client.Do(httpReq)
	if err != nil {
		// Some providers hard-close unsupported paths; fallback to chat/completions.
		return responsesExecutionResult{}, true, "responses_request_failed", nil
	}
	defer resp.Body.Close()
	if fallback, reason := isCompatibleResponsesFallback(resp.StatusCode, resp.Body); fallback {
		return responsesExecutionResult{}, true, reason, nil
	}
	if resp.StatusCode >= 400 {
		return responsesExecutionResult{}, false, "", classifyHTTPError(resp)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return responsesExecutionResult{}, false, "", err
	}
	if !looksLikeResponsesPayload(raw) {
		return responsesExecutionResult{}, true, "responses_payload_shape_mismatch", nil
	}
	return parseResponsesJSONResponse(raw, account.ID), false, "", nil
}

func (h *ResponsesHandler) tryExecuteCompatibleResponsesStreamRequest(ctx context.Context, account accounts.Account, req gatewayopenai.ResponsesRequest, messages []conversations.Message, emit func(string) error, credential string) (responsesExecutionResult, bool, string, error) {
	body, err := json.Marshal(buildOfficialResponsesBody(req, messages, true, effectiveCodexInstructions(req.Instructions)))
	if err != nil {
		return responsesExecutionResult{}, false, "", err
	}
	adapter := provideropenai.NewAdapter(resolveAccountBaseURL(account))
	httpReq, err := adapter.BuildRequest(ctx, providers.Request{
		Path:   "/responses",
		Method: http.MethodPost,
		APIKey: credential,
		Body:   body,
	})
	if err != nil {
		return responsesExecutionResult{}, false, "", err
	}
	resp, err := h.client.Do(httpReq)
	if err != nil {
		return responsesExecutionResult{}, true, "responses_request_failed", nil
	}
	defer resp.Body.Close()
	if fallback, reason := isCompatibleResponsesFallback(resp.StatusCode, resp.Body); fallback {
		return responsesExecutionResult{}, true, reason, nil
	}
	if resp.StatusCode >= 400 {
		return responsesExecutionResult{}, false, "", classifyHTTPError(resp)
	}
	collector := newResponsesUsageCollector(account.ID)
	err = consumeResponsesStream(resp.Body, emit, collector.Observe)
	if err == nil {
		collector.Save(h.usage)
	}
	return responsesExecutionResult{
		Snapshot:    collector.snapshotOrDefault(),
		OutputItems: collector.outputItems(),
	}, false, "", err
}

func isCompatibleResponsesFallback(status int, body io.Reader) (bool, string) {
	switch status {
	case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented:
		return true, fmt.Sprintf("responses_http_%d", status)
	case http.StatusBadRequest:
		raw, err := io.ReadAll(body)
		if err != nil {
			return false, ""
		}
		lower := strings.ToLower(string(raw))
		if strings.Contains(lower, "invalid url") || strings.Contains(lower, "not found") {
			return true, "responses_bad_request_invalid_url"
		}
		return false, ""
	default:
		return false, ""
	}
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

func buildChatCompletionsBody(req gatewayopenai.ResponsesRequest, messages []conversations.Message, stream bool) map[string]any {
	chatMessages := buildChatMessages(messages)
	if instructions := strings.TrimSpace(req.Instructions); instructions != "" {
		chatMessages = append([]map[string]any{
			{
				"role":    "system",
				"content": instructions,
			},
		}, chatMessages...)
	}
	body := map[string]any{
		"model":    req.Model,
		"stream":   stream,
		"messages": chatMessages,
	}
	if stream {
		body["stream_options"] = map[string]any{
			"include_usage": true,
		}
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
	if req.MaxOutputTokens != nil {
		body["max_completion_tokens"] = *req.MaxOutputTokens
		body["max_tokens"] = *req.MaxOutputTokens
	}
	if value, ok := decodeRawJSON(req.Metadata); ok {
		body["metadata"] = value
	}
	if value, ok := decodeRawJSON(req.Include); ok {
		body["include"] = value
	}
	if value, ok := decodeRawJSON(req.Reasoning); ok {
		body["reasoning"] = value
		if reasoning, ok := value.(map[string]any); ok {
			if effort, ok := reasoning["effort"].(string); ok && strings.TrimSpace(effort) != "" {
				body["reasoning_effort"] = effort
			}
		}
	}
	if value, ok := extractResponseFormatFromText(req.Text); ok {
		body["response_format"] = value
	}
	return body
}

func extractResponseFormatFromText(raw json.RawMessage) (any, bool) {
	value, ok := decodeRawJSON(raw)
	if !ok {
		return nil, false
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	format, ok := obj["format"]
	if !ok {
		return nil, false
	}
	return format, true
}

func buildChatMessages(messages []conversations.Message) []map[string]any {
	items := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		rawItem, hasRawItem := unmarshalRawItem(message.RawItemJSON)
		switch resolvedItemType(message, rawItem, hasRawItem) {
		case "message":
			content := strings.TrimSpace(message.Content)
			if hasRawItem {
				content = strings.TrimSpace(extractChatMessageContent(rawItem))
			}
			if content == "" {
				continue
			}
			role := message.Role
			if hasRawItem {
				if rawRole, _ := rawItem["role"].(string); strings.TrimSpace(rawRole) != "" {
					role = rawRole
				}
			}
			items = append(items, map[string]any{
				"role":    normalizeChatRole(role),
				"content": content,
			})
		case "function_call":
			callID := ""
			name := ""
			arguments := ""
			if hasRawItem {
				callID, _ = rawItem["call_id"].(string)
				name, _ = rawItem["name"].(string)
				arguments, _ = rawItem["arguments"].(string)
			}
			if strings.TrimSpace(name) == "" {
				continue
			}
			items = append(items, map[string]any{
				"role":    "assistant",
				"content": "",
				"tool_calls": []map[string]any{
					{
						"id":   callID,
						"type": "function",
						"function": map[string]any{
							"name":      name,
							"arguments": arguments,
						},
					},
				},
			})
		case "function_call_output":
			callID := ""
			output := strings.TrimSpace(message.Content)
			if hasRawItem {
				callID, _ = rawItem["call_id"].(string)
				if rawOutput, _ := rawItem["output"].(string); strings.TrimSpace(rawOutput) != "" {
					output = rawOutput
				}
			}
			if strings.TrimSpace(output) == "" {
				continue
			}
			items = append(items, map[string]any{
				"role":         "tool",
				"tool_call_id": callID,
				"content":      output,
			})
		}
	}
	return items
}

func buildResponsesResponse(id string, model string, text string, status string, itemID string, snapshot usage.Snapshot, outputItems []map[string]any) map[string]any {
	if len(outputItems) == 0 {
		outputItems = []map[string]any{buildOutputItem(itemID, text, status)}
	}
	if strings.TrimSpace(text) == "" {
		text = outputItemsText(outputItems)
	}
	return map[string]any{
		"id":                 id,
		"object":             "response",
		"created_at":         time.Now().Unix(),
		"status":             status,
		"error":              nil,
		"incomplete_details": nil,
		"model":              model,
		"store":              false,
		"output_text":        text,
		"output":             outputItems,
		"usage":              buildResponsesUsagePayload(snapshot),
	}
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

func cloneOutputItemWithStatus(item map[string]any, status string) map[string]any {
	cloned := cloneJSONMap(item)
	if cloned == nil {
		return nil
	}
	cloned["status"] = status
	return cloned
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

func newRouterResponseID(conversationID int64) string {
	return fmt.Sprintf("resp_%d_%d", conversationID, time.Now().UnixNano())
}

func newRouterResponseIDForSequence(conversationID int64, sequence int) string {
	return fmt.Sprintf("resp_%d_seq_%d_%d", conversationID, sequence, time.Now().UnixNano())
}

func conversationIDFromResponseID(value string) (int64, error) {
	parts := strings.Split(value, "_")
	if len(parts) < 3 || parts[0] != "resp" {
		return 0, errors.New("invalid previous_response_id")
	}
	return strconv.ParseInt(parts[1], 10, 64)
}

func responseSequenceFromResponseID(value string) (int, bool) {
	parts := strings.Split(value, "_")
	if len(parts) >= 5 && parts[0] == "resp" && parts[2] == "seq" {
		sequence, err := strconv.Atoi(parts[3])
		if err == nil {
			return sequence, true
		}
	}
	return 0, false
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

func dedupePrefix(next, existing string) string {
	if next == "" {
		return ""
	}
	if strings.HasPrefix(next, existing) {
		return strings.TrimPrefix(next, existing)
	}
	return next
}

func consumeChatCompletionsStream(body io.Reader, emit func(string) error, accountID int64) ([]map[string]any, usage.Snapshot, error) {
	scanner := bufio.NewScanner(body)
	collector := newChatCompletionsStreamCollector()
	snapshot := emptyResponsesUsageSnapshot()
	snapshot.AccountID = accountID
	sawDone := false
	frameCount := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			sawDone = true
			return collector.outputItems(), snapshot, nil
		}
		var frame map[string]any
		if err := json.Unmarshal([]byte(payload), &frame); err != nil {
			return nil, snapshot, err
		}
		frameCount++
		applyChatCompletionsUsageFrame(&snapshot, frame)
		choices, _ := frame["choices"].([]any)
		if len(choices) == 0 {
			continue
		}
		firstChoice, _ := choices[0].(map[string]any)
		delta, _ := firstChoice["delta"].(map[string]any)
		if chunk, ok := delta["content"].(string); ok && chunk != "" {
			if err := emit(chunk); err != nil {
				return nil, snapshot, err
			}
		}
		collector.observe(delta)
	}
	if err := scanner.Err(); err != nil {
		return nil, snapshot, err
	}
	if !sawDone {
		return nil, snapshot, fmt.Errorf("stream closed before [DONE] (frames=%d)", frameCount)
	}
	return collector.outputItems(), snapshot, nil
}

type chatCompletionsStreamCollector struct {
	calls map[int]*streamToolCall
}

type streamToolCall struct {
	index     int
	callID    string
	name      string
	arguments strings.Builder
}

func newChatCompletionsStreamCollector() *chatCompletionsStreamCollector {
	return &chatCompletionsStreamCollector{
		calls: map[int]*streamToolCall{},
	}
}

func (c *chatCompletionsStreamCollector) observe(delta map[string]any) {
	toolCalls, ok := delta["tool_calls"].([]any)
	if !ok {
		return
	}
	for _, rawCall := range toolCalls {
		call, ok := rawCall.(map[string]any)
		if !ok {
			continue
		}
		index := asInt(call["index"])
		current := c.calls[index]
		if current == nil {
			current = &streamToolCall{index: index}
			c.calls[index] = current
		}
		if current.callID == "" {
			current.callID, _ = call["id"].(string)
		}
		functionPayload, ok := call["function"].(map[string]any)
		if !ok {
			continue
		}
		if current.name == "" {
			current.name, _ = functionPayload["name"].(string)
		}
		if arguments, ok := functionPayload["arguments"].(string); ok {
			current.arguments.WriteString(arguments)
		}
	}
}

func (c *chatCompletionsStreamCollector) outputItems() []map[string]any {
	if len(c.calls) == 0 {
		return nil
	}
	indexes := make([]int, 0, len(c.calls))
	for index := range c.calls {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	items := make([]map[string]any, 0, len(indexes))
	for _, index := range indexes {
		call := c.calls[index]
		if call == nil || strings.TrimSpace(call.name) == "" {
			continue
		}
		items = append(items, map[string]any{
			"id":        newResponseItemID(),
			"type":      "function_call",
			"call_id":   call.callID,
			"name":      call.name,
			"arguments": call.arguments.String(),
			"status":    "completed",
		})
	}
	return items
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

func looksLikeResponsesPayload(raw []byte) bool {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false
	}
	if object, _ := payload["object"].(string); object == "response" {
		return true
	}
	if _, ok := payload["output"]; ok {
		return true
	}
	if _, ok := payload["output_text"]; ok {
		return true
	}
	return false
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

func resolvedItemType(message conversations.Message, rawItem map[string]any, hasRawItem bool) string {
	if hasRawItem {
		if itemType, _ := rawItem["type"].(string); strings.TrimSpace(itemType) != "" {
			return itemType
		}
	}
	if strings.TrimSpace(message.ItemType) != "" {
		return message.ItemType
	}
	return "message"
}

func extractChatMessageContent(rawItem map[string]any) string {
	if text, _ := rawItem["text"].(string); strings.TrimSpace(text) != "" {
		return text
	}
	content, ok := rawItem["content"].([]any)
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
			continue
		}
		if textPayload, ok := part["text"].(map[string]any); ok {
			if value, _ := textPayload["value"].(string); strings.TrimSpace(value) != "" {
				parts = append(parts, value)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func normalizeChatRole(role string) string {
	switch role {
	case "system", "assistant", "user", "tool":
		return role
	case "developer":
		return "system"
	default:
		return "user"
	}
}

func isResponseDetailPath(path string) bool {
	return strings.HasPrefix(path, "/v1/responses/") || strings.HasPrefix(path, "/responses/")
}

func isResponsesCompactPath(path string) bool {
	return path == "/v1/responses/compact" || path == "/responses/compact"
}

func isResponseInputItemsPath(path string) bool {
	return strings.HasSuffix(path, "/input_items") && isResponseDetailPath(path)
}

func isResponseCancelPath(path string) bool {
	return strings.HasSuffix(path, "/cancel") && isResponseDetailPath(path)
}

func isResponsesInputTokensPath(path string) bool {
	return path == "/v1/responses/input_tokens" || path == "/responses/input_tokens"
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

func pathDir(path string) string {
	index := strings.LastIndex(path, "/")
	if index <= 0 {
		return path
	}
	return path[:index]
}

func (h *ResponsesHandler) buildStoredResponse(responseID string) (map[string]any, error) {
	conversationID, err := conversationIDFromResponseID(responseID)
	if err != nil {
		return nil, err
	}
	messages, err := h.conversations.ListMessages(conversationID)
	if err != nil {
		return nil, err
	}
	targets := responseOutputMessages(messages, responseID)
	if len(targets) == 0 {
		return nil, errors.New("response not found")
	}

	snapshot, err := h.latestConversationUsage(messages, conversationID)
	if err != nil {
		snapshot = emptyResponsesUsageSnapshot()
	}
	outputItems := make([]map[string]any, 0, len(targets))
	textParts := make([]string, 0, len(targets))
	for _, target := range targets {
		if rawItem, ok := unmarshalRawItem(target.RawItemJSON); ok {
			outputItems = append(outputItems, rawItem)
		}
		if shouldAggregateFinalOutputText(target) && strings.TrimSpace(target.Content) != "" {
			textParts = append(textParts, target.Content)
		}
	}
	return buildResponsesResponse(responseID, "gpt-5.4", strings.Join(textParts, "\n"), "completed", buildResponseItemID(responseID), snapshot, outputItems), nil
}

func (h *ResponsesHandler) buildStoredInputItems(responseID string) ([]map[string]any, error) {
	conversationID, err := conversationIDFromResponseID(responseID)
	if err != nil {
		return nil, err
	}
	messages, err := h.conversations.ListMessages(conversationID)
	if err != nil {
		return nil, err
	}
	targets := responseOutputMessages(messages, responseID)
	if len(targets) == 0 {
		return nil, errors.New("response not found")
	}
	targetSequence := targets[0].SequenceNo
	items := make([]map[string]any, 0)
	for _, message := range messages {
		if message.SequenceNo >= targetSequence {
			break
		}
		if rawItem, ok := unmarshalRawItem(message.RawItemJSON); ok {
			items = append(items, wrapStoredInputItem(message, rawItem))
			continue
		}
		items = append(items, wrapStoredInputItem(message, map[string]any{
			"type": "message",
			"role": message.Role,
			"content": []map[string]any{
				{
					"type": "input_text",
					"text": message.Content,
				},
			},
		}))
	}
	return items, nil
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

func wrapStoredInputItem(message conversations.Message, item map[string]any) map[string]any {
	itemType, _ := item["type"].(string)
	if strings.TrimSpace(itemType) == "" {
		itemType = "message"
	}
	role, _ := item["role"].(string)
	if strings.TrimSpace(role) == "" {
		role = message.Role
	}
	return map[string]any{
		"id":      fmt.Sprintf("msg_input_%d_%d", message.ConversationID, message.SequenceNo),
		"object":  "response.input_item",
		"content": item["content"],
		"role":    role,
		"type":    itemType,
		"call_id": item["call_id"],
		"output":  item["output"],
	}
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

func shouldAggregateFinalOutputText(message conversations.Message) bool {
	itemType := strings.TrimSpace(message.ItemType)
	if itemType == "" {
		return true
	}
	return itemType == "message"
}

func isFunctionCallOutputItem(item map[string]any) bool {
	itemType, _ := item["type"].(string)
	return itemType == "function_call"
}

func firstOutputItem(items []map[string]any) map[string]any {
	if len(items) == 0 {
		return nil
	}
	return items[0]
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

type inputItemsPage struct {
	Items   []map[string]any
	FirstID string
	LastID  string
	HasMore bool
}

func paginateInputItems(items []map[string]any, query url.Values) inputItemsPage {
	paged := append([]map[string]any{}, items...)
	if strings.EqualFold(query.Get("order"), "desc") {
		reverseInputItems(paged)
	}

	after := strings.TrimSpace(query.Get("after"))
	if after != "" {
		index := indexOfInputItem(paged, after)
		if index >= 0 && index+1 <= len(paged) {
			paged = paged[index+1:]
		}
	}

	hasMore := false
	if limit, err := strconv.Atoi(strings.TrimSpace(query.Get("limit"))); err == nil && limit > 0 && limit < len(paged) {
		hasMore = true
		paged = paged[:limit]
	}

	page := inputItemsPage{
		Items:   paged,
		HasMore: hasMore,
	}
	if len(paged) > 0 {
		page.FirstID, _ = paged[0]["id"].(string)
		page.LastID, _ = paged[len(paged)-1]["id"].(string)
	}
	return page
}

func reverseInputItems(items []map[string]any) {
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
}

func indexOfInputItem(items []map[string]any, id string) int {
	for index, item := range items {
		if current, _ := item["id"].(string); current == id {
			return index
		}
	}
	return -1
}

func parseChatCompletionsResponse(raw []byte) (string, []map[string]any) {
	var payload struct {
		Choices []struct {
			Message map[string]any `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || len(payload.Choices) == 0 {
		text := strings.TrimSpace(string(raw))
		return text, []map[string]any{buildOutputItem(newResponseItemID(), text, "completed")}
	}
	message := payload.Choices[0].Message
	text, _ := message["content"].(string)
	if toolCallItems := buildFunctionCallOutputItems(message); len(toolCallItems) > 0 {
		return text, toolCallItems
	}
	return text, []map[string]any{buildOutputItem(newResponseItemID(), text, "completed")}
}

func buildFunctionCallOutputItems(message map[string]any) []map[string]any {
	toolCalls, ok := message["tool_calls"].([]any)
	if !ok || len(toolCalls) == 0 {
		return nil
	}
	items := make([]map[string]any, 0, len(toolCalls))
	for _, rawCall := range toolCalls {
		call, ok := rawCall.(map[string]any)
		if !ok {
			continue
		}
		functionPayload, ok := call["function"].(map[string]any)
		if !ok {
			continue
		}
		name, _ := functionPayload["name"].(string)
		arguments, _ := functionPayload["arguments"].(string)
		callID, _ := call["id"].(string)
		if strings.TrimSpace(name) == "" {
			continue
		}
		items = append(items, map[string]any{
			"id":        newResponseItemID(),
			"type":      "function_call",
			"call_id":   callID,
			"name":      name,
			"arguments": arguments,
			"status":    "completed",
		})
	}
	return items
}

func responseOutputMessages(messages []conversations.Message, responseID string) []conversations.Message {
	if sequence, ok := responseSequenceFromResponseID(responseID); ok {
		items := make([]conversations.Message, 0)
		for i := range messages {
			if messages[i].Role == "assistant" && messages[i].SequenceNo == sequence {
				items = append(items, messages[i])
			}
		}
		if len(items) > 0 {
			return items
		}
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			sequence := messages[i].SequenceNo
			items := make([]conversations.Message, 0)
			for j := range messages {
				if messages[j].Role == "assistant" && messages[j].SequenceNo == sequence {
					items = append(items, messages[j])
				}
			}
			return items
		}
	}
	return nil
}

func (h *ResponsesHandler) latestConversationUsage(messages []conversations.Message, conversationID int64) (usage.Snapshot, error) {
	accountList, err := h.accounts.List()
	if err != nil {
		return usage.Snapshot{}, err
	}
	var latest usage.Snapshot
	found := false
	for _, account := range accountList {
		snapshot, err := h.usage.GetLatest(account.ID)
		if err != nil || snapshot.LastTotalTokens <= 0 {
			continue
		}
		if !found || snapshot.CheckedAt.After(latest.CheckedAt) {
			latest = snapshot
			found = true
		}
	}
	if found {
		return latest, nil
	}
	return emptyResponsesUsageSnapshot(), nil
}

func applyChatCompletionsUsageFrame(snapshot *usage.Snapshot, frame map[string]any) {
	if snapshot == nil {
		return
	}
	usagePayload, ok := frame["usage"].(map[string]any)
	if !ok {
		return
	}
	snapshot.LastInputTokens = asFloat(usagePayload["prompt_tokens"])
	snapshot.LastOutputTokens = asFloat(usagePayload["completion_tokens"])
	snapshot.LastTotalTokens = asFloat(usagePayload["total_tokens"])
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
