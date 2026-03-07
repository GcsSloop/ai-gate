package bootstrap

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/api"
	"github.com/gcssloop/codex-router/backend/internal/auth"
	"github.com/gcssloop/codex-router/backend/internal/conversations"
	"github.com/gcssloop/codex-router/backend/internal/policy"
	"github.com/gcssloop/codex-router/backend/internal/scheduler"
	"github.com/gcssloop/codex-router/backend/internal/store/sqlite"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

type Config struct {
	ListenAddr        string
	DatabasePath      string
	SchedulerInterval time.Duration
}

type App struct {
	listenAddr string
	handler    http.Handler
	store      *sqlite.Store
	cancel     context.CancelFunc
	background sync.WaitGroup
}

func NewApp(_ context.Context, cfg Config) (*App, error) {
	if cfg.ListenAddr == "" {
		return nil, errors.New("listen address is required")
	}
	if cfg.DatabasePath == "" {
		return nil, errors.New("database path is required")
	}

	store, err := sqlite.Open(cfg.DatabasePath)
	if err != nil {
		return nil, err
	}

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	usageRepo := usage.NewSQLiteRepository(store.DB())
	conversationRepo := conversations.NewSQLiteRepository(store.DB())
	policyRepo := policy.NewMemoryRepository()
	authConnector := auth.NewOAuthConnector(auth.Config{})
	stateStore := auth.NewStateStore(5 * time.Minute)
	accountsHandler := api.NewAccountsHandler(accountRepo, authConnector, stateStore)
	conversationsHandler := api.NewConversationsHandler(conversationRepo)

	mux := http.NewServeMux()
	mux.Handle("/accounts", accountsHandler)
	mux.Handle("/accounts/", accountsHandler)
	mux.Handle("/policy/", api.NewPolicyHandler(policyRepo))
	mux.Handle("/monitoring/overview", api.NewMonitoringHandler(accountRepo, usageRepo))
	mux.Handle("/conversations", conversationsHandler)
	mux.Handle("/conversations/", conversationsHandler)
	mux.Handle("/v1/chat/completions", api.NewGatewayHandler(accountRepo, usageRepo, conversationRepo))

	appCtx, cancel := context.WithCancel(context.Background())
	app := &App{listenAddr: cfg.ListenAddr, handler: mux, store: store, cancel: cancel}

	interval := cfg.SchedulerInterval
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	recoveryJob := scheduler.NewRecoveryJob(accountRepo, func(context.Context, accounts.Account) error {
		return nil
	}, interval)
	app.background.Add(1)
	go func() {
		defer app.background.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-appCtx.Done():
				return
			case now := <-ticker.C:
				_ = recoveryJob.Run(appCtx, now.UTC())
			}
		}
	}()

	return app, nil
}

func (a *App) ListenAddr() string {
	return a.listenAddr
}

func (a *App) Handler() http.Handler {
	return a.handler
}

func (a *App) Close() error {
	if a.cancel != nil {
		a.cancel()
	}
	a.background.Wait()
	if a.store != nil {
		return a.store.Close()
	}
	return nil
}
