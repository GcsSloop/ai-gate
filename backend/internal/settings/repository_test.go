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
		LaunchAtLogin:           true,
		SilentStart:             true,
		CloseToTray:             false,
		ShowProxySwitchOnHome:   false,
		ShowHomeUpdateIndicator: false,
		ProxyHost:               "localhost",
		ProxyPort:               15721,
		AutoFailoverEnabled:     true,
		AutoBackupIntervalHours: 12,
		BackupRetentionCount:    7,
		Language:                "en-US",
		ThemeMode:               "dark",
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
