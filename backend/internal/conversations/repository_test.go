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
		ItemType:       "function_call_output",
		RawItemJSON:    `{"type":"function_call_output","call_id":"call_123","output":"hello"}`,
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
		ConversationID:    conversationID,
		AccountID:         2,
		FallbackFromRunID: &firstRunID,
		Status:            "completed",
		StreamOffset:      42,
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
	if messages[0].ItemType != "function_call_output" {
		t.Fatalf("ItemType = %q, want function_call_output", messages[0].ItemType)
	}
	if messages[0].RawItemJSON == "" {
		t.Fatal("RawItemJSON is empty, want persisted raw item")
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

func TestSQLiteRepositoryListAccountCallStats(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := conversations.NewSQLiteRepository(store.DB())

	conversationA, err := repo.CreateConversation(conversations.Conversation{
		ClientID:             "client-a",
		TargetProviderFamily: "openai",
		DefaultModel:         "gpt-5.4",
		State:                "active",
	})
	if err != nil {
		t.Fatalf("CreateConversation(A) returned error: %v", err)
	}
	conversationB, err := repo.CreateConversation(conversations.Conversation{
		ClientID:             "client-b",
		TargetProviderFamily: "openai",
		DefaultModel:         "gpt-4.1",
		State:                "active",
	})
	if err != nil {
		t.Fatalf("CreateConversation(B) returned error: %v", err)
	}

	for _, run := range []conversations.Run{
		{ConversationID: conversationA, AccountID: 1, Model: "gpt-5.4", Status: "completed"},
		{ConversationID: conversationA, AccountID: 1, Model: "gpt-4.1", Status: "soft_failed"},
		{ConversationID: conversationB, AccountID: 1, Model: "gpt-4.1", Status: "completed"},
		{ConversationID: conversationA, AccountID: 2, Model: "gpt-5.4", Status: "rate_limited"},
	} {
		if _, err := repo.CreateRun(run); err != nil {
			t.Fatalf("CreateRun returned error: %v", err)
		}
	}

	stats, err := repo.ListAccountCallStats()
	if err != nil {
		t.Fatalf("ListAccountCallStats returned error: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("ListAccountCallStats returned %d rows, want 2", len(stats))
	}

	if stats[0].AccountID != 1 || stats[0].TotalCalls != 3 {
		t.Fatalf("stats[0] = %+v, want account 1 with 3 calls", stats[0])
	}
	if stats[0].ModelCalls["gpt-5.4"] != 1 {
		t.Fatalf("account 1 gpt-5.4 = %d, want 1", stats[0].ModelCalls["gpt-5.4"])
	}
	if stats[0].ModelCalls["gpt-4.1"] != 2 {
		t.Fatalf("account 1 gpt-4.1 = %d, want 2", stats[0].ModelCalls["gpt-4.1"])
	}

	if stats[1].AccountID != 2 || stats[1].TotalCalls != 1 {
		t.Fatalf("stats[1] = %+v, want account 2 with 1 call", stats[1])
	}
	if stats[1].ModelCalls["gpt-5.4"] != 1 {
		t.Fatalf("account 2 gpt-5.4 = %d, want 1", stats[1].ModelCalls["gpt-5.4"])
	}
}

func TestSQLiteRepositoryCompactsOlderAuditMessages(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repo := conversations.NewSQLiteRepository(
		store.DB(),
		conversations.WithAuditStoragePolicyProvider(func() conversations.AuditStoragePolicy {
			return conversations.AuditStoragePolicy{
				MessageLimit:              10,
				FunctionCallLimit:         10,
				FunctionCallOutputLimit:   2,
				ReasoningLimit:            10,
				CustomToolCallLimit:       10,
				CustomToolCallOutputLimit: 10,
			}
		}),
	)

	conversationID, err := repo.CreateConversation(conversations.Conversation{
		ClientID:             "client-1",
		TargetProviderFamily: "openai",
		DefaultModel:         "gpt-5.4",
		State:                "active",
	})
	if err != nil {
		t.Fatalf("CreateConversation returned error: %v", err)
	}

	for i, payload := range []string{
		`{"type":"function_call_output","call_id":"call_1","name":"shell","output":"first result"}`,
		`{"type":"function_call_output","call_id":"call_2","name":"shell","output":"second result"}`,
		`{"type":"function_call_output","call_id":"call_3","name":"shell","output":"third result"}`,
	} {
		if err := repo.AppendMessage(conversations.Message{
			ConversationID: conversationID,
			Role:           "assistant",
			Content:        "output payload",
			ItemType:       "function_call_output",
			RawItemJSON:    payload,
			SequenceNo:     i,
		}); err != nil {
			t.Fatalf("AppendMessage(%d) returned error: %v", i, err)
		}
	}

	messages, err := repo.ListMessages(conversationID)
	if err != nil {
		t.Fatalf("ListMessages returned error: %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("len(messages) = %d, want 3", len(messages))
	}
	if messages[0].StorageMode != "summary" {
		t.Fatalf("messages[0].StorageMode = %q, want summary", messages[0].StorageMode)
	}
	if messages[0].RawItemJSON != "" {
		t.Fatalf("messages[0].RawItemJSON = %q, want empty after compaction", messages[0].RawItemJSON)
	}
	if messages[0].ToolName != "shell" {
		t.Fatalf("messages[0].ToolName = %q, want shell", messages[0].ToolName)
	}
	if messages[0].RawBytes == 0 || messages[0].RawSHA256 == "" {
		t.Fatalf("messages[0] summary metadata missing: %+v", messages[0])
	}
	if messages[1].StorageMode != "full" || messages[2].StorageMode != "full" {
		t.Fatalf("newest messages should remain full, got %q and %q", messages[1].StorageMode, messages[2].StorageMode)
	}
}

func TestOptimizeAuditStorageCompactsHistoricalRows(t *testing.T) {
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
		DefaultModel:         "gpt-5.4",
		State:                "active",
	})
	if err != nil {
		t.Fatalf("CreateConversation returned error: %v", err)
	}

	for i := 0; i < 4; i++ {
		if err := repo.AppendMessage(conversations.Message{
			ConversationID: conversationID,
			Role:           "assistant",
			Content:        "message body",
			ItemType:       "reasoning",
			RawItemJSON:    `{"type":"reasoning","summary":"very long chain of thought"}`,
			SequenceNo:     i,
		}); err != nil {
			t.Fatalf("AppendMessage(%d) returned error: %v", i, err)
		}
	}

	optimizer := conversations.NewAuditStorageOptimizer(store.DB())
	result, err := optimizer.Optimize(conversations.AuditStoragePolicy{
		MessageLimit:              10,
		FunctionCallLimit:         10,
		FunctionCallOutputLimit:   10,
		ReasoningLimit:            2,
		CustomToolCallLimit:       10,
		CustomToolCallOutputLimit: 10,
	})
	if err != nil {
		t.Fatalf("Optimize returned error: %v", err)
	}
	if result.CompactedRows != 2 {
		t.Fatalf("CompactedRows = %d, want 2", result.CompactedRows)
	}

	messages, err := repo.ListMessages(conversationID)
	if err != nil {
		t.Fatalf("ListMessages returned error: %v", err)
	}
	if messages[0].StorageMode != "summary" || messages[1].StorageMode != "summary" {
		t.Fatalf("oldest reasoning rows should be summary, got %q and %q", messages[0].StorageMode, messages[1].StorageMode)
	}
	if messages[2].StorageMode != "full" || messages[3].StorageMode != "full" {
		t.Fatalf("newest reasoning rows should remain full, got %q and %q", messages[2].StorageMode, messages[3].StorageMode)
	}
}
