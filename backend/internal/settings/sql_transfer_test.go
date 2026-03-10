package settings_test

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/settings"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
)

func TestSQLTransferExportAndImportOwnedTables(t *testing.T) {
	t.Parallel()

	sourceStore, err := sqlitestore.Open(filepath.Join(t.TempDir(), "source.sqlite"))
	if err != nil {
		t.Fatalf("Open(source) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = sourceStore.Close()
	})

	sourceSettings := settings.NewSQLiteRepository(sourceStore.DB())
	if err := sourceSettings.SaveFailoverQueue([]int64{9, 2, 1}); err != nil {
		t.Fatalf("SaveFailoverQueue(source) returned error: %v", err)
	}

	accountRepo := accounts.NewSQLiteRepository(sourceStore.DB())
	if err := accountRepo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "team-east",
		AuthMode:      accounts.AuthModeAPIKey,
		CredentialRef: "sk-east",
		BaseURL:       "https://example.test/v1",
		Status:        accounts.StatusActive,
		Priority:      10,
	}); err != nil {
		t.Fatalf("Create(account) returned error: %v", err)
	}

	transfer := settings.NewSQLTransfer(sourceStore.DB())
	exported, err := transfer.Export()
	if err != nil {
		t.Fatalf("Export returned error: %v", err)
	}
	if len(exported) == 0 {
		t.Fatal("Export returned empty payload")
	}
	var exportedPayload map[string]any
	if err := json.Unmarshal(exported, &exportedPayload); err != nil {
		t.Fatalf("Export returned non-JSON payload: %v", err)
	}
	tables, ok := exportedPayload["tables"].(map[string]any)
	if !ok {
		t.Fatalf("Export payload missing tables field: %s", string(exported))
	}
	if len(tables) != 3 {
		t.Fatalf("len(tables) = %d, want 3 account-domain tables", len(tables))
	}
	if _, ok := tables["accounts"]; !ok {
		t.Fatalf("export payload missing accounts table: %s", string(exported))
	}
	if _, ok := tables["account_usage_snapshots"]; !ok {
		t.Fatalf("export payload missing account_usage_snapshots table: %s", string(exported))
	}
	if _, ok := tables["failover_queue_items"]; !ok {
		t.Fatalf("export payload missing failover_queue_items table: %s", string(exported))
	}
	if _, ok := tables["messages"]; ok {
		t.Fatalf("export payload unexpectedly contains messages table: %s", string(exported))
	}

	targetStore, err := sqlitestore.Open(filepath.Join(t.TempDir(), "target.sqlite"))
	if err != nil {
		t.Fatalf("Open(target) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = targetStore.Close()
	})

	targetTransfer := settings.NewSQLTransfer(targetStore.DB())
	if err := targetTransfer.Import(exported); err != nil {
		t.Fatalf("Import returned error: %v", err)
	}

	targetSettings := settings.NewSQLiteRepository(targetStore.DB())
	gotQueue, err := targetSettings.ListFailoverQueue()
	if err != nil {
		t.Fatalf("ListFailoverQueue(target) returned error: %v", err)
	}
	if !equalInt64s(gotQueue, []int64{9, 2, 1}) {
		t.Fatalf("imported queue = %v, want [9 2 1]", gotQueue)
	}

	items, err := accounts.NewSQLiteRepository(targetStore.DB()).List()
	if err != nil {
		t.Fatalf("List(accounts) returned error: %v", err)
	}
	if len(items) != 1 || items[0].AccountName != "team-east" {
		t.Fatalf("imported accounts = %+v, want [team-east]", items)
	}
}

