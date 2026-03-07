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
	SequenceNo     int
	CreatedAt      time.Time
}

type Run struct {
	ID                int64
	ConversationID    int64
	AccountID         int64
	FallbackFromRunID *int64
	Status            string
	StreamOffset      int
	StartedAt         time.Time
}
