package config_test

import (
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/config"
)

func TestLoadDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.ListenAddr != "127.0.0.1:6789" {
		t.Fatalf("ListenAddr = %q, want %q", cfg.ListenAddr, "127.0.0.1:6789")
	}
	if cfg.DatabasePath != "data/codex-router.sqlite" {
		t.Fatalf("DatabasePath = %q, want %q", cfg.DatabasePath, "data/codex-router.sqlite")
	}
	if cfg.SchedulerInterval != 5*time.Minute {
		t.Fatalf("SchedulerInterval = %s, want %s", cfg.SchedulerInterval, 5*time.Minute)
	}
}

func TestLoadRejectsShortEncryptionKey(t *testing.T) {
	t.Setenv("CODEX_ROUTER_ENCRYPTION_KEY", "short")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load returned nil error, want validation error")
	}
}

func TestLoadParsesValuesFromEnv(t *testing.T) {
	t.Setenv("CODEX_ROUTER_LISTEN_ADDR", "127.0.0.1:9999")
	t.Setenv("CODEX_ROUTER_DATABASE_PATH", "/tmp/codex-router.db")
	t.Setenv("CODEX_ROUTER_SCHEDULER_INTERVAL", "30s")
	t.Setenv("CODEX_ROUTER_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.ListenAddr != "127.0.0.1:9999" {
		t.Fatalf("ListenAddr = %q, want %q", cfg.ListenAddr, "127.0.0.1:9999")
	}
	if cfg.DatabasePath != "/tmp/codex-router.db" {
		t.Fatalf("DatabasePath = %q, want %q", cfg.DatabasePath, "/tmp/codex-router.db")
	}
	if cfg.SchedulerInterval != 30*time.Second {
		t.Fatalf("SchedulerInterval = %s, want %s", cfg.SchedulerInterval, 30*time.Second)
	}
	if cfg.EncryptionKey != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("EncryptionKey = %q, want expected value", cfg.EncryptionKey)
	}
}

func TestLoadRejectsNonLocalListenAddr(t *testing.T) {
	t.Setenv("CODEX_ROUTER_LISTEN_ADDR", "0.0.0.0:6789")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load returned nil error, want localhost validation error")
	}
}
