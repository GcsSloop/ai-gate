package conversations

import (
	"database/sql"
	"fmt"
)

type SQLiteRepository struct {
	db             *sql.DB
	policyProvider func() AuditStoragePolicy
}

type SQLiteRepositoryOption func(*SQLiteRepository)

func WithAuditStoragePolicyProvider(provider func() AuditStoragePolicy) SQLiteRepositoryOption {
	return func(repo *SQLiteRepository) {
		repo.policyProvider = provider
	}
}

func NewSQLiteRepository(db *sql.DB, opts ...SQLiteRepositoryOption) *SQLiteRepository {
	repo := &SQLiteRepository{
		db: db,
		policyProvider: func() AuditStoragePolicy {
			return DefaultAuditStoragePolicy()
		},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(repo)
		}
	}
	return repo
}

func (r *SQLiteRepository) CreateConversation(conversation Conversation) (int64, error) {
	result, err := r.db.Exec(
		`INSERT INTO conversations (client_id, target_provider_family, default_model, current_account_id, state)
		 VALUES (?, ?, ?, ?, ?)`,
		conversation.ClientID,
		conversation.TargetProviderFamily,
		conversation.DefaultModel,
		nullInt64(conversation.CurrentAccountID),
		conversation.State,
	)
	if err != nil {
		return 0, fmt.Errorf("insert conversation: %w", err)
	}

	return result.LastInsertId()
}

