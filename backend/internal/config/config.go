package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultListenAddr        = "127.0.0.1:6789"
	defaultServerListenAddr  = "0.0.0.0:6789"
	defaultSchedulerInterval = 5 * time.Minute
	defaultHTTPPrefix        = "/ai-router"
	defaultServerHTTPPrefix  = "/ai-gate"
)

type Config struct {
	ListenAddr            string
	DatabasePath          string
	SchedulerInterval     time.Duration
	EncryptionKey         string
	ServerMode            bool
	HTTPPrefix            string
	ProxyEnabledByDefault bool
	SkipCodexConfig       bool
	ServerPassword        string
}

func Load(args ...string) (Config, error) {
	serverMode := serverModeRequested(args)
	defaultDatabasePath := resolveDefaultDatabasePath(serverMode)
	defaultAddr := defaultListenAddr
	defaultPrefix := defaultHTTPPrefix
	if serverMode {
		defaultAddr = defaultServerListenAddr
		defaultPrefix = defaultServerHTTPPrefix
	}
	cfg := Config{
		ListenAddr:            readString("CODEX_ROUTER_LISTEN_ADDR", defaultAddr),
		DatabasePath:          readString("CODEX_ROUTER_DATABASE_PATH", defaultDatabasePath),
		SchedulerInterval:     defaultSchedulerInterval,
		EncryptionKey:         os.Getenv("CODEX_ROUTER_ENCRYPTION_KEY"),
		ServerMode:            serverMode,
		HTTPPrefix:            normalizePrefix(readString("AI_GATE_HTTP_PREFIX", defaultPrefix)),
		ProxyEnabledByDefault: serverMode,
		SkipCodexConfig:       serverMode,
		ServerPassword:        os.Getenv("AI_GATE_SERVER_PASSWORD"),
	}

	if value := os.Getenv("CODEX_ROUTER_SCHEDULER_INTERVAL"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, err
		}
		cfg.SchedulerInterval = parsed
	}

	if cfg.EncryptionKey != "" && len(cfg.EncryptionKey) < 32 {
		return Config{}, errors.New("encryption key must be at least 32 characters")
	}
	if cfg.ServerMode && strings.TrimSpace(cfg.ServerPassword) == "" {
		return Config{}, errors.New("AI_GATE_SERVER_PASSWORD is required in server mode")
	}
	if err := validateLocalListenAddr(cfg.ListenAddr); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func serverModeRequested(args []string) bool {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("AI_GATE_MODE")), "server") {
		return true
	}
	for _, arg := range args {
		if arg == "--server" || arg == "-server" {
			return true
		}
	}
	return false
}

func resolveDefaultDatabasePath(serverMode bool) string {
	if serverMode {
		wd, err := os.Getwd()
		if err != nil || strings.TrimSpace(wd) == "" {
			return filepath.Join("data", "aigate.sqlite")
		}
		return filepath.Join(wd, "data", "aigate.sqlite")
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "data/aigate.sqlite"
	}
	return filepath.Join(home, ".aigate", "data", "aigate.sqlite")
}

func normalizePrefix(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "/" {
		return ""
	}
	trimmed = "/" + strings.Trim(trimmed, "/")
	return trimmed
}

func readString(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func validateLocalListenAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid listen addr %q: %w", addr, err)
	}
	normalized := strings.TrimSpace(host)
	switch normalized {
	case "127.0.0.1", "localhost", "::1", "0.0.0.0", "::":
		return nil
	default:
		return fmt.Errorf("listen addr %q is invalid, use 127.0.0.1/localhost/::1 for local-only or 0.0.0.0/:: for LAN sharing", addr)
	}
}
