package bootstrap

import (
	"context"
	"errors"
)

type Config struct {
	ListenAddr string
}

type App struct {
	listenAddr string
}

func NewApp(_ context.Context, cfg Config) (*App, error) {
	if cfg.ListenAddr == "" {
		return nil, errors.New("listen address is required")
	}

	return &App{listenAddr: cfg.ListenAddr}, nil
}

func (a *App) ListenAddr() string {
	return a.listenAddr
}
