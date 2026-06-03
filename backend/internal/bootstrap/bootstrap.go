package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accountdrv"
	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/api"
	"github.com/gcssloop/codex-router/backend/internal/auth"
	"github.com/gcssloop/codex-router/backend/internal/conversations"
	"github.com/gcssloop/codex-router/backend/internal/netproxy"
	"github.com/gcssloop/codex-router/backend/internal/policy"
	"github.com/gcssloop/codex-router/backend/internal/scheduler"
	"github.com/gcssloop/codex-router/backend/internal/secrets"
	"github.com/gcssloop/codex-router/backend/internal/settings"
	"github.com/gcssloop/codex-router/backend/internal/store/sqlite"
	"github.com/gcssloop/codex-router/backend/internal/usage"
	"github.com/gcssloop/codex-router/backend/internal/usage/refresh"
	"github.com/gcssloop/codex-router/backend/internal/usagedrv"
	"github.com/gcssloop/codex-router/backend/internal/usagedrv/builtin"
	luadrv "github.com/gcssloop/codex-router/backend/internal/usagedrv/lua"
	"github.com/gcssloop/codex-router/backend/internal/usagedrv/registry"
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

type backgroundTask func(context.Context, time.Time)

func NewApp(_ context.Context, cfg Config) (*App, error) {
	if cfg.ListenAddr == "" {
		return nil, errors.New("listen address is required")
	}
	if cfg.DatabasePath == "" {
		return nil, errors.New("database path is required")
	}
	luaScriptRoot := filepath.Join(filepath.Dir(cfg.DatabasePath), "usage-scripts")

	store, err := sqlite.Open(cfg.DatabasePath)
	if err != nil {
		return nil, err
	}
	if err := cleanupLegacyAuditData(store.DB()); err != nil {
		_ = store.Close()
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
	upstreamHTTPClient := netproxy.NewHTTPClient(settingsRepo)
	conversationRepo := conversations.NewSQLiteRepository(store.DB())
	policyRepo := policy.NewMemoryRepository()
	authConnector := auth.NewOAuthConnector(auth.Config{
		ClientID:              "app_EMoamEEZ73f0CkXaXp7hrann",
		TokenURL:              "https://auth.openai.com/oauth/token",
		DeviceAuthUserCodeURL: "https://auth.openai.com/api/accounts/deviceauth/usercode",
		DeviceAuthTokenURL:    "https://auth.openai.com/api/accounts/deviceauth/token",
		DeviceRedirectURL:     "https://auth.openai.com/deviceauth/callback",
		DeviceVerificationURL: "https://auth.openai.com/codex/device",
	})
	stateStore := auth.NewStateStore(5 * time.Minute)
	stateEvents := api.NewStateEventBus()
	driverRegistry, err := registry.New(
		[]accountdrv.AccountDriver{
			accountdrv.NewOfficialDriver(upstreamHTTPClient, accountRepo),
			accountdrv.NewAPIKeyDriver(),
		},
		[]usagedrv.UsageDriver{
			builtin.NewOpenAIOfficialDriver(upstreamHTTPClient),
			builtin.NewPPChatDriver(upstreamHTTPClient),
			luadrv.NewDriver(upstreamHTTPClient, "", luadrv.WithManagedScriptRoot(luaScriptRoot)),
		},
	)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	refreshOrchestrator := refresh.NewOrchestratorWithSettings(accountRepo, usageRepo, driverRegistry, settingsRepo)
	accountsHandler := api.NewAccountsHandler(
		accountRepo,
		usageRepo,
		authConnector,
		stateStore,
		api.WithAccountsStateEvents(stateEvents),
		api.WithAccountsUsageRefresher(refreshOrchestrator),
		api.WithAccountsSettings(settingsRepo),
		api.WithAccountsHTTPClient(upstreamHTTPClient),
		api.WithAccountsDriverRegistry(driverRegistry),
		api.WithAccountsLuaScriptRoot(luaScriptRoot),
	)
	conversationsHandler := api.NewConversationsHandler(conversationRepo)

	apiMux := http.NewServeMux()
	apiMux.Handle("/accounts", accountsHandler)
	apiMux.Handle("/accounts/", accountsHandler)
	apiMux.Handle("/policy/", api.NewPolicyHandler(policyRepo))
	apiMux.Handle("/monitoring/overview", api.NewMonitoringHandler(accountRepo, usageRepo))
	dashboardHandler := api.NewDashboardHandler(usageRepo, api.WithDashboardStateEvents(stateEvents))
	apiMux.Handle("/dashboard/summary", dashboardHandler)
	apiMux.Handle("/dashboard/trends", dashboardHandler)
	apiMux.Handle("/dashboard/recent-events", dashboardHandler)
	apiMux.Handle("/dashboard/model-distribution", dashboardHandler)
	apiMux.Handle("/dashboard/state-events", dashboardHandler)
	apiMux.Handle("/conversations", conversationsHandler)
	apiMux.Handle("/conversations/", conversationsHandler)
	settingsHandler := api.NewSettingsHandler(
		settingsRepo,
		api.WithSettingsDatabase(store.DB(), cfg.DatabasePath),
		api.WithSettingsAccounts(accountRepo),
		api.WithSettingsCredentialCipher(credentialCipher),
	)
	toolingHandler := api.NewToolingHandler(api.WithToolingHTTPClient(upstreamHTTPClient))
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
	apiMux.Handle("/settings/database/backups/", settingsHandler)
	apiMux.Handle("/settings/database/backup", settingsHandler)
	apiMux.Handle("/settings/database/restore", settingsHandler)
	apiMux.Handle("/settings/audit-storage/optimize", settingsHandler)
	apiMux.Handle("/settings/proxy/status", settingsHandler)
	apiMux.Handle("/settings/proxy/enable", settingsHandler)
	apiMux.Handle("/settings/proxy/disable", settingsHandler)
	apiMux.Handle("/tooling/state", toolingHandler)
	apiMux.Handle("/tooling/settings", toolingHandler)
	apiMux.Handle("/tooling/skills/import", toolingHandler)
	apiMux.Handle("/tooling/skills/apply", toolingHandler)
	apiMux.Handle("/tooling/skills/", toolingHandler)
	apiMux.Handle("/tooling/skills/repos", toolingHandler)
	apiMux.Handle("/tooling/skills/repos/search", toolingHandler)
	apiMux.Handle("/tooling/skills/repos/", toolingHandler)
	apiMux.Handle("/tooling/mcp/state", toolingHandler)
	apiMux.Handle("/tooling/mcp/templates", toolingHandler)
	apiMux.Handle("/tooling/mcp/servers", toolingHandler)
	apiMux.Handle("/tooling/mcp/servers/", toolingHandler)
	apiMux.Handle("/tooling/mcp/import", toolingHandler)
	apiMux.Handle("/tooling/mcp/install", toolingHandler)
	apiMux.Handle("/tooling/mcp/apply", toolingHandler)
	gatewayHandler := api.NewGatewayHandler(
		accountRepo,
		usageRepo,
		conversationRepo,
		api.WithGatewaySettings(settingsRepo),
		api.WithGatewayHTTPClient(upstreamHTTPClient),
		api.WithGatewayStateEvents(stateEvents),
	)
	responsesHandler := api.NewResponsesHandler(
		accountRepo,
		usageRepo,
		conversationRepo,
		api.WithResponsesSettings(settingsRepo),
		api.WithResponsesHTTPClient(upstreamHTTPClient),
		api.WithResponsesStateEvents(stateEvents),
	)
	apiMux.Handle("/chat/completions", api.RequireProxyEnabled(gatewayHandler))
	apiMux.Handle("/v1/chat/completions", api.RequireProxyEnabled(gatewayHandler))
	apiMux.Handle("/responses", api.RequireProxyEnabled(responsesHandler))
	apiMux.Handle("/responses/", api.RequireProxyEnabled(responsesHandler))
	apiMux.Handle("/v1/responses", api.RequireProxyEnabled(responsesHandler))
	apiMux.Handle("/v1/responses/", api.RequireProxyEnabled(responsesHandler))
	apiMux.Handle("/models", api.RequireProxyEnabled(responsesHandler))
	apiMux.Handle("/models/", api.RequireProxyEnabled(responsesHandler))
	apiMux.Handle("/v1/models", api.RequireProxyEnabled(responsesHandler))
	apiMux.Handle("/v1/models/", api.RequireProxyEnabled(responsesHandler))

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/ai-router/webui/", http.StatusTemporaryRedirect)
			return
		}
		http.NotFound(w, r)
	})
	mux.Handle("/ai-router/api/", withCORS(withLANShareAccessControl(settingsRepo, http.StripPrefix("/ai-router/api", apiMux))))

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
	compactionJob := scheduler.NewUsageCompactionJob(func(_ context.Context, now time.Time) error {
		return usageRepo.CompactEvents(now.UTC())
	})
	app.background.Add(1)
	go func() {
		defer app.background.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		startupTasks := []backgroundTask{
			loggedBackgroundTask("account recovery", recoveryJob.Run),
			loggedBackgroundTask("usage refresh", refreshOrchestrator.Run),
		}
		tasks := []backgroundTask{
			loggedBackgroundTask("account recovery", recoveryJob.Run),
			loggedBackgroundTask("usage refresh", refreshOrchestrator.Run),
			loggedBackgroundTask("usage compaction", compactionJob.Run),
			loggedBackgroundTask("database backup", backupJob.Run),
		}
		runBackgroundCycle(appCtx, time.Now().UTC(), startupTasks...)
		runBackgroundLoop(appCtx, ticker.C, tasks...)
	}()

	return app, nil
}

