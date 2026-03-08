package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

const (
	defaultListenAddr        = "127.0.0.1:6789"
	defaultDatabasePath      = "data/codex-router.sqlite"
	defaultSchedulerInterval = 5 * time.Minute
)

type Config struct {
	ListenAddr        string
	DatabasePath      string
	SchedulerInterval time.Duration
	EncryptionKey     string
}

func Load() (Config, error) {
	cfg := Config{
		ListenAddr:        readString("CODEX_ROUTER_LISTEN_ADDR", defaultListenAddr),
		DatabasePath:      readString("CODEX_ROUTER_DATABASE_PATH", defaultDatabasePath),
		SchedulerInterval: defaultSchedulerInterval,
		EncryptionKey:     os.Getenv("CODEX_ROUTER_ENCRYPTION_KEY"),
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
	case "127.0.0.1", "localhost", "::1":
		return nil
	default:
		return fmt.Errorf("listen addr %q is not local-only, use 127.0.0.1/localhost/::1", addr)
	}
}
