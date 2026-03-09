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
	defaultSchedulerInterval = 5 * time.Minute
)

type Config struct {
	ListenAddr        string
	DatabasePath      string
	SchedulerInterval time.Duration
	EncryptionKey     string
	ThinGatewayMode   bool
}

func Load() (Config, error) {
	defaultDatabasePath := resolveDefaultDatabasePath()
		cfg := Config{
			ListenAddr:        readString("CODEX_ROUTER_LISTEN_ADDR", defaultListenAddr),
			DatabasePath:      readString("CODEX_ROUTER_DATABASE_PATH", defaultDatabasePath),
			SchedulerInterval: defaultSchedulerInterval,
			EncryptionKey:     os.Getenv("CODEX_ROUTER_ENCRYPTION_KEY"),
			ThinGatewayMode:   readBool("THIN_GATEWAY_MODE", true),
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
	if err := validateLocalListenAddr(cfg.ListenAddr); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func resolveDefaultDatabasePath() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "data/aigate.sqlite"
	}
	return filepath.Join(home, ".aigate", "data", "aigate.sqlite")
}

func readString(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func readBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	case "":
		return fallback
	default:
		return fallback
	}
}

func validateLocalListenAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid listen addr %q: %w", addr, err)
	}
	normalized := strings.TrimSpace(host)
	switch normalized {
	case "127.0.0.1", "localhost", "::1":
		return nil
	default:
		return fmt.Errorf("listen addr %q is not local-only, use 127.0.0.1/localhost/::1", addr)
	}
}
