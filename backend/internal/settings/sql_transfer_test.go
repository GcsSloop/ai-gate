package settings_test

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/secrets"
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
	accountRepo := accounts.NewSQLiteRepository(sourceStore.DB())
	if err := accountRepo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "team-east-a",
		AuthMode:      accounts.AuthModeAPIKey,
		CredentialRef: "sk-east-a",
		BaseURL:       "https://example.test/v1",
		Status:        accounts.StatusActive,
		Priority:      10,
	}); err != nil {
		t.Fatalf("Create(account-a) returned error: %v", err)
	}
	if err := accountRepo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "team-east-b",
		AuthMode:      accounts.AuthModeAPIKey,
		CredentialRef: "sk-east-b",
		BaseURL:       "https://example.test/v1",
		Status:        accounts.StatusActive,
		Priority:      9,
	}); err != nil {
		t.Fatalf("Create(account-b) returned error: %v", err)
	}
	if err := accountRepo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "team-east-c",
		AuthMode:      accounts.AuthModeAPIKey,
		CredentialRef: "sk-east-c",
		BaseURL:       "https://example.test/v1",
		Status:        accounts.StatusActive,
		Priority:      8,
	}); err != nil {
		t.Fatalf("Create(account-c) returned error: %v", err)
	}
	sourceAccounts, err := accountRepo.List()
	if err != nil {
		t.Fatalf("List(source accounts) returned error: %v", err)
	}
	if len(sourceAccounts) != 3 {
		t.Fatalf("len(source accounts) = %d, want 3", len(sourceAccounts))
	}
	if err := sourceSettings.SaveFailoverQueue([]int64{
		sourceAccounts[2].ID,
		sourceAccounts[1].ID,
		sourceAccounts[0].ID,
	}); err != nil {
		t.Fatalf("SaveFailoverQueue(source) returned error: %v", err)
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
	if !equalInt64s(gotQueue, []int64{3, 2, 1}) {
		t.Fatalf("imported queue = %v, want [3 2 1]", gotQueue)
	}

	items, err := accounts.NewSQLiteRepository(targetStore.DB()).List()
	if err != nil {
		t.Fatalf("List(accounts) returned error: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("len(imported accounts) = %d, want 3", len(items))
	}
	if items[0].AccountName != "team-east-a" || items[1].AccountName != "team-east-b" || items[2].AccountName != "team-east-c" {
		t.Fatalf("imported accounts names = [%s %s %s], want [team-east-a team-east-b team-east-c]", items[0].AccountName, items[1].AccountName, items[2].AccountName)
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

func TestSQLTransferImportMergesAccountsWithDuplicateNames(t *testing.T) {
	t.Parallel()

	sourceStore, err := sqlitestore.Open(filepath.Join(t.TempDir(), "source.sqlite"))
	if err != nil {
		t.Fatalf("Open(source) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = sourceStore.Close()
	})
	sourceAccounts := accounts.NewSQLiteRepository(sourceStore.DB())
	if err := sourceAccounts.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "dup-name",
		AuthMode:      accounts.AuthModeAPIKey,
		CredentialRef: "sk-source",
		BaseURL:       "https://source.example/v1",
		Status:        accounts.StatusActive,
		Priority:      20,
	}); err != nil {
		t.Fatalf("Create(source account) returned error: %v", err)
	}
	exported, err := settings.NewSQLTransfer(sourceStore.DB()).Export()
	if err != nil {
		t.Fatalf("Export returned error: %v", err)
	}

	targetStore, err := sqlitestore.Open(filepath.Join(t.TempDir(), "target.sqlite"))
	if err != nil {
		t.Fatalf("Open(target) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = targetStore.Close()
	})
	targetAccounts := accounts.NewSQLiteRepository(targetStore.DB())
	if err := targetAccounts.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "dup-name",
		AuthMode:      accounts.AuthModeAPIKey,
		CredentialRef: "sk-target",
		BaseURL:       "https://target.example/v1",
		Status:        accounts.StatusActive,
		Priority:      10,
	}); err != nil {
		t.Fatalf("Create(target account) returned error: %v", err)
	}

	if err := settings.NewSQLTransfer(targetStore.DB()).Import(exported); err != nil {
		t.Fatalf("Import returned error: %v", err)
	}

	items, err := targetAccounts.List()
	if err != nil {
		t.Fatalf("List(accounts) returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(accounts) = %d, want 2 (merge mode should keep duplicates)", len(items))
	}
}