func TestSQLTransferImportAccountsWithExtraSourceColumns(t *testing.T) {
	t.Parallel()

	targetStore, err := sqlitestore.Open(filepath.Join(t.TempDir(), "target.sqlite"))
	if err != nil {
		t.Fatalf("Open(target) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = targetStore.Close()
	})

	targetTransfer := settings.NewSQLTransfer(targetStore.DB())

	raw := `{
  "format": "aigate-db-exchange",
  "version": 1,
  "tables": {
    "accounts": {
      "columns": ["provider_type","account_name","source_icon","auth_mode","credential_ref","base_url","status","priority","is_active","supports_responses","cooldown_until","created_at","legacy_extra_col"],
      "rows": [[
        {"type":"text","value":"openai-compatible"},
        {"type":"text","value":"extra-schema"},
        {"type":"text","value":"openai"},
        {"type":"text","value":"api_key"},
        {"type":"text","value":"sk-extra"},
        {"type":"text","value":"https://example.test/v1"},
        {"type":"text","value":"active"},
        {"type":"integer","value":"9"},
        {"type":"integer","value":"1"},
        {"type":"integer","value":"1"},
        {"type":"null"},
        {"type":"text","value":"2026-03-10 00:00:00"},
        {"type":"text","value":"legacy-value"}
      ]]
    }
  }
}`

	if err := targetTransfer.Import([]byte(raw)); err != nil {
		t.Fatalf("Import returned error: %v", err)
	}

	items, err := accounts.NewSQLiteRepository(targetStore.DB()).List()
	if err != nil {
		t.Fatalf("List(accounts) returned error: %v", err)
	}
	if len(items) != 1 || items[0].AccountName != "extra-schema" || items[0].Priority != 9 {
		t.Fatalf("imported accounts = %+v, want account extra-schema with priority 9", items)
	}
}

func TestSQLTransferExportSparsifiesAccountUsageSnapshots(t *testing.T) {
	t.Parallel()

	sourceStore, err := sqlitestore.Open(filepath.Join(t.TempDir(), "source.sqlite"))
	if err != nil {
		t.Fatalf("Open(source) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = sourceStore.Close()
	})

	accountRepo := accounts.NewSQLiteRepository(sourceStore.DB())
	if err := accountRepo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "sparse-test",
		AuthMode:      accounts.AuthModeAPIKey,
		CredentialRef: "sk-sparse",
		BaseURL:       "https://example.test/v1",
		Status:        accounts.StatusActive,
		Priority:      10,
	}); err != nil {
		t.Fatalf("Create(account) returned error: %v", err)
	}
	items, err := accountRepo.List()
	if err != nil || len(items) == 0 {
		t.Fatalf("List(account) returned error=%v len=%d", err, len(items))
	}
	accountID := items[0].ID

	now := time.Now().UTC()
	insertSnapshot := func(ts time.Time, tokens int) {
		_, execErr := sourceStore.DB().Exec(
			`INSERT INTO account_usage_snapshots (account_id, last_total_tokens, checked_at) VALUES (?, ?, ?)`,
			accountID,
			tokens,
			ts.Format(time.RFC3339),
		)
		if execErr != nil {
			t.Fatalf("insert snapshot at %s failed: %v", ts.Format(time.RFC3339), execErr)
		}
	}

	insertSnapshot(now.Add(-2*time.Hour), 101)
	insertSnapshot(now.Add(-3*time.Hour), 102)
	insertSnapshot(now.Add(-20*time.Minute), 103)
	insertSnapshot(now.AddDate(0, 0, -10).Add(-10*time.Minute), 201)
	insertSnapshot(now.AddDate(0, 0, -10).Add(-40*time.Minute), 202)
	insertSnapshot(now.AddDate(0, 0, -40).Add(-2*time.Hour), 301)
	insertSnapshot(now.AddDate(0, 0, -40).Add(-5*time.Hour), 302)

	transfer := settings.NewSQLTransfer(sourceStore.DB())
	exported, err := transfer.Export()
	if err != nil {
		t.Fatalf("Export returned error: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(exported, &payload); err != nil {
		t.Fatalf("unmarshal export payload failed: %v", err)
	}
	tables, ok := payload["tables"].(map[string]any)
	if !ok {
		t.Fatalf("tables field type = %T, want map", payload["tables"])
	}
	snapshots, ok := tables["account_usage_snapshots"].(map[string]any)
	if !ok {
		t.Fatalf("account_usage_snapshots field type = %T, want map", tables["account_usage_snapshots"])
	}
	rows, ok := snapshots["rows"].([]any)
	if !ok {
		t.Fatalf("snapshot rows type = %T, want array", snapshots["rows"])
	}
	if len(rows) != 5 {
		t.Fatalf("len(snapshot rows) = %d, want 5 (3 recent + 1 mid + 1 old)", len(rows))
	}
}