func (r *SQLiteRepository) ListConversations(offset, limit int) ([]Conversation, error) {
	rows, err := r.db.Query(
		`SELECT id, client_id, target_provider_family, default_model, current_account_id, state, created_at
		 FROM conversations
		 ORDER BY id ASC
		 LIMIT ? OFFSET ?`,
		limit,
		offset,
	)
	if err != nil {
		return nil, fmt.Errorf("query conversations: %w", err)
	}
	defer rows.Close()

	var items []Conversation
	for rows.Next() {
		var item Conversation
		var currentAccount sql.NullInt64
		if err := rows.Scan(
			&item.ID,
			&item.ClientID,
			&item.TargetProviderFamily,
			&item.DefaultModel,
			&currentAccount,
			&item.State,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		if currentAccount.Valid {
			value := currentAccount.Int64
			item.CurrentAccountID = &value
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conversations: %w", err)
	}
	return items, nil
}

func (r *SQLiteRepository) AppendMessage(message Message) error {
	message = compactMessageRecord(message, false)
	_, err := r.db.Exec(
		`INSERT INTO messages (
			conversation_id, role, content, item_type, raw_item_json,
			content_preview, content_sha256, content_bytes,
			raw_preview, raw_sha256, raw_bytes,
			tool_name, tool_call_id, summary_json, storage_mode,
			sequence_no
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		message.ConversationID,
		message.Role,
		message.Content,
		message.ItemType,
		message.RawItemJSON,
		message.ContentPreview,
		message.ContentSHA256,
		message.ContentBytes,
		message.RawPreview,
		message.RawSHA256,
		message.RawBytes,
		message.ToolName,
		message.ToolCallID,
		message.SummaryJSON,
		message.StorageMode,
		message.SequenceNo,
	)
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}
	if _, err := r.compactOlderMessages(message.ItemType); err != nil {
		return err
	}
	return nil
}

func (r *SQLiteRepository) CreateRun(run Run) (int64, error) {
	result, err := r.db.Exec(
		`INSERT INTO runs (conversation_id, account_id, model, fallback_from_run_id, stream_offset, status)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		run.ConversationID,
		run.AccountID,
		run.Model,
		nullInt64(run.FallbackFromRunID),
		run.StreamOffset,
		run.Status,
	)
	if err != nil {
		return 0, fmt.Errorf("insert run: %w", err)
	}

	return result.LastInsertId()
}

func (r *SQLiteRepository) ListMessages(conversationID int64) ([]Message, error) {
	rows, err := r.db.Query(
		`SELECT id, conversation_id, role, content, item_type, raw_item_json,
		        content_preview, content_sha256, content_bytes,
		        raw_preview, raw_sha256, raw_bytes,
		        tool_name, tool_call_id, summary_json, storage_mode,
		        sequence_no, created_at
		 FROM messages WHERE conversation_id = ? ORDER BY sequence_no ASC, id ASC`,
		conversationID,
	)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var message Message
		if err := rows.Scan(
			&message.ID,
			&message.ConversationID,
			&message.Role,
			&message.Content,
			&message.ItemType,
			&message.RawItemJSON,
			&message.ContentPreview,
			&message.ContentSHA256,
			&message.ContentBytes,
			&message.RawPreview,
			&message.RawSHA256,
			&message.RawBytes,
			&message.ToolName,
			&message.ToolCallID,
			&message.SummaryJSON,
			&message.StorageMode,
			&message.SequenceNo,
			&message.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}
	return messages, nil
}

func (r *SQLiteRepository) compactOlderMessages(itemType string) (int, error) {
	limit := r.limitForItemType(itemType)
	if limit <= 0 {
		return 0, nil
	}
	rows, err := r.db.Query(
		`SELECT id, role, content, item_type, raw_item_json
		 FROM messages
		 WHERE item_type = ? AND storage_mode = ?
		 ORDER BY id DESC
		 LIMIT -1 OFFSET ?`,
		itemType,
		messageStorageModeFull,
		limit,
	)
	if err != nil {
		return 0, fmt.Errorf("query compact candidates for %s: %w", itemType, err)
	}
	defer rows.Close()

	type candidate struct {
		id      int64
		role    string
		content string
		raw     string
	}
	var candidates []candidate
	for rows.Next() {
		var item candidate
		var storedItemType string
		if err := rows.Scan(&item.id, &item.role, &item.content, &storedItemType, &item.raw); err != nil {
			return 0, fmt.Errorf("scan compact candidate for %s: %w", itemType, err)
		}
		candidates = append(candidates, item)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate compact candidates for %s: %w", itemType, err)
	}
	for _, candidate := range candidates {
		compacted := compactMessageRecord(Message{
			Role:        candidate.role,
			Content:     candidate.content,
			ItemType:    itemType,
			RawItemJSON: candidate.raw,
		}, true)
		if _, err := r.db.Exec(
			`UPDATE messages
			 SET content = ?, raw_item_json = ?, content_preview = ?, content_sha256 = ?, content_bytes = ?,
			     raw_preview = ?, raw_sha256 = ?, raw_bytes = ?, tool_name = ?, tool_call_id = ?,
			     summary_json = ?, storage_mode = ?
			 WHERE id = ?`,
			compacted.Content,
			compacted.RawItemJSON,
			compacted.ContentPreview,
			compacted.ContentSHA256,
			compacted.ContentBytes,
			compacted.RawPreview,
			compacted.RawSHA256,
			compacted.RawBytes,
			compacted.ToolName,
			compacted.ToolCallID,
			compacted.SummaryJSON,
			compacted.StorageMode,
			candidate.id,
		); err != nil {
			return 0, fmt.Errorf("compact message %d: %w", candidate.id, err)
		}
	}
	return len(candidates), nil
}

func (r *SQLiteRepository) limitForItemType(itemType string) int {
	policy := DefaultAuditStoragePolicy()
	if r.policyProvider != nil {
		policy = r.policyProvider()
	}
	switch itemType {
	case "function_call":
		return policy.FunctionCallLimit
	case "function_call_output":
		return policy.FunctionCallOutputLimit
	case "reasoning":
		return policy.ReasoningLimit
	case "custom_tool_call":
		return policy.CustomToolCallLimit
	case "custom_tool_call_output":
		return policy.CustomToolCallOutputLimit
	case "message":
		return policy.MessageLimit
	default:
		return policy.MessageLimit
	}
}

func (r *SQLiteRepository) ListRuns(conversationID int64) ([]Run, error) {
	rows, err := r.db.Query(
		`SELECT id, conversation_id, account_id, model, fallback_from_run_id, status, stream_offset, started_at
		 FROM runs WHERE conversation_id = ? ORDER BY id ASC`,
		conversationID,
	)
	if err != nil {
		return nil, fmt.Errorf("query runs: %w", err)
	}
	defer rows.Close()

	var runs []Run
	for rows.Next() {
		var run Run
		var fallback sql.NullInt64
		if err := rows.Scan(&run.ID, &run.ConversationID, &run.AccountID, &run.Model, &fallback, &run.Status, &run.StreamOffset, &run.StartedAt); err != nil {
			return nil, fmt.Errorf("scan run: %w", err)
		}
		if fallback.Valid {
			value := fallback.Int64
			run.FallbackFromRunID = &value
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runs: %w", err)
	}
	return runs, nil
}

func (r *SQLiteRepository) CountConversations() (int, error) {
	return r.count(`SELECT COUNT(*) FROM conversations`)
}

func (r *SQLiteRepository) CountActiveConversations() (int, error) {
	return r.count(`SELECT COUNT(*) FROM conversations WHERE state = 'active'`)
}

func (r *SQLiteRepository) CountRuns() (int, error) {
	return r.count(`SELECT COUNT(*) FROM runs`)
}

func (r *SQLiteRepository) CountFailoverRuns() (int, error) {
	return r.count(`SELECT COUNT(*) FROM runs WHERE fallback_from_run_id IS NOT NULL`)
}

func (r *SQLiteRepository) ListAccountCallStats() ([]AccountCallStats, error) {
	rows, err := r.db.Query(
		`SELECT r.account_id, COALESCE(NULLIF(r.model, ''), c.default_model, ''), COUNT(*)
		 FROM runs r
		 LEFT JOIN conversations c ON c.id = r.conversation_id
		 WHERE r.account_id IS NOT NULL
		 GROUP BY r.account_id, COALESCE(NULLIF(r.model, ''), c.default_model, '')
		 ORDER BY r.account_id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query account call stats: %w", err)
	}
	defer rows.Close()

	statsByAccount := map[int64]*AccountCallStats{}
	order := make([]int64, 0)
	for rows.Next() {
		var accountID int64
		var model string
		var count int
		if err := rows.Scan(&accountID, &model, &count); err != nil {
			return nil, fmt.Errorf("scan account call stats: %w", err)
		}
		stat, ok := statsByAccount[accountID]
		if !ok {
			stat = &AccountCallStats{
				AccountID:  accountID,
				ModelCalls: map[string]int{},
			}
			statsByAccount[accountID] = stat
			order = append(order, accountID)
		}
		stat.TotalCalls += count
		if model != "" {
			stat.ModelCalls[model] += count
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate account call stats: %w", err)
	}

	result := make([]AccountCallStats, 0, len(order))
	for _, accountID := range order {
		result = append(result, *statsByAccount[accountID])
	}
	return result, nil
}

func (r *SQLiteRepository) count(query string) (int, error) {
	var total int
	if err := r.db.QueryRow(query).Scan(&total); err != nil {
		return 0, fmt.Errorf("count rows: %w", err)
	}
	return total, nil
}

func nullInt64(value *int64) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *value, Valid: true}
}