func TestSQLTransferImportMergesFiveDuplicateNamesIntoTenAccounts(t *testing.T) {
	t.Parallel()

	sourceStore, err := sqlitestore.Open(filepath.Join(t.TempDir(), "source.sqlite"))
	if err != nil {
		t.Fatalf("Open(source) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = sourceStore.Close()
	})
	sourceAccounts := accounts.NewSQLiteRepository(sourceStore.DB())
	for i := 0; i < 5; i++ {
		if err := sourceAccounts.Create(accounts.Account{
			ProviderType:  accounts.ProviderOpenAICompatible,
			AccountName:   "dup-name",
			AuthMode:      accounts.AuthModeAPIKey,
			CredentialRef: "sk-source-" + strconv.Itoa(i),
			BaseURL:       "https://source.example/v1",
			Status:        accounts.StatusActive,
			Priority:      20 - i,
		}); err != nil {
			t.Fatalf("Create(source account %d) returned error: %v", i, err)
		}
	}
	exported, err := settings.NewSQLTransfer(sourceStore.DB()).Export()
	if err != nil {
		t.Fatalf("Export returned error: %v", err)
	}

	targetStore, err := sqlitestore.Open(filepath.Join(t.TempDir(), "target.sqlite"))
	if err != nil {
		t.Fatalf("Open(target) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = targetStore.Close()
	})
	targetAccounts := accounts.NewSQLiteRepository(targetStore.DB())
	for i := 0; i < 5; i++ {
		if err := targetAccounts.Create(accounts.Account{
			ProviderType:  accounts.ProviderOpenAICompatible,
			AccountName:   "dup-name",
			AuthMode:      accounts.AuthModeAPIKey,
			CredentialRef: "sk-target-" + strconv.Itoa(i),
			BaseURL:       "https://target.example/v1",
			Status:        accounts.StatusActive,
			Priority:      10 - i,
		}); err != nil {
			t.Fatalf("Create(target account %d) returned error: %v", i, err)
		}
	}

	if err := settings.NewSQLTransfer(targetStore.DB()).Import(exported); err != nil {
		t.Fatalf("Import returned error: %v", err)
	}

	items, err := targetAccounts.List()
	if err != nil {
		t.Fatalf("List(accounts) returned error: %v", err)
	}
	if len(items) != 10 {
		t.Fatalf("len(accounts) = %d, want 10 (5 existing + 5 imported with same name)", len(items))
	}

	seenIDs := make(map[int64]struct{}, len(items))
	for _, item := range items {
		if _, exists := seenIDs[item.ID]; exists {
			t.Fatalf("duplicate primary key id detected: %d", item.ID)
		}
		seenIDs[item.ID] = struct{}{}
		if item.AccountName != "dup-name" {
			t.Fatalf("account_name = %q, want dup-name", item.AccountName)
		}
	}
}

