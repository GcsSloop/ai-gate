package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/auth"
	"github.com/gcssloop/codex-router/backend/internal/providers"
	providercodex "github.com/gcssloop/codex-router/backend/internal/providers/codex"
	provideropenai "github.com/gcssloop/codex-router/backend/internal/providers/openai"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

const officialOpenAIBaseURL = "https://api.openai.com/v1"

type AccountsHandler struct {
	repo       accounts.Repository
	usage      AccountsUsage
	connector  *auth.OAuthConnector
	stateStore *auth.StateStore
	client     *http.Client
}

type AccountsUsage interface {
	ListLatest() ([]usage.Snapshot, error)
	Save(snapshot usage.Snapshot) error
}

func NewAccountsHandler(repo accounts.Repository, usage AccountsUsage, connector *auth.OAuthConnector, stateStore *auth.StateStore) *AccountsHandler {
	return &AccountsHandler{repo: repo, usage: usage, connector: connector, stateStore: stateStore, client: http.DefaultClient}
}

func (h *AccountsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/accounts":
		h.createAccount(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/accounts/usage":
		h.listAccountsUsage(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/accounts":
		h.listAccounts(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/accounts/auth/authorize":
		h.createAuthSession(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/accounts/import-local":
		h.importLocalAuth(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/accounts/import-current":
		h.importCurrentAuth(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/accounts/") && countPathSegments(r.URL.Path) == 2:
		h.updateAccount(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/accounts/") && countPathSegments(r.URL.Path) == 2:
		h.deleteAccount(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/accounts/") && strings.HasSuffix(r.URL.Path, "/test"):
		h.testAccount(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/accounts/") && strings.HasSuffix(r.URL.Path, "/disable"):
		h.disableAccount(w, r)
	default:
		http.NotFound(w, r)
	}
}

type createAccountRequest struct {
	ProviderType      accounts.ProviderType `json:"provider_type"`
	AccountName       string                `json:"account_name"`
	AuthMode          accounts.AuthMode     `json:"auth_mode"`
	BaseURL           string                `json:"base_url"`
	CredentialRef     string                `json:"credential_ref"`
	SupportsResponses *bool                 `json:"supports_responses"`
}

type importLocalAuthRequest struct {
	AccountName string `json:"account_name"`
	Content     string `json:"content"`
	Path        string `json:"path"`
}

type importCurrentAuthRequest struct {
	AccountName string `json:"account_name"`
}

type updateAccountRequest struct {
	AccountName       string          `json:"account_name"`
	BaseURL           string          `json:"base_url"`
	CredentialRef     string          `json:"credential_ref"`
	Status            accounts.Status `json:"status"`
	Priority          *int            `json:"priority"`
	IsActive          *bool           `json:"is_active"`
	SupportsResponses *bool           `json:"supports_responses"`
}

type accountTestResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
	Content string `json:"content,omitempty"`
}

type accountChatTestRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

func (h *AccountsHandler) createAccount(w http.ResponseWriter, r *http.Request) {
	var req createAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	supportsResponses := true
	if req.SupportsResponses != nil {
		supportsResponses = *req.SupportsResponses
	}
	if req.ProviderType == accounts.ProviderOpenAIOfficial {
		supportsResponses = true
	}

	err := h.repo.Create(accounts.Account{
		ProviderType:      req.ProviderType,
		AccountName:       req.AccountName,
		AuthMode:          req.AuthMode,
		BaseURL:           req.BaseURL,
		CredentialRef:     req.CredentialRef,
		Status:            accounts.StatusActive,
		SupportsResponses: supportsResponses,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *AccountsHandler) listAccounts(w http.ResponseWriter, _ *http.Request) {
	accountList, err := h.repo.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type responseItem struct {
		ID                       int64                 `json:"id"`
		ProviderType             accounts.ProviderType `json:"provider_type"`
		AccountName              string                `json:"account_name"`
		AuthMode                 accounts.AuthMode     `json:"auth_mode"`
		BaseURL                  string                `json:"base_url"`
		Status                   accounts.Status       `json:"status"`
		CooldownRemainingSeconds *int64                `json:"cooldown_remaining_seconds,omitempty"`
		Balance                  float64               `json:"balance"`
		QuotaRemaining           float64               `json:"quota_remaining"`
		RPMRemaining             float64               `json:"rpm_remaining"`
		TPMRemaining             float64               `json:"tpm_remaining"`
		HealthScore              float64               `json:"health_score"`
		RecentErrorRate          float64               `json:"recent_error_rate"`
		LastTotalTokens          float64               `json:"last_total_tokens"`
		LastInputTokens          float64               `json:"last_input_tokens"`
		LastOutputTokens         float64               `json:"last_output_tokens"`
		ModelContextWindow       float64               `json:"model_context_window"`
		PrimaryUsedPercent       float64               `json:"primary_used_percent"`
		SecondaryUsedPercent     float64               `json:"secondary_used_percent"`
		PrimaryResetsAt          *time.Time            `json:"primary_resets_at,omitempty"`
		SecondaryResetsAt        *time.Time            `json:"secondary_resets_at,omitempty"`
		Priority                 int                   `json:"priority"`
		IsActive                 bool                  `json:"is_active"`
		SupportsResponses        bool                  `json:"supports_responses"`
	}

	response := make([]responseItem, 0, len(accountList))
	now := time.Now().UTC()
	for _, account := range accountList {
		item := responseItem{
			ID:                   account.ID,
			ProviderType:         account.ProviderType,
			AccountName:          account.AccountName,
			AuthMode:             account.AuthMode,
			BaseURL:              account.BaseURL,
			Status:               account.Status,
			Priority:             account.Priority,
			IsActive:             account.IsActive,
			SupportsResponses:    account.SupportsResponses,
			Balance:              0,
			QuotaRemaining:       0,
			RPMRemaining:         0,
			TPMRemaining:         0,
			HealthScore:          0,
			RecentErrorRate:      0,
			LastTotalTokens:      0,
			LastInputTokens:      0,
			LastOutputTokens:     0,
			ModelContextWindow:   0,
			PrimaryUsedPercent:   0,
			SecondaryUsedPercent: 0,
		}
		if account.CooldownUntil != nil {
			remaining := int64(account.CooldownUntil.Sub(now).Seconds())
			if remaining < 0 {
				remaining = 0
			}
			item.CooldownRemainingSeconds = &remaining
		}
		response = append(response, item)
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *AccountsHandler) listAccountsUsage(w http.ResponseWriter, _ *http.Request) {
	accountList, err := h.repo.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.refreshOfficialUsage(context.Background(), accountList)

	type responseItem struct {
		AccountID            int64      `json:"account_id"`
		Balance              float64    `json:"balance"`
		QuotaRemaining       float64    `json:"quota_remaining"`
		RPMRemaining         float64    `json:"rpm_remaining"`
		TPMRemaining         float64    `json:"tpm_remaining"`
		HealthScore          float64    `json:"health_score"`
		RecentErrorRate      float64    `json:"recent_error_rate"`
		LastTotalTokens      float64    `json:"last_total_tokens"`
		LastInputTokens      float64    `json:"last_input_tokens"`
		LastOutputTokens     float64    `json:"last_output_tokens"`
		ModelContextWindow   float64    `json:"model_context_window"`
		PrimaryUsedPercent   float64    `json:"primary_used_percent"`
		SecondaryUsedPercent float64    `json:"secondary_used_percent"`
		PrimaryResetsAt      *time.Time `json:"primary_resets_at,omitempty"`
		SecondaryResetsAt    *time.Time `json:"secondary_resets_at,omitempty"`
	}

	usageByAccount := map[int64]usage.Snapshot{}
	if h.usage != nil {
		if snapshots, err := h.usage.ListLatest(); err == nil {
			for _, snapshot := range snapshots {
				usageByAccount[snapshot.AccountID] = snapshot
			}
		}
	}

	response := make([]responseItem, 0, len(accountList))
	for _, account := range accountList {
		snapshot := usageByAccount[account.ID]
		response = append(response, responseItem{
			AccountID:            account.ID,
			Balance:              snapshot.Balance,
			QuotaRemaining:       snapshot.QuotaRemaining,
			RPMRemaining:         snapshot.RPMRemaining,
			TPMRemaining:         snapshot.TPMRemaining,
			HealthScore:          snapshot.HealthScore,
			RecentErrorRate:      snapshot.RecentErrorRate,
			LastTotalTokens:      snapshot.LastTotalTokens,
			LastInputTokens:      snapshot.LastInputTokens,
			LastOutputTokens:     snapshot.LastOutputTokens,
			ModelContextWindow:   snapshot.ModelContextWindow,
			PrimaryUsedPercent:   snapshot.PrimaryUsedPercent,
			SecondaryUsedPercent: snapshot.SecondaryUsedPercent,
			PrimaryResetsAt:      snapshot.PrimaryResetsAt,
			SecondaryResetsAt:    snapshot.SecondaryResetsAt,
		})
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *AccountsHandler) refreshOfficialUsage(ctx context.Context, accountList []accounts.Account) {
	if h.usage == nil {
		return
	}
	for i := range accountList {
		account := &accountList[i]
		if !usesOfficialCodexAdapter(*account) {
			continue
		}
		if err := ensureOfficialAccountSession(ctx, h.client, h.repo, account); err != nil {
			continue
		}
		credential, err := resolveCredential(*account)
		if err != nil {
			continue
		}
		accountID, err := resolveLocalAccountID(*account)
		if err != nil {
			continue
		}
		req, err := providercodex.NewAdapter(resolveAccountBaseURL(*account)).BuildUsageRequest(ctx, credential, accountID)
		if err != nil {
			continue
		}
		resp, err := h.client.Do(req)
		if err != nil {
			continue
		}
		raw, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil || resp.StatusCode >= 400 {
			continue
		}
		snapshot, ok := parseOfficialUsageSnapshot(raw)
		if !ok {
			continue
		}
		snapshot.AccountID = account.ID
		snapshot.CheckedAt = time.Now().UTC()
		_ = h.usage.Save(snapshot)
	}
}

func (h *AccountsHandler) createAuthSession(w http.ResponseWriter, _ *http.Request) {
	authURL, state, err := h.connector.AuthorizationURL()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := h.stateStore.New(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"authorization_url": authURL,
		"state":             state,
	})
}

func (h *AccountsHandler) importLocalAuth(w http.ResponseWriter, r *http.Request) {
	accountName, raw, err := decodeLocalImportRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if accountName == "" {
		accountName = "local-codex"
	}

	err = h.repo.Create(accounts.Account{
		ProviderType:      accounts.ProviderOpenAIOfficial,
		AccountName:       accountName,
		AuthMode:          accounts.AuthModeLocalImport,
		CredentialRef:     string(raw),
		BaseURL:           officialCodexBaseURL,
		Status:            accounts.StatusActive,
		SupportsResponses: true,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *AccountsHandler) importCurrentAuth(w http.ResponseWriter, r *http.Request) {
	var req importCurrentAuthRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	accountName := strings.TrimSpace(req.AccountName)
	if accountName == "" {
		accountName = "local-codex"
	}

	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	authPath := filepath.Join(home, ".codex", "auth.json")
	_, raw, err := auth.LoadLocalAuthFile(authPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err = h.repo.Create(accounts.Account{
		ProviderType:      accounts.ProviderOpenAIOfficial,
		AccountName:       accountName,
		AuthMode:          accounts.AuthModeLocalImport,
		CredentialRef:     string(raw),
		BaseURL:           officialCodexBaseURL,
		Status:            accounts.StatusActive,
		SupportsResponses: true,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *AccountsHandler) updateAccount(w http.ResponseWriter, r *http.Request) {
	id, err := accountIDFromPath(strings.TrimSuffix(r.URL.Path, "/"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	current, err := h.repo.GetByID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	var req updateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.AccountName != "" {
		current.AccountName = req.AccountName
	}
	if req.BaseURL != "" {
		current.BaseURL = req.BaseURL
	}
	if req.CredentialRef != "" {
		current.CredentialRef = req.CredentialRef
	}
	if req.Status != "" {
		current.Status = req.Status
	}
	if req.Priority != nil {
		current.Priority = *req.Priority
	}
	if req.IsActive != nil {
		current.IsActive = *req.IsActive
		if *req.IsActive {
			if err := h.repo.SetActive(id); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			log.Printf("accounts: active account updated account_id=%d account_name=%q", current.ID, current.AccountName)
		}
	}
	if req.SupportsResponses != nil {
		current.SupportsResponses = *req.SupportsResponses
	}

	if err := h.repo.Update(current); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *AccountsHandler) testAccount(w http.ResponseWriter, r *http.Request) {
	id, err := accountIDFromPath(strings.TrimSuffix(strings.TrimSuffix(r.URL.Path, "/test"), "/"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	account, err := h.repo.GetByID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	credential, err := resolveCredential(account)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if usesOfficialCodexAdapter(account) {
		if err := ensureOfficialAccountSession(r.Context(), h.client, h.repo, &account); err != nil {
			writeJSON(w, http.StatusOK, accountTestResponse{OK: false, Message: "官方账户会话刷新失败", Details: err.Error()})
			return
		}
		credential, err = resolveCredential(account)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	var reqBody accountChatTestRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if reqBody.Model == "" {
		reqBody.Model = defaultTestModelForAccount(account)
	}
	if reqBody.Input == "" {
		reqBody.Input = "ping"
	}

	writeJSON(w, http.StatusOK, h.runAccountTest(r.Context(), account, credential, reqBody.Model, reqBody.Input))
}

func (h *AccountsHandler) runAccountTest(ctx context.Context, account accounts.Account, credential string, requestedModel string, input string) accountTestResponse {
	model := strings.TrimSpace(requestedModel)
	if model == "" {
		model = defaultTestModelForAccount(account)
	}

	if account.AuthMode == accounts.AuthModeLocalImport || account.ProviderType == accounts.ProviderOpenAIOfficial {
		return h.runResponsesTest(ctx, account, credential, model, input)
	}

	result := h.runChatCompletionTest(ctx, account, credential, model, input)
	if result.OK || strings.TrimSpace(requestedModel) != "" {
		return result
	}

	fallbackModel, err := h.discoverFallbackModel(ctx, account, credential, model)
	if err != nil || fallbackModel == "" || fallbackModel == model {
		return result
	}

	fallbackResult := h.runChatCompletionTest(ctx, account, credential, fallbackModel, input)
	if fallbackResult.OK {
		fallbackResult.Details = fmt.Sprintf("模型 %s 已返回响应（自动从 %s 切换）", fallbackModel, model)
	}
	return fallbackResult
}

func (h *AccountsHandler) runResponsesTest(ctx context.Context, account accounts.Account, credential string, model string, input string) accountTestResponse {
	if usesOfficialCodexAdapter(account) {
		accountID, err := resolveLocalAccountID(account)
		if err != nil {
			return accountTestResponse{OK: false, Message: "本地凭证缺少账户信息", Details: err.Error()}
		}
		body, err := json.Marshal(map[string]any{
			"model":  model,
			"stream": true,
			"store":  false,
			"input": []map[string]any{
				{
					"role": "user",
					"content": []map[string]any{
						{
							"type": "input_text",
							"text": input,
						},
					},
				},
			},
			"instructions": effectiveCodexInstructions(""),
		})
		if err != nil {
			return accountTestResponse{OK: false, Message: "构造测试请求失败", Details: err.Error()}
		}
		adapter := providercodex.NewAdapter(resolveAccountBaseURL(account))
		req, err := adapter.BuildResponsesRequest(ctx, credential, accountID, body, true)
		if err != nil {
			return accountTestResponse{OK: false, Message: "构造测试请求失败", Details: err.Error()}
		}
		resp, err := h.client.Do(req)
		if err != nil {
			return accountTestResponse{OK: false, Message: "请求上游失败", Details: err.Error()}
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			raw, _ := io.ReadAll(resp.Body)
			return accountTestResponse{
				OK:      false,
				Message: "上游测试失败",
				Details: buildUpstreamErrorDetails(resp.Status, raw),
				Content: strings.TrimSpace(string(raw)),
			}
		}
		collector := newResponsesUsageCollector(account.ID)
		var builder strings.Builder
		if err := consumeResponsesStream(resp.Body, func(delta string) error {
			builder.WriteString(delta)
			return nil
		}, collector.Observe); err != nil {
			return accountTestResponse{OK: false, Message: "读取上游流失败", Details: err.Error()}
		}
		collector.Save(h.usage)
		return accountTestResponse{
			OK:      true,
			Message: "OpenAI responses 测试成功",
			Details: "模型 " + model + " 已返回响应",
			Content: builder.String(),
		}
	}

	body, err := json.Marshal(map[string]any{
		"model": model,
		"input": input,
	})
	if err != nil {
		return accountTestResponse{OK: false, Message: "构造测试请求失败", Details: err.Error()}
	}

	req, err := providers.NewJSONRequest(ctx, http.MethodPost, strings.TrimRight(resolveAccountBaseURL(account), "/")+"/responses", credential, body)
	if err != nil {
		return accountTestResponse{OK: false, Message: "构造测试请求失败", Details: err.Error()}
	}
	return h.executeUpstreamTest(req, model, parseResponsesContent, "OpenAI responses 测试成功")
}

func (h *AccountsHandler) runChatCompletionTest(ctx context.Context, account accounts.Account, credential string, model string, input string) accountTestResponse {
	body, err := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": input},
		},
		"stream": false,
	})
	if err != nil {
		return accountTestResponse{OK: false, Message: "构造测试请求失败", Details: err.Error()}
	}

	adapter := provideropenai.NewAdapter(resolveAccountBaseURL(account))
	req, err := adapter.BuildRequest(ctx, providers.Request{
		Path:   "/chat/completions",
		Method: http.MethodPost,
		APIKey: credential,
		Body:   body,
	})
	if err != nil {
		return accountTestResponse{OK: false, Message: "构造测试请求失败", Details: err.Error()}
	}

	return h.executeUpstreamTest(req, model, parseChatCompletionsContent, "远端连通性测试成功")
}

func (h *AccountsHandler) executeUpstreamTest(req *http.Request, model string, parser func([]byte) string, successMessage string) accountTestResponse {
	resp, err := h.client.Do(req)
	if err != nil {
		return accountTestResponse{OK: false, Message: "请求上游失败", Details: err.Error()}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return accountTestResponse{OK: false, Message: "读取上游响应失败", Details: err.Error()}
	}

	if resp.StatusCode >= 400 {
		return accountTestResponse{
			OK:      false,
			Message: "上游测试失败",
			Details: buildUpstreamErrorDetails(resp.Status, raw),
			Content: strings.TrimSpace(string(raw)),
		}
	}

	return accountTestResponse{
		OK:      true,
		Message: successMessage,
		Details: "模型 " + model + " 已返回响应",
		Content: parser(raw),
	}
}

func (h *AccountsHandler) discoverFallbackModel(ctx context.Context, account accounts.Account, credential string, currentModel string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(resolveAccountBaseURL(account), "/")+"/models", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+credential)

	resp, err := h.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("upstream models request failed: %s", resp.Status)
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}

	available := make(map[string]struct{}, len(payload.Data))
	for _, model := range payload.Data {
		if strings.TrimSpace(model.ID) != "" {
			available[model.ID] = struct{}{}
		}
	}

	for _, candidate := range preferredTestModels(account) {
		if candidate == currentModel {
			continue
		}
		if _, ok := available[candidate]; ok {
			return candidate, nil
		}
	}
	for _, model := range payload.Data {
		if model.ID != "" && model.ID != currentModel {
			return model.ID, nil
		}
	}
	return "", nil
}

func defaultTestModelForAccount(account accounts.Account) string {
	if account.AuthMode == accounts.AuthModeLocalImport || account.ProviderType == accounts.ProviderOpenAIOfficial {
		return "gpt-5.4"
	}
	return "gpt-5.4"
}

func preferredTestModels(account accounts.Account) []string {
	if account.AuthMode == accounts.AuthModeLocalImport || account.ProviderType == accounts.ProviderOpenAIOfficial {
		return []string{"gpt-5.4", "gpt-5.2-codex", "gpt-5.1-codex-max", "gpt-4.1"}
	}
	return []string{"gpt-5.4", "gpt-5.2-codex", "gpt-5.1-codex-max", "gpt-4.1"}
}

func parseResponsesContent(raw []byte) string {
	var payload struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return strings.TrimSpace(string(raw))
	}
	if strings.TrimSpace(payload.OutputText) != "" {
		return payload.OutputText
	}
	for _, item := range payload.Output {
		for _, content := range item.Content {
			if strings.TrimSpace(content.Text) != "" {
				return content.Text
			}
		}
	}
	return strings.TrimSpace(string(raw))
}

func parseChatCompletionsContent(raw []byte) string {
	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return strings.TrimSpace(string(raw))
	}
	if len(payload.Choices) > 0 {
		return payload.Choices[0].Message.Content
	}
	return strings.TrimSpace(string(raw))
}

func buildUpstreamErrorDetails(status string, raw []byte) string {
	body := strings.TrimSpace(string(raw))
	if body == "" {
		return "上游返回错误：" + status
	}
	return "上游返回错误：" + status + "\n" + body
}

func parseOfficialUsageSnapshot(raw []byte) (usage.Snapshot, bool) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return usage.Snapshot{}, false
	}

	rateLimit, _ := payload["rate_limit"].(map[string]any)
	primary, _ := rateLimit["primary_window"].(map[string]any)
	secondary, _ := rateLimit["secondary_window"].(map[string]any)
	credits, _ := payload["credits"].(map[string]any)

	primaryUsed := asFloat(primary["used_percent"])
	secondaryUsed := asFloat(secondary["used_percent"])
	balance := asFloat(credits["balance"])
	allowed, _ := rateLimit["allowed"].(bool)
	limitReached, _ := rateLimit["limit_reached"].(bool)
	hasCredits, _ := credits["has_credits"].(bool)
	unlimited, _ := credits["unlimited"].(bool)

	if primaryUsed == 0 &&
		secondaryUsed == 0 &&
		balance == 0 &&
		!allowed &&
		!hasCredits &&
		!unlimited {
		return usage.Snapshot{}, false
	}

	primaryRemaining := mathMax(100-primaryUsed, 0)
	secondaryRemaining := mathMax(100-secondaryUsed, 0)
	snapshot := usage.Snapshot{
		Balance:              balance,
		RPMRemaining:         primaryRemaining,
		TPMRemaining:         secondaryRemaining,
		PrimaryUsedPercent:   primaryUsed,
		SecondaryUsedPercent: secondaryUsed,
		PrimaryResetsAt:      unixSecondsPtr(int64(asFloat(primary["reset_at"]))),
		SecondaryResetsAt:    unixSecondsPtr(int64(asFloat(secondary["reset_at"]))),
		HealthScore:          (primaryRemaining + secondaryRemaining) / 200,
		ThrottledRecently:    limitReached || !allowed,
	}
	return snapshot, true
}

func unixSecondsPtr(seconds int64) *time.Time {
	if seconds <= 0 {
		return nil
	}
	value := time.Unix(seconds, 0).UTC()
	return &value
}

func mathMax(a float64, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func (h *AccountsHandler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	id, err := accountIDFromPath(strings.TrimSuffix(r.URL.Path, "/"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.repo.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AccountsHandler) disableAccount(w http.ResponseWriter, r *http.Request) {
	id, err := accountIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.repo.UpdateStatus(id, accounts.StatusDisabled); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func accountIDFromPath(path string) (int64, error) {
	trimmed := strings.TrimPrefix(path, "/accounts/")
	trimmed = strings.TrimSuffix(trimmed, "/disable")
	trimmed = strings.TrimSuffix(trimmed, "/test")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return 0, errors.New("missing account id")
	}
	return strconv.ParseInt(trimmed, 10, 64)
}

func countPathSegments(path string) int {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return 0
	}
	return len(strings.Split(trimmed, "/"))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func decodeLocalImportRequest(r *http.Request) (string, []byte, error) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			return "", nil, err
		}
		accountName := r.FormValue("account_name")
		file, _, err := r.FormFile("auth_file")
		if err != nil {
			return "", nil, err
		}
		defer file.Close()
		raw, err := io.ReadAll(file)
		if err != nil {
			return "", nil, err
		}
		if _, err := auth.LoadLocalAuthFileContent(raw); err != nil {
			return "", nil, err
		}
		return accountName, raw, nil
	}

	var req importLocalAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return "", nil, err
	}
	raw := []byte(req.Content)
	if len(bytes.TrimSpace(raw)) == 0 && strings.TrimSpace(req.Path) != "" {
		_, fileRaw, err := auth.LoadLocalAuthFile(req.Path)
		if err != nil {
			return "", nil, err
		}
		raw = fileRaw
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", nil, errors.New("missing auth.json content")
	}
	if _, err := auth.LoadLocalAuthFileContent(raw); err != nil {
		return "", nil, err
	}
	return req.AccountName, raw, nil
}

func resolveAccountBaseURL(account accounts.Account) string {
	if usesOfficialCodexAdapter(account) {
		baseURL := strings.TrimSpace(account.BaseURL)
		if baseURL == "" || baseURL == officialOpenAIBaseURL {
			return officialCodexBaseURL
		}
		return baseURL
	}
	if strings.TrimSpace(account.BaseURL) != "" {
		return account.BaseURL
	}
	return account.BaseURL
}
