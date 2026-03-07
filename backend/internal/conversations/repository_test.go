package conversations_test

import (
	"path/filepath"
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/conversations"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
)

func TestSQLiteRepositoryConversationMessageAndRunPersistence(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := conversations.NewSQLiteRepository(store.DB())

	conversationID, err := repo.CreateConversation(conversations.Conversation{
		ClientID:             "client-1",
		TargetProviderFamily: "openai",
		DefaultModel:         "gpt-4.1",
		State:                "active",
	})
	if err != nil {
		t.Fatalf("CreateConversation returned error: %v", err)
	}

	if err := repo.AppendMessage(conversations.Message{
		ConversationID: conversationID,
		Role:           "user",
		Content:        "hello",
		SequenceNo:     1,
	}); err != nil {
		t.Fatalf("AppendMessage returned error: %v", err)
	}

	firstRunID, err := repo.CreateRun(conversations.Run{
		ConversationID: conversationID,
		AccountID:      1,
		Status:         "partial",
		StreamOffset:   42,
	})
	if err != nil {
		t.Fatalf("CreateRun(first) returned error: %v", err)
	}

	_, err = repo.CreateRun(conversations.Run{
		ConversationID:   conversationID,
		AccountID:        2,
		FallbackFromRunID: &firstRunID,
		Status:           "completed",
		StreamOffset:     42,
	})
	if err != nil {
		t.Fatalf("CreateRun(second) returned error: %v", err)
	}

	messages, err := repo.ListMessages(conversationID)
	if err != nil {
		t.Fatalf("ListMessages returned error: %v", err)
	}
	if len(messages) != 1 || messages[0].Content != "hello" {
		t.Fatalf("ListMessages returned %+v, want one persisted message", messages)
	}

	runs, err := repo.ListRuns(conversationID)
	if err != nil {
		t.Fatalf("ListRuns returned error: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("ListRuns returned %d rows, want 2", len(runs))
	}
	if runs[1].FallbackFromRunID == nil || *runs[1].FallbackFromRunID != firstRunID {
		t.Fatalf("FallbackFromRunID = %v, want %d", runs[1].FallbackFromRunID, firstRunID)
	}
	if runs[0].StreamOffset != 42 {
		t.Fatalf("StreamOffset = %d, want 42", runs[0].StreamOffset)
	}
}

func TestSQLiteRepositoryListConversations(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := conversations.NewSQLiteRepository(store.DB())

	for i := 0; i < 3; i++ {
		if _, err := repo.CreateConversation(conversations.Conversation{
			ClientID:             "client",
			TargetProviderFamily: "openai",
			DefaultModel:         "gpt-4.1",
			State:                "active",
		}); err != nil {
			t.Fatalf("CreateConversation(%d) returned error: %v", i, err)
		}
	}

	got, err := repo.ListConversations(1, 2)
	if err != nil {
		t.Fatalf("ListConversations returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListConversations returned %d rows, want 2", len(got))
	}
	if got[0].ID != 2 {
		t.Fatalf("first returned conversation id = %d, want 2", got[0].ID)
	}
}
