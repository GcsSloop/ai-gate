package main

import (
	"context"
	"log"
	"net/http"

	"github.com/gcssloop/codex-router/backend/internal/bootstrap"
	"github.com/gcssloop/codex-router/backend/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	app, err := bootstrap.NewApp(context.Background(), bootstrap.Config{
		ListenAddr:        cfg.ListenAddr,
		DatabasePath:      cfg.DatabasePath,
		SchedulerInterval: cfg.SchedulerInterval,
		EncryptionKey:     cfg.EncryptionKey,
	})
	if err != nil {
		log.Fatalf("create app: %v", err)
	}
	defer func() {
		if err := app.Close(); err != nil {
			log.Printf("close app: %v", err)
		}
	}()

	log.Printf("routerd listening on %s", app.ListenAddr())
	if err := http.ListenAndServe(app.ListenAddr(), app.Handler()); err != nil {
		log.Fatalf("serve http: %v", err)
	}
}
