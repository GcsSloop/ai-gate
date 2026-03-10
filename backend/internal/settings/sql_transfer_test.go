package settings_test

import (
	"fmt"
	"path/filepath"
	"testing"

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
	if err := sourceSettings.SaveAppSettings(settings.AppSettings{
		CloseToTray:             true,
		ShowProxySwitchOnHome:   true,
		ProxyHost:               "localhost",
		ProxyPort:               15721,
		AutoBackupIntervalHours: 6,
		BackupRetentionCount:    4,
		AutoFailoverEnabled:     true,
	}); err != nil {
		t.Fatalf("SaveAppSettings(source) returned error: %v", err)
	}
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

	targetStore, err := sqlitestore.Open(filepath.Join(t.TempDir(), "target.sqlite"))
	if err != nil {
		t.Fatalf("Open(target) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = targetStore.Close()
	})

	targetSettings := settings.NewSQLiteRepository(targetStore.DB())
	if err := targetSettings.SaveAppSettings(settings.DefaultAppSettings()); err != nil {
		t.Fatalf("SaveAppSettings(target) returned error: %v", err)
	}

	targetTransfer := settings.NewSQLTransfer(targetStore.DB())
	if err := targetTransfer.Import(exported); err != nil {
		t.Fatalf("Import returned error: %v", err)
	}

	gotSettings, err := targetSettings.GetAppSettings()
	if err != nil {
		t.Fatalf("GetAppSettings(target) returned error: %v", err)
	}
	if gotSettings.ProxyHost != "localhost" || gotSettings.ProxyPort != 15721 || !gotSettings.AutoFailoverEnabled || gotSettings.AutoBackupIntervalHours != 6 || gotSettings.BackupRetentionCount != 4 {
		t.Fatalf("imported settings = %+v, want source settings", gotSettings)
	}

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

	raw := `BEGIN TRANSACTION;
CREATE TABLE "accounts" (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  provider_type TEXT NOT NULL,
  account_name TEXT NOT NULL,
  source_icon TEXT NOT NULL DEFAULT 'openai',
  auth_mode TEXT NOT NULL,
  credential_ref TEXT NOT NULL,
  base_url TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  priority INTEGER NOT NULL DEFAULT 0,
  is_active INTEGER NOT NULL DEFAULT 0,
  supports_responses INTEGER NOT NULL DEFAULT 0,
  cooldown_until DATETIME,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  legacy_extra_col TEXT NOT NULL DEFAULT ''
);
INSERT INTO "accounts" ("provider_type","account_name","source_icon","auth_mode","credential_ref","base_url","status","priority","is_active","supports_responses","cooldown_until","created_at","legacy_extra_col")
VALUES ('openai-compatible','extra-schema','openai','api_key','sk-extra','https://example.test/v1','active',9,1,1,NULL,'2026-03-10 00:00:00','legacy-value');
`
	for _, table := range []string{
		"account_usage_snapshots",
		"conversations",
		"messages",
		"runs",
		"app_settings",
		"failover_queue_items",
	} {
		raw += fmt.Sprintf("CREATE TABLE \"%s\" (id INTEGER PRIMARY KEY AUTOINCREMENT);\n", table)
	}
	raw += "COMMIT;\n"

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