func TestSQLTransferExportDecryptsCredentialWhenReaderProvided(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "source.sqlite"))
	if err != nil {
		t.Fatalf("Open(source) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	cipher, err := secrets.NewCipher("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewCipher returned error: %v", err)
	}
	repo := accounts.NewSQLiteRepository(store.DB(), cipher)
	rawCredential := `{"auth_mode":"oauth","tokens":{"access_token":"header.payload.sig","account_id":"acc_123"}}`
	if err := repo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAIOfficial,
		AccountName:   "official-portable",
		AuthMode:      accounts.AuthModeLocalImport,
		CredentialRef: rawCredential,
		BaseURL:       "https://chatgpt.com/backend-api/codex",
		Status:        accounts.StatusActive,
		Priority:      10,
	}); err != nil {
		t.Fatalf("Create(account) returned error: %v", err)
	}

	transfer := settings.NewSQLTransfer(
		store.DB(),
		settings.WithAccountCredentialReader(func(accountID int64, stored string) (string, error) {
			account, getErr := repo.GetByID(accountID)
			if getErr != nil {
				return stored, getErr
			}
			return account.CredentialRef, nil
		}),
	)
	exported, err := transfer.Export()
	if err != nil {
		t.Fatalf("Export returned error: %v", err)
	}

	var payload struct {
		Tables map[string]struct {
			Columns []string `json:"columns"`
			Rows    [][]struct {
				Type  string `json:"type"`
				Value any    `json:"value"`
			} `json:"rows"`
		} `json:"tables"`
	}
	if err := json.Unmarshal(exported, &payload); err != nil {
		t.Fatalf("unmarshal payload returned error: %v", err)
	}
	accountsTable, ok := payload.Tables["accounts"]
	if !ok || len(accountsTable.Rows) == 0 {
		t.Fatalf("exported payload missing accounts rows")
	}
	credentialIndex := -1
	for i, col := range accountsTable.Columns {
		if col == "credential_ref" {
			credentialIndex = i
			break
		}
	}
	if credentialIndex < 0 {
		t.Fatal("accounts table missing credential_ref column")
	}
	exportedCredential, ok := accountsTable.Rows[0][credentialIndex].Value.(string)
	if !ok {
		t.Fatalf("credential_ref value type = %T, want string", accountsTable.Rows[0][credentialIndex].Value)
	}
	if exportedCredential != rawCredential {
		t.Fatalf("exported credential_ref mismatch, got %q", exportedCredential)
	}
}

func TestSQLTransferImportEncryptsCredentialWhenWriterProvided(t *testing.T) {
	t.Parallel()

	targetStore, err := sqlitestore.Open(filepath.Join(t.TempDir(), "target.sqlite"))
	if err != nil {
		t.Fatalf("Open(target) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = targetStore.Close()
	})

	cipher, err := secrets.NewCipher("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewCipher returned error: %v", err)
	}
	targetTransfer := settings.NewSQLTransfer(
		targetStore.DB(),
		settings.WithAccountCredentialWriter(func(plain string) (string, error) {
			return cipher.EncryptString(plain)
		}),
	)

	rawCredential := `{"auth_mode":"oauth","tokens":{"access_token":"header.payload.sig","account_id":"acc_777"}}`
	raw := `{
  "format": "aigate-db-exchange",
  "version": 1,
  "tables": {
    "accounts": {
      "columns": ["provider_type","account_name","source_icon","auth_mode","credential_ref","base_url","status","priority","is_active","supports_responses","cooldown_until","created_at"],
      "rows": [[
        {"type":"text","value":"openai-official"},
        {"type":"text","value":"import-encrypt"},
        {"type":"text","value":"openai"},
        {"type":"text","value":"local_import"},
        {"type":"text","value":` + strconv.Quote(rawCredential) + `},
        {"type":"text","value":"https://chatgpt.com/backend-api/codex"},
        {"type":"text","value":"active"},
        {"type":"integer","value":"10"},
        {"type":"integer","value":"0"},
        {"type":"integer","value":"1"},
        {"type":"null"},
        {"type":"text","value":"2026-03-10 00:00:00"}
      ]]
    }
  }
}`

	if err := targetTransfer.Import([]byte(raw)); err != nil {
		t.Fatalf("Import returned error: %v", err)
	}

	var storedCredential string
	if err := targetStore.DB().QueryRow(`SELECT credential_ref FROM accounts WHERE account_name = ?`, "import-encrypt").Scan(&storedCredential); err != nil {
		t.Fatalf("select credential_ref returned error: %v", err)
	}
	if storedCredential == rawCredential {
		t.Fatal("credential_ref should be encrypted at rest after import")
	}

	repo := accounts.NewSQLiteRepository(targetStore.DB(), cipher)
	items, err := repo.List()
	if err != nil {
		t.Fatalf("List(accounts) returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(accounts) = %d, want 1", len(items))
	}
	if items[0].CredentialRef != rawCredential {
		t.Fatalf("decrypted credential_ref mismatch, got %q", items[0].CredentialRef)
	}
}
