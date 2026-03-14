package conversations

import "time"

type Conversation struct {
	ID                   int64
	ClientID             string
	TargetProviderFamily string
	DefaultModel         string
	CurrentAccountID     *int64
	State                string
	CreatedAt            time.Time
}

type Message struct {
	ID             int64
	ConversationID int64
	Role           string
	Content        string
	ItemType       string
	RawItemJSON    string
	ContentPreview string
	ContentSHA256  string
	ContentBytes   int
	RawPreview     string
	RawSHA256      string
	RawBytes       int
	ToolName       string
	ToolCallID     string
	SummaryJSON    string
	StorageMode    string
	SequenceNo     int
	CreatedAt      time.Time
}

type Run struct {
	ID                int64
	ConversationID    int64
	AccountID         int64
	Model             string
	FallbackFromRunID *int64
	Status            string
	StreamOffset      int
	StartedAt         time.Time
}

type AccountCallStats struct {
	AccountID  int64
	TotalCalls int
	ModelCalls map[string]int
}

type AuditStoragePolicy struct {
	MessageLimit              int
	FunctionCallLimit         int
	FunctionCallOutputLimit   int
	ReasoningLimit            int
	CustomToolCallLimit       int
	CustomToolCallOutputLimit int
}

func DefaultAuditStoragePolicy() AuditStoragePolicy {
	return AuditStoragePolicy{
		MessageLimit:              200,
		FunctionCallLimit:         100,
		FunctionCallOutputLimit:   100,
		ReasoningLimit:            40,
		CustomToolCallLimit:       100,
		CustomToolCallOutputLimit: 100,
	}
}
