package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/api"
	"github.com/gcssloop/codex-router/backend/internal/conversations"
)

func TestConversationsHandler(t *testing.T) {
	t.Parallel()

	handler := api.NewConversationsHandler(conversationQuery{
		conversations: []conversations.Conversation{
			{ID: 1, ClientID: "client-1", State: "active"},
			{ID: 2, ClientID: "client-2", State: "active"},
			{ID: 3, ClientID: "client-3", State: "done"},
		},
		runs: map[int64][]conversations.Run{
			2: {
				{ID: 10, ConversationID: 2, AccountID: 1, Status: "capacity_failed", StreamOffset: 100},
				{ID: 11, ConversationID: 2, AccountID: 2, Status: "completed", StreamOffset: 100},
			},
		},
	})

	listReq := httptest.NewRequest(http.MethodGet, "/conversations?page=1&page_size=2", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("GET /conversations status = %d, want %d", listRec.Code, http.StatusOK)
	}

	var listed []conversations.Conversation
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("listed len = %d, want 2", len(listed))
	}

	runReq := httptest.NewRequest(http.MethodGet, "/conversations/2/runs", nil)
	runRec := httptest.NewRecorder()
	handler.ServeHTTP(runRec, runReq)
	if runRec.Code != http.StatusOK {
		t.Fatalf("GET /conversations/2/runs status = %d, want %d", runRec.Code, http.StatusOK)
	}

	var runs []conversations.Run
	if err := json.Unmarshal(runRec.Body.Bytes(), &runs); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("runs len = %d, want 2", len(runs))
	}
}

type conversationQuery struct {
	conversations []conversations.Conversation
	runs          map[int64][]conversations.Run
}

func (q conversationQuery) ListConversations(offset, limit int) ([]conversations.Conversation, error) {
	end := offset + limit
	if end > len(q.conversations) {
		end = len(q.conversations)
	}
	return q.conversations[offset:end], nil
}

func (q conversationQuery) ListRuns(conversationID int64) ([]conversations.Run, error) {
	return q.runs[conversationID], nil
}

type accountLister struct {
	items []accounts.Account
}

func (l accountLister) List() ([]accounts.Account, error) {
	return l.items, nil
}