func loggedBackgroundTask(name string, task func(context.Context, time.Time) error) backgroundTask {
	return func(ctx context.Context, runAt time.Time) {
		if task == nil {
			return
		}
		if err := task(ctx, runAt.UTC()); err != nil {
			log.Printf("%s failed: %v", name, err)
		}
	}
}

func runBackgroundLoop(ctx context.Context, ticks <-chan time.Time, tasks ...backgroundTask) {
	for {
		select {
		case <-ctx.Done():
			return
		case now, ok := <-ticks:
			if !ok {
				return
			}
			runBackgroundCycle(ctx, now.UTC(), tasks...)
		}
	}
}

func runBackgroundCycle(ctx context.Context, now time.Time, tasks ...backgroundTask) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("background scheduler panic recovered: %v\n%s", recovered, debug.Stack())
		}
	}()
	for _, task := range tasks {
		if task != nil {
			task(ctx, now.UTC())
		}
	}
}

func cleanupLegacyAuditData(db *sql.DB) error {
	const cleanupKey = "audit_cleanup_v1"

	var existing string
	switch err := db.QueryRow(`SELECT value FROM maintenance_state WHERE key = ?`, cleanupKey).Scan(&existing); err {
	case nil:
		return nil
	case sql.ErrNoRows:
	default:
		return fmt.Errorf("query maintenance state: %w", err)
	}

	for _, statement := range []string{
		`DELETE FROM messages`,
		`DELETE FROM runs`,
		`DELETE FROM conversations`,
	} {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("cleanup legacy audit data: %w", err)
		}
	}
	if _, err := db.Exec(`VACUUM`); err != nil {
		return fmt.Errorf("vacuum legacy audit data: %w", err)
	}
	if _, err := db.Exec(
		`INSERT INTO maintenance_state (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`,
		cleanupKey,
		"done",
	); err != nil {
		return fmt.Errorf("mark maintenance state: %w", err)
	}
	return nil
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

