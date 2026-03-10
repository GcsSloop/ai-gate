package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/bootstrap"
	"github.com/gcssloop/codex-router/backend/internal/config"
)

const (
	parentHeartbeatEnv       = "CODEX_ROUTER_PARENT_HEARTBEAT"
	parentHeartbeatStdinMode = "stdin"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	appCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app, err := bootstrap.NewApp(appCtx, bootstrap.Config{
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

	if os.Getenv(parentHeartbeatEnv) == parentHeartbeatStdinMode {
		go monitorParentHeartbeat(appCtx, cancel, os.Stdin, 5*time.Second, 1*time.Second)
	}

	server := &http.Server{Addr: app.ListenAddr(), Handler: app.Handler()}
	go func() {
		<-appCtx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("shutdown server: %v", err)
		}
	}()

	log.Printf("routerd listening on %s", app.ListenAddr())
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("serve http: %v", err)
		return
	}
}
