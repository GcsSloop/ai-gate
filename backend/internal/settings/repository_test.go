package settings_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/settings"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
)

func TestRepositoryReturnsDefaultsWhenUnset(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := settings.NewSQLiteRepository(store.DB())

	got, err := repo.GetAppSettings()
	if err != nil {
		t.Fatalf("GetAppSettings returned error: %v", err)
	}

	want := settings.DefaultAppSettings()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetAppSettings = %+v, want %+v", got, want)
	}

	queue, err := repo.ListFailoverQueue()
	if err != nil {
		t.Fatalf("ListFailoverQueue returned error: %v", err)
	}
	if len(queue) != 0 {
		t.Fatalf("ListFailoverQueue returned %v, want empty", queue)
	}
}

func TestRepositoryPersistsAppSettingsAndQueue(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := settings.NewSQLiteRepository(store.DB())
	wantSettings := settings.AppSettings{
		LaunchAtLogin:                true,
		SilentStart:                  true,
		CloseToTray:                  false,
		ShowProxySwitchOnHome:        false,
		ShowHomeUpdateIndicator:      false,
		StatusRefreshIntervalSeconds: 15,
		ProxyHost:                    "localhost",
		ProxyPort:                    15721,
		AutoFailoverEnabled:          true,
		AutoBackupIntervalHours:      12,
		BackupRetentionCount:         7,
		Language:                     "en-US",
		ThemeMode:                    "dark",
		ProviderPricing: map[string]settings.PricingRule{
			"codex": {InputPerMillion: 3.5, OutputPerMillion: 14.2},
		},
		AccountPricing: map[string]settings.PricingRule{
			settings.AccountPricingKey(3): {InputPerMillion: 2.1, OutputPerMillion: 9.6},
		},
	}

	if err := repo.SaveAppSettings(wantSettings); err != nil {
		t.Fatalf("SaveAppSettings returned error: %v", err)
	}
	if err := repo.SaveFailoverQueue([]int64{3, 1, 8}); err != nil {
		t.Fatalf("SaveFailoverQueue returned error: %v", err)
	}

	gotSettings, err := repo.GetAppSettings()
	if err != nil {
		t.Fatalf("GetAppSettings returned error: %v", err)
	}
	if !reflect.DeepEqual(gotSettings, wantSettings) {
		t.Fatalf("GetAppSettings = %+v, want %+v", gotSettings, wantSettings)
	}

	gotQueue, err := repo.ListFailoverQueue()
	if err != nil {
		t.Fatalf("ListFailoverQueue returned error: %v", err)
	}
	wantQueue := []int64{3, 1, 8}
	if !reflect.DeepEqual(gotQueue, wantQueue) {
		t.Fatalf("ListFailoverQueue = %v, want %v", gotQueue, wantQueue)
	}
}

func TestRepositoryClampsStatusRefreshInterval(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := settings.NewSQLiteRepository(store.DB())

	value := settings.DefaultAppSettings()
	value.StatusRefreshIntervalSeconds = 1
	if err := repo.SaveAppSettings(value); err != nil {
		t.Fatalf("SaveAppSettings(low) returned error: %v", err)
	}

	low, err := repo.GetAppSettings()
	if err != nil {
		t.Fatalf("GetAppSettings(low) returned error: %v", err)
	}
	if low.StatusRefreshIntervalSeconds != 5 {
		t.Fatalf("low.StatusRefreshIntervalSeconds = %d, want 5", low.StatusRefreshIntervalSeconds)
	}

	value.StatusRefreshIntervalSeconds = 9_999
	if err := repo.SaveAppSettings(value); err != nil {
		t.Fatalf("SaveAppSettings(high) returned error: %v", err)
	}

	high, err := repo.GetAppSettings()
	if err != nil {
		t.Fatalf("GetAppSettings(high) returned error: %v", err)
	}
	if high.StatusRefreshIntervalSeconds != 3600 {
		t.Fatalf("high.StatusRefreshIntervalSeconds = %d, want 3600", high.StatusRefreshIntervalSeconds)
	}
}

func TestRepositoryPersistsPricingRules(t *testing.T) {
	t.Parallel()

	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := settings.NewSQLiteRepository(store.DB())
	value := settings.DefaultAppSettings()
	value.ProviderPricing = map[string]settings.PricingRule{
		"codex":  {InputPerMillion: 4.5, OutputPerMillion: 18},
		"openai": {InputPerMillion: 2.5, OutputPerMillion: 10},
		"broken": {InputPerMillion: -1, OutputPerMillion: 10},
		"":       {InputPerMillion: 1, OutputPerMillion: 1},
	}
	value.AccountPricing = map[string]settings.PricingRule{
		settings.AccountPricingKey(12): {InputPerMillion: 1.2, OutputPerMillion: 8.4},
	}

	if err := repo.SaveAppSettings(value); err != nil {
		t.Fatalf("SaveAppSettings returned error: %v", err)
	}

	got, err := repo.GetAppSettings()
	if err != nil {
		t.Fatalf("GetAppSettings returned error: %v", err)
	}

	wantProvider := map[string]settings.PricingRule{
		"codex":  {InputPerMillion: 4.5, OutputPerMillion: 18},
		"openai": {InputPerMillion: 2.5, OutputPerMillion: 10},
	}
	if !reflect.DeepEqual(got.ProviderPricing, wantProvider) {
		t.Fatalf("ProviderPricing = %+v, want %+v", got.ProviderPricing, wantProvider)
	}

	wantAccount := map[string]settings.PricingRule{
		"12": {InputPerMillion: 1.2, OutputPerMillion: 8.4},
	}
	if !reflect.DeepEqual(got.AccountPricing, wantAccount) {
		t.Fatalf("AccountPricing = %+v, want %+v", got.AccountPricing, wantAccount)
	}
}
