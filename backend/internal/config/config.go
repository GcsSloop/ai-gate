package config

import (
	"errors"
	"os"
	"time"
)

const (
	defaultListenAddr        = "127.0.0.1:8080"
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

	return cfg, nil
}

func readString(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
