package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/auth"
	"github.com/gcssloop/codex-router/backend/internal/conversations"
	gatewayopenai "github.com/gcssloop/codex-router/backend/internal/gateway/openai"
	"github.com/gcssloop/codex-router/backend/internal/providers"
	provideropenai "github.com/gcssloop/codex-router/backend/internal/providers/openai"
	"github.com/gcssloop/codex-router/backend/internal/routing"
	streamproxy "github.com/gcssloop/codex-router/backend/internal/streaming"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

type GatewayAccounts interface {
	List() ([]accounts.Account, error)
	Update(account accounts.Account) error
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
	if r.Method != http.MethodPost || (r.URL.Path != "/v1/chat/completions" && r.URL.Path != "/chat/completions") {
		http.NotFound(w, r)
		return
	}

	req, err := gatewayopenai.ParseChatCompletionRequest(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	logRequestSummary("gateway", r.URL.Path, r.Method, req.Model, r.RemoteAddr, summarizeChatRequestLog(req.Messages, req.Stream))

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
	log.Printf("gateway: created conversation conversation_id=%d model=%q", conversationID, req.Model)

	if req.Stream {
		h.serveStream(r.Context(), w, req, body, candidates, conversationID)
		return
	}

	var upstreamResponse []byte
	executor := routing.NewExecutor(h.conversations, func(ctx context.Context, candidate routing.Candidate) error {
		account := candidate.Account
		startedAt := time.Now()
		logUpstreamSummary("gateway", conversationID, account, "/chat/completions", req.Model)
		if err := ensureOfficialAccountSession(ctx, h.client, h.accounts, &account); err != nil {
			logFailureSummary("gateway", conversationID, account.ID, "ensure_session", startedAt, err)
			return err
		}
		credential, err := resolveCredential(account)
		if err != nil {
			logFailureSummary("gateway", conversationID, account.ID, "resolve_credential", startedAt, err)
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
			logFailureSummary("gateway", conversationID, account.ID, "build_request", startedAt, err)
			return err
		}

		resp, err := h.client.Do(upstreamReq)
		if err != nil {
			logFailureSummary("gateway", conversationID, account.ID, "upstream_request", startedAt, err)
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			logFailureSummary("gateway", conversationID, account.ID, "upstream_status", startedAt, providers.HTTPError{StatusCode: resp.StatusCode})
			return providers.HTTPError{StatusCode: resp.StatusCode}
		}

		upstreamResponse, err = io.ReadAll(resp.Body)
		if err != nil {
			logFailureSummary("gateway", conversationID, account.ID, "read_response", startedAt, err)
		} else {
			logResultSummary("gateway", conversationID, account.ID, resp.StatusCode, startedAt, string(upstreamResponse))
		}
		return err
	})

	err = executor.ExecuteNonStream(r.Context(), conversationID, req.Model, candidates, routing.TokenBudget{
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

func (h *GatewayHandler) serveStream(ctx context.Context, w http.ResponseWriter, req gatewayopenai.ChatCompletionRequest, body []byte, candidates []routing.Candidate, conversationID int64) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	writeChunk := func(delta string) error {
		payload := map[string]any{
			"choices": []map[string]any{
				{"delta": map[string]string{"content": delta}},
			},
		}
		data, err := json.Marshal(payload)
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

	proxy := streamproxy.NewProxy(h.conversations, func(ctx context.Context, attempt streamproxy.Attempt) error {
		account := attempt.Candidate.Account
		startedAt := time.Now()
		logUpstreamSummary("gateway", conversationID, account, "/chat/completions", req.Model)
		if err := ensureOfficialAccountSession(ctx, h.client, h.accounts, &account); err != nil {
			logFailureSummary("gateway", conversationID, account.ID, "ensure_session", startedAt, err)
			return err
		}
		credential, err := resolveCredential(account)
		if err != nil {
			logFailureSummary("gateway", conversationID, account.ID, "resolve_credential", startedAt, err)
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
			logFailureSummary("gateway", conversationID, account.ID, "build_request", startedAt, err)
			return err
		}

		resp, err := h.client.Do(upstreamReq)
		if err != nil {
			logFailureSummary("gateway", conversationID, account.ID, "upstream_request", startedAt, err)
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			logFailureSummary("gateway", conversationID, account.ID, "upstream_status", startedAt, providers.HTTPError{StatusCode: resp.StatusCode})
			return providers.HTTPError{StatusCode: resp.StatusCode}
		}

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			if !strings.HasPrefix(line, "data: ") {
				return errors.New("invalid stream frame")
			}
			payload := strings.TrimPrefix(line, "data: ")
			if payload == "[DONE]" {
				return nil
			}

			var frame struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(payload), &frame); err != nil {
				return err
			}
			if len(frame.Choices) == 0 {
				continue
			}
			if err := attempt.Emit(frame.Choices[0].Delta.Content); err != nil {
				return err
			}
		}
		if err := scanner.Err(); err != nil {
			logFailureSummary("gateway", conversationID, account.ID, "read_stream", startedAt, err)
			return err
		}
		logResultSummary("gateway", conversationID, account.ID, resp.StatusCode, startedAt, "")
		return nil
	})

	output, err := proxy.Execute(ctx, conversationID, req.Model, candidates, routing.TokenBudget{
		ProjectedInputTokens:  float64(len(req.Messages) * 500),
		ProjectedOutputTokens: 1500,
		SafetyFactor:          1.3,
		EstimatedCost:         0.01,
	})
	if err != nil {
		_ = writeChunk("[stream failed over and exhausted candidates]")
		return
	}
	_ = writeChunk(output)

	_, _ = w.Write([]byte("data: [DONE]\n\n"))
	if flusher != nil {
		flusher.Flush()
	}
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
