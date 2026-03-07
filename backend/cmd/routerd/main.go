package main

import (
	"context"
	"log"

	"github.com/gcssloop/codex-router/backend/internal/bootstrap"
	"github.com/gcssloop/codex-router/backend/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	app, err := bootstrap.NewApp(context.Background(), bootstrap.Config{
		ListenAddr: cfg.ListenAddr,
	})
	if err != nil {
		log.Fatalf("create app: %v", err)
	}

	log.Printf("routerd initialized on %s", app.ListenAddr())
}
