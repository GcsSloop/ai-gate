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
	"github.com/gcssloop/codex-router/backend/internal/secrets"
	"github.com/gcssloop/codex-router/backend/internal/settings"
	"github.com/gcssloop/codex-router/backend/internal/store/sqlite"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

type Config struct {
	ListenAddr        string
	DatabasePath      string
	SchedulerInterval time.Duration
	EncryptionKey     string
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

	var credentialCipher *secrets.Cipher
	if cfg.EncryptionKey != "" {
		credentialCipher, err = secrets.NewCipher(cfg.EncryptionKey)
		if err != nil {
			_ = store.Close()
			return nil, err
		}
	}

	accountRepo := accounts.NewSQLiteRepository(store.DB(), credentialCipher)
	settingsRepo := settings.NewSQLiteRepository(store.DB())
	usageRepo := usage.NewSQLiteRepository(store.DB())
	conversationRepo := conversations.NewSQLiteRepository(store.DB())
	policyRepo := policy.NewMemoryRepository()
	authConnector := auth.NewOAuthConnector(auth.Config{})
	stateStore := auth.NewStateStore(5 * time.Minute)
	accountsHandler := api.NewAccountsHandler(accountRepo, usageRepo, authConnector, stateStore)
	conversationsHandler := api.NewConversationsHandler(conversationRepo)

	apiMux := http.NewServeMux()
	apiMux.Handle("/accounts", accountsHandler)
	apiMux.Handle("/accounts/", accountsHandler)
	apiMux.Handle("/policy/", api.NewPolicyHandler(policyRepo))
	apiMux.Handle("/monitoring/overview", api.NewMonitoringHandler(accountRepo, usageRepo))
	apiMux.Handle("/dashboard/summary", api.NewDashboardHandler(conversationRepo))
	apiMux.Handle("/dashboard/account-stats", api.NewDashboardHandler(conversationRepo))
	apiMux.Handle("/conversations", conversationsHandler)
	apiMux.Handle("/conversations/", conversationsHandler)
	settingsHandler := api.NewSettingsHandler(
		settingsRepo,
		api.WithSettingsDatabase(store.DB(), cfg.DatabasePath),
		api.WithSettingsAccounts(accountRepo),
		api.WithSettingsCredentialCipher(credentialCipher),
	)
	apiMux.Handle("/settings/codex/backup", settingsHandler)
	apiMux.Handle("/settings/codex/backups", settingsHandler)
	apiMux.Handle("/settings/codex/backups/", settingsHandler)
	apiMux.Handle("/settings/codex/restore", settingsHandler)
	apiMux.Handle("/settings/app", settingsHandler)
	apiMux.Handle("/settings/failover-queue", settingsHandler)
	apiMux.Handle("/settings/database/sql-export", settingsHandler)
	apiMux.Handle("/settings/database/sql-import", settingsHandler)
	apiMux.Handle("/settings/database/json-export", settingsHandler)
	apiMux.Handle("/settings/database/json-import", settingsHandler)
	apiMux.Handle("/settings/database/backups", settingsHandler)
	apiMux.Handle("/settings/database/backup", settingsHandler)
	apiMux.Handle("/settings/database/restore", settingsHandler)
	apiMux.Handle("/settings/proxy/status", settingsHandler)
	apiMux.Handle("/settings/proxy/enable", settingsHandler)
	apiMux.Handle("/settings/proxy/disable", settingsHandler)
	gatewayHandler := api.NewGatewayHandler(accountRepo, usageRepo, conversationRepo, api.WithGatewaySettings(settingsRepo))
	responsesHandler := api.NewResponsesHandler(accountRepo, usageRepo, conversationRepo, api.WithResponsesSettings(settingsRepo))
	apiMux.Handle("/chat/completions", api.RequireProxyEnabled(gatewayHandler))
	apiMux.Handle("/v1/chat/completions", api.RequireProxyEnabled(gatewayHandler))
	apiMux.Handle("/responses", api.RequireProxyEnabled(responsesHandler))
	apiMux.Handle("/v1/responses", api.RequireProxyEnabled(responsesHandler))
	apiMux.Handle("/models", api.RequireProxyEnabled(responsesHandler))
	apiMux.Handle("/v1/models", api.RequireProxyEnabled(responsesHandler))

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/ai-router/webui/", http.StatusTemporaryRedirect)
			return
		}
		http.NotFound(w, r)
	})
	mux.Handle("/ai-router/api/", withCORS(http.StripPrefix("/ai-router/api", apiMux)))

	appCtx, cancel := context.WithCancel(context.Background())
	app := &App{listenAddr: cfg.ListenAddr, handler: mux, store: store, cancel: cancel}

	interval := cfg.SchedulerInterval
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	recoveryJob := scheduler.NewRecoveryJob(accountRepo, func(context.Context, accounts.Account) error {
		return nil
	}, interval)
	backupJob := scheduler.NewDBBackupJob(
		settingsRepo,
		settings.NewDBBackupManager(store.DB(), cfg.DatabasePath),
	)
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
				_ = backupJob.Run(appCtx, now.UTC())
			}
		}
	}()

	return app, nil
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
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
