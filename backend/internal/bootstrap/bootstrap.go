package bootstrap

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/api"
	"github.com/gcssloop/codex-router/backend/internal/auth"
	"github.com/gcssloop/codex-router/backend/internal/conversations"
	"github.com/gcssloop/codex-router/backend/internal/policy"
	"github.com/gcssloop/codex-router/backend/internal/store/sqlite"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

type Config struct {
	ListenAddr string
	DatabasePath string
}

type App struct {
	listenAddr string
	handler http.Handler
	store *sqlite.Store
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

	return &App{listenAddr: cfg.ListenAddr, handler: mux, store: store}, nil
}

func (a *App) ListenAddr() string {
	return a.listenAddr
}

func (a *App) Handler() http.Handler {
	return a.handler
}

func (a *App) Close() error {
	if a.store != nil {
		return a.store.Close()
	}
	return nil
}
