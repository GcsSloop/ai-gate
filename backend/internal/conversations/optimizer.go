package conversations

import "database/sql"

type AuditStorageOptimizationResult struct {
	CompactedRows int  `json:"compacted_rows"`
	Vacuumed      bool `json:"vacuumed"`
}

type AuditStorageOptimizer struct {
	db *sql.DB
}

func NewAuditStorageOptimizer(db *sql.DB) *AuditStorageOptimizer {
	return &AuditStorageOptimizer{db: db}
}

func (o *AuditStorageOptimizer) Optimize(policy AuditStoragePolicy) (AuditStorageOptimizationResult, error) {
	repo := NewSQLiteRepository(o.db, WithAuditStoragePolicyProvider(func() AuditStoragePolicy {
		return policy
	}))
	compactedRows := 0
	for _, itemType := range []string{
		"message",
		"function_call",
		"function_call_output",
		"reasoning",
		"custom_tool_call",
		"custom_tool_call_output",
	} {
		count, err := repo.compactOlderMessages(itemType)
		if err != nil {
			return AuditStorageOptimizationResult{}, err
		}
		compactedRows += count
	}

	result := AuditStorageOptimizationResult{CompactedRows: compactedRows}
	if compactedRows == 0 {
		return result, nil
	}
	if _, err := o.db.Exec(`VACUUM`); err == nil {
		result.Vacuumed = true
	}
	return result, nil
}