func withLANShareAccessControl(repo settings.ReadRepository, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if repo == nil {
			next.ServeHTTP(w, r)
			return
		}

		appSettings, err := repo.GetAppSettings()
		if err != nil || !appSettings.LANShareEnabled {
			next.ServeHTTP(w, r)
			return
		}

		remoteIP, err := remoteIPFromAddr(r.RemoteAddr)
		if err != nil {
			http.Error(w, "invalid remote address", http.StatusForbidden)
			return
		}
		if remoteIP.IsLoopback() {
			next.ServeHTTP(w, r)
			return
		}
		if appSettings.LANShareWhitelistEnabled {
			if strings.TrimSpace(appSettings.LANShareIPWhitelist) == "" {
				http.Error(w, "remote address is not allowed by lan share whitelist", http.StatusForbidden)
				return
			}
			allowed, err := ipAllowedByWhitelist(remoteIP, appSettings.LANShareIPWhitelist)
			if err != nil {
				log.Printf("lan share whitelist parse failed: %v", err)
				http.Error(w, "lan share whitelist is invalid", http.StatusForbidden)
				return
			}
			if !allowed {
				http.Error(w, "remote address is not allowed by lan share whitelist", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func remoteIPFromAddr(addr string) (net.IP, error) {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil, fmt.Errorf("parse remote ip: %q", host)
	}
	return ip, nil
}

func ipAllowedByWhitelist(ip net.IP, raw string) (bool, error) {
	if strings.TrimSpace(raw) == "" {
		return true, nil
	}
	for _, entry := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';'
	}) {
		normalized := strings.TrimSpace(entry)
		if normalized == "" {
			continue
		}
		if strings.EqualFold(normalized, "localhost") {
			continue
		}
		if allowedIP := net.ParseIP(normalized); allowedIP != nil {
			if allowedIP.Equal(ip) {
				return true, nil
			}
			continue
		}
		_, network, err := net.ParseCIDR(normalized)
		if err != nil {
			return false, err
		}
		if network.Contains(ip) {
			return true, nil
		}
	}
	return false, nil
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
