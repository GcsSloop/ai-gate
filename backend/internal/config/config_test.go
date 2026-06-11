package config_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/config"
)

func TestLoadDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.ListenAddr != "127.0.0.1:6789" {
		t.Fatalf("ListenAddr = %q, want %q", cfg.ListenAddr, "127.0.0.1:6789")
	}
	expectedDefault := filepath.Join(home, ".aigate", "data", "aigate.sqlite")
	if cfg.DatabasePath != expectedDefault {
		t.Fatalf("DatabasePath = %q, want %q", cfg.DatabasePath, expectedDefault)
	}
	if cfg.SchedulerInterval != 5*time.Minute {
		t.Fatalf("SchedulerInterval = %s, want %s", cfg.SchedulerInterval, 5*time.Minute)
	}
	if cfg.ServerMode {
		t.Fatal("ServerMode = true, want false by default")
	}
	if cfg.HTTPPrefix != "/ai-router" {
		t.Fatalf("HTTPPrefix = %q, want %q", cfg.HTTPPrefix, "/ai-router")
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

func TestLoadAllowsLANShareListenAddr(t *testing.T) {
	t.Setenv("CODEX_ROUTER_LISTEN_ADDR", "0.0.0.0:6789")
	t.Setenv("CODEX_ROUTER_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.ListenAddr != "0.0.0.0:6789" {
		t.Fatalf("ListenAddr = %q, want %q", cfg.ListenAddr, "0.0.0.0:6789")
	}
}

func TestLoadServerModeDefaults(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("AI_GATE_MODE", "server")
	t.Setenv("CODEX_ROUTER_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if !cfg.ServerMode {
		t.Fatal("ServerMode = false, want true")
	}
	if cfg.ListenAddr != "0.0.0.0:6789" {
		t.Fatalf("ListenAddr = %q, want %q", cfg.ListenAddr, "0.0.0.0:6789")
	}
	expectedDB := filepath.Join(root, "data", "aigate.sqlite")
	if cfg.DatabasePath != expectedDB {
		t.Fatalf("DatabasePath = %q, want %q", cfg.DatabasePath, expectedDB)
	}
	if cfg.HTTPPrefix != "/ai-gate" {
		t.Fatalf("HTTPPrefix = %q, want %q", cfg.HTTPPrefix, "/ai-gate")
	}
	if !cfg.ProxyEnabledByDefault {
		t.Fatal("ProxyEnabledByDefault = false, want true")
	}
	if !cfg.SkipCodexConfig {
		t.Fatal("SkipCodexConfig = false, want true")
	}
}

func TestLoadServerModeFromArgs(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("CODEX_ROUTER_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")

	cfg, err := config.Load("--server")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !cfg.ServerMode {
		t.Fatal("ServerMode = false, want true")
	}
}
