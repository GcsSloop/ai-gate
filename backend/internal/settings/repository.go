package settings

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
)

type PricingRule struct {
	InputPerMillion  float64 `json:"input_per_million"`
	OutputPerMillion float64 `json:"output_per_million"`
}

const (
	UpstreamProxyModeSystem = "system"
	UpstreamProxyModeDirect = "direct"
	UpstreamProxyModeManual = "manual"
)

type AppSettings struct {
	LaunchAtLogin                bool                   `json:"launch_at_login"`
	SilentStart                  bool                   `json:"silent_start"`
	CloseToTray                  bool                   `json:"close_to_tray"`
	ShowProxySwitchOnHome        bool                   `json:"show_proxy_switch_on_home"`
	ShowHomeUpdateIndicator      bool                   `json:"show_home_update_indicator"`
	StatusRefreshIntervalSeconds int                    `json:"status_refresh_interval_seconds"`
	UsageRequestTimeoutSeconds   int                    `json:"usage_request_timeout_seconds"`
	ProxyHost                    string                 `json:"proxy_host"`
	ProxyPort                    int                    `json:"proxy_port"`
	LANShareEnabled              bool                   `json:"lan_share_enabled"`
	LANShareWhitelistEnabled     bool                   `json:"lan_share_whitelist_enabled"`
	LANShareIPWhitelist          string                 `json:"lan_share_ip_whitelist"`
	UpstreamProxyMode            string                 `json:"upstream_proxy_mode"`
	UpstreamProxyURL             string                 `json:"upstream_proxy_url"`
	UpstreamProxyUsername        string                 `json:"upstream_proxy_username"`
	UpstreamProxyPassword        string                 `json:"upstream_proxy_password"`
	UpstreamSkipTLSVerify        bool                   `json:"upstream_skip_tls_verify"`
	AutoFailoverEnabled          bool                   `json:"auto_failover_enabled"`
	AutoBackupIntervalHours      int                    `json:"auto_backup_interval_hours"`
	BackupRetentionCount         int                    `json:"backup_retention_count"`
	Language                     string                 `json:"language"`
	ThemeMode                    string                 `json:"theme_mode"`
	ProviderPricing              map[string]PricingRule `json:"provider_pricing"`
	AccountPricing               map[string]PricingRule `json:"account_pricing"`
}

type ReadRepository interface {
	GetAppSettings() (AppSettings, error)
	ListFailoverQueue() ([]int64, error)
}

type Repository interface {
	ReadRepository
	SaveAppSettings(AppSettings) error
	SaveFailoverQueue([]int64) error
}

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(db *sql.DB) *SQLiteRepository {
	return &SQLiteRepository{db: db}
}

func DefaultAppSettings() AppSettings {
	return AppSettings{
		CloseToTray:                  true,
		ShowProxySwitchOnHome:        true,
		ShowHomeUpdateIndicator:      true,
		StatusRefreshIntervalSeconds: 60,
		UsageRequestTimeoutSeconds:   15,
		ProxyHost:                    "127.0.0.1",
		ProxyPort:                    6789,
		LANShareEnabled:              false,
		LANShareWhitelistEnabled:     false,
		LANShareIPWhitelist:          "",
		UpstreamProxyMode:            UpstreamProxyModeSystem,
		AutoFailoverEnabled:          true,
		AutoBackupIntervalHours:      24,
		BackupRetentionCount:         10,
		Language:                     "zh-CN",
		ThemeMode:                    "system",
	}
}

func (r *SQLiteRepository) GetAppSettings() (AppSettings, error) {
	row := r.db.QueryRow(
		`SELECT launch_at_login, silent_start, close_to_tray, show_proxy_switch_on_home, show_home_update_indicator, status_refresh_interval_seconds, usage_request_timeout_seconds, proxy_host, proxy_port, lan_share_enabled, lan_share_whitelist_enabled, lan_share_ip_whitelist,
		        upstream_proxy_mode, upstream_proxy_url, upstream_proxy_username, upstream_proxy_password, upstream_skip_tls_verify, auto_failover_enabled, auto_backup_interval_hours, backup_retention_count,
		        language, theme_mode, provider_pricing, account_pricing
		 FROM app_settings WHERE id = 1`,
	)

	var launchAtLogin int
	var silentStart int
	var closeToTray int
	var showProxySwitchOnHome int
	var showHomeUpdateIndicator int
	var statusRefreshIntervalSeconds int
	var usageRequestTimeoutSeconds int
	var proxyHost string
	var proxyPort int
	var lanShareEnabled int
	var lanShareWhitelistEnabled int
	var lanShareIPWhitelist string
	var upstreamProxyMode string
	var upstreamProxyURL string
	var upstreamProxyUsername string
	var upstreamProxyPassword string
	var upstreamSkipTLSVerify int
	var autoFailoverEnabled int
	var autoBackupIntervalHours int
	var backupRetentionCount int
	var language string
	var themeMode string
	var providerPricingJSON string
	var accountPricingJSON string

	if err := row.Scan(
		&launchAtLogin,
		&silentStart,
		&closeToTray,
		&showProxySwitchOnHome,
		&showHomeUpdateIndicator,
		&statusRefreshIntervalSeconds,
		&usageRequestTimeoutSeconds,
		&proxyHost,
		&proxyPort,
		&lanShareEnabled,
		&lanShareWhitelistEnabled,
		&lanShareIPWhitelist,
		&upstreamProxyMode,
		&upstreamProxyURL,
		&upstreamProxyUsername,
		&upstreamProxyPassword,
		&upstreamSkipTLSVerify,
		&autoFailoverEnabled,
		&autoBackupIntervalHours,
		&backupRetentionCount,
		&language,
		&themeMode,
		&providerPricingJSON,
		&accountPricingJSON,
	); err != nil {
		if err == sql.ErrNoRows {
			return DefaultAppSettings(), nil
		}
		return AppSettings{}, fmt.Errorf("select app settings: %w", err)
	}

	providerPricing, err := decodePricingMap(providerPricingJSON)
	if err != nil {
		return AppSettings{}, fmt.Errorf("decode provider pricing: %w", err)
	}
	accountPricing, err := decodePricingMap(accountPricingJSON)
	if err != nil {
		return AppSettings{}, fmt.Errorf("decode account pricing: %w", err)
	}

	return sanitize(AppSettings{
		LaunchAtLogin:                launchAtLogin == 1,
		SilentStart:                  silentStart == 1,
		CloseToTray:                  closeToTray == 1,
		ShowProxySwitchOnHome:        showProxySwitchOnHome == 1,
		ShowHomeUpdateIndicator:      showHomeUpdateIndicator == 1,
		StatusRefreshIntervalSeconds: statusRefreshIntervalSeconds,
		UsageRequestTimeoutSeconds:   usageRequestTimeoutSeconds,
		ProxyHost:                    proxyHost,
		ProxyPort:                    proxyPort,
		LANShareEnabled:              lanShareEnabled == 1,
		LANShareWhitelistEnabled:     lanShareWhitelistEnabled == 1,
		LANShareIPWhitelist:          lanShareIPWhitelist,
		UpstreamProxyMode:            upstreamProxyMode,
		UpstreamProxyURL:             upstreamProxyURL,
		UpstreamProxyUsername:        upstreamProxyUsername,
		UpstreamProxyPassword:        upstreamProxyPassword,
		UpstreamSkipTLSVerify:        upstreamSkipTLSVerify == 1,
		AutoFailoverEnabled:          autoFailoverEnabled == 1,
		AutoBackupIntervalHours:      autoBackupIntervalHours,
		BackupRetentionCount:         backupRetentionCount,
		Language:                     language,
		ThemeMode:                    themeMode,
		ProviderPricing:              providerPricing,
		AccountPricing:               accountPricing,
	}), nil
}

func (r *SQLiteRepository) SaveAppSettings(value AppSettings) error {
	value = sanitize(value)
	providerPricingJSON, err := encodePricingMap(value.ProviderPricing)
	if err != nil {
		return fmt.Errorf("encode provider pricing: %w", err)
	}
	accountPricingJSON, err := encodePricingMap(value.AccountPricing)
	if err != nil {
		return fmt.Errorf("encode account pricing: %w", err)
	}
	_, err = r.db.Exec(
		`INSERT INTO app_settings (
			id, launch_at_login, silent_start, close_to_tray, show_proxy_switch_on_home, show_home_update_indicator, status_refresh_interval_seconds, usage_request_timeout_seconds, proxy_host, proxy_port, lan_share_enabled, lan_share_whitelist_enabled, lan_share_ip_whitelist,
			upstream_proxy_mode, upstream_proxy_url, upstream_proxy_username, upstream_proxy_password, upstream_skip_tls_verify, auto_failover_enabled, auto_backup_interval_hours, backup_retention_count,
			language, theme_mode, provider_pricing, account_pricing, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			launch_at_login = excluded.launch_at_login,
			silent_start = excluded.silent_start,
			close_to_tray = excluded.close_to_tray,
			show_proxy_switch_on_home = excluded.show_proxy_switch_on_home,
			show_home_update_indicator = excluded.show_home_update_indicator,
			status_refresh_interval_seconds = excluded.status_refresh_interval_seconds,
			usage_request_timeout_seconds = excluded.usage_request_timeout_seconds,
			proxy_host = excluded.proxy_host,
			proxy_port = excluded.proxy_port,
			lan_share_enabled = excluded.lan_share_enabled,
			lan_share_whitelist_enabled = excluded.lan_share_whitelist_enabled,
			lan_share_ip_whitelist = excluded.lan_share_ip_whitelist,
			upstream_proxy_mode = excluded.upstream_proxy_mode,
			upstream_proxy_url = excluded.upstream_proxy_url,
			upstream_proxy_username = excluded.upstream_proxy_username,
			upstream_proxy_password = excluded.upstream_proxy_password,
			upstream_skip_tls_verify = excluded.upstream_skip_tls_verify,
			auto_failover_enabled = excluded.auto_failover_enabled,
			auto_backup_interval_hours = excluded.auto_backup_interval_hours,
			backup_retention_count = excluded.backup_retention_count,
			language = excluded.language,
			theme_mode = excluded.theme_mode,
			provider_pricing = excluded.provider_pricing,
			account_pricing = excluded.account_pricing,
			updated_at = CURRENT_TIMESTAMP`,
		1,
		boolToInt(value.LaunchAtLogin),
		boolToInt(value.SilentStart),
		boolToInt(value.CloseToTray),
		boolToInt(value.ShowProxySwitchOnHome),
		boolToInt(value.ShowHomeUpdateIndicator),
		value.StatusRefreshIntervalSeconds,
		value.UsageRequestTimeoutSeconds,
		value.ProxyHost,
		value.ProxyPort,
		boolToInt(value.LANShareEnabled),
		boolToInt(value.LANShareWhitelistEnabled),
		value.LANShareIPWhitelist,
		value.UpstreamProxyMode,
		value.UpstreamProxyURL,
		value.UpstreamProxyUsername,
		value.UpstreamProxyPassword,
		boolToInt(value.UpstreamSkipTLSVerify),
		boolToInt(value.AutoFailoverEnabled),
		value.AutoBackupIntervalHours,
		value.BackupRetentionCount,
		value.Language,
		value.ThemeMode,
		providerPricingJSON,
		accountPricingJSON,
	)
	if err != nil {
		return fmt.Errorf("upsert app settings: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) ListFailoverQueue() ([]int64, error) {
	rows, err := r.db.Query(`SELECT account_id FROM failover_queue_items ORDER BY position ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("query failover queue: %w", err)
	}
	defer rows.Close()

	var accountIDs []int64
	for rows.Next() {
		var accountID int64
		if err := rows.Scan(&accountID); err != nil {
			return nil, fmt.Errorf("scan failover queue: %w", err)
		}
		accountIDs = append(accountIDs, accountID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate failover queue: %w", err)
	}
	return accountIDs, nil
}

func (r *SQLiteRepository) SaveFailoverQueue(accountIDs []int64) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin save failover queue: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.Exec(`DELETE FROM failover_queue_items`); err != nil {
		return fmt.Errorf("clear failover queue: %w", err)
	}
	for index, accountID := range accountIDs {
		if _, err = tx.Exec(
			`INSERT INTO failover_queue_items (account_id, position) VALUES (?, ?)`,
			accountID,
			index,
		); err != nil {
			return fmt.Errorf("insert failover queue item: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit failover queue: %w", err)
	}
	return nil
}

func sanitize(value AppSettings) AppSettings {
	defaults := DefaultAppSettings()
	if value.ProxyHost == "" {
		value.ProxyHost = defaults.ProxyHost
	}
	if value.ProxyPort <= 0 {
		value.ProxyPort = defaults.ProxyPort
	}
	value.LANShareIPWhitelist = sanitizeIPWhitelist(value.LANShareIPWhitelist)
	switch value.UpstreamProxyMode {
	case UpstreamProxyModeSystem, UpstreamProxyModeDirect, UpstreamProxyModeManual:
	default:
		value.UpstreamProxyMode = defaults.UpstreamProxyMode
	}
	value.UpstreamProxyURL = sanitizeOptionalString(value.UpstreamProxyURL)
	value.UpstreamProxyUsername = sanitizeOptionalString(value.UpstreamProxyUsername)
	value.UpstreamProxyPassword = sanitizeOptionalString(value.UpstreamProxyPassword)
	if value.UpstreamProxyMode != UpstreamProxyModeManual {
		value.UpstreamProxyURL = ""
		value.UpstreamProxyUsername = ""
		value.UpstreamProxyPassword = ""
	}
	if value.AutoBackupIntervalHours <= 0 {
		value.AutoBackupIntervalHours = defaults.AutoBackupIntervalHours
	}
	if value.BackupRetentionCount <= 0 {
		value.BackupRetentionCount = defaults.BackupRetentionCount
	}
	if value.StatusRefreshIntervalSeconds <= 0 {
		value.StatusRefreshIntervalSeconds = defaults.StatusRefreshIntervalSeconds
	}
	if value.StatusRefreshIntervalSeconds < 5 {
		value.StatusRefreshIntervalSeconds = 5
	}
	if value.StatusRefreshIntervalSeconds > 3600 {
		value.StatusRefreshIntervalSeconds = 3600
	}
	if value.UsageRequestTimeoutSeconds <= 0 {
		value.UsageRequestTimeoutSeconds = defaults.UsageRequestTimeoutSeconds
	}
	if value.UsageRequestTimeoutSeconds < 3 {
		value.UsageRequestTimeoutSeconds = 3
	}
	if value.UsageRequestTimeoutSeconds > 300 {
		value.UsageRequestTimeoutSeconds = 300
	}
	if value.Language != "en-US" {
		value.Language = defaults.Language
	}
	if value.ThemeMode != "light" && value.ThemeMode != "dark" {
		value.ThemeMode = defaults.ThemeMode
	}
	value.ProviderPricing = sanitizePricingMap(value.ProviderPricing)
	value.AccountPricing = sanitizePricingMap(value.AccountPricing)
	return value
}

func sanitizePricingMap(input map[string]PricingRule) map[string]PricingRule {
	if len(input) == 0 {
		return map[string]PricingRule{}
	}
	output := make(map[string]PricingRule, len(input))
	for key, rule := range input {
		if key == "" {
			continue
		}
		if rule.InputPerMillion < 0 || rule.OutputPerMillion < 0 {
			continue
		}
		output[key] = rule
	}
	if len(output) == 0 {
		return map[string]PricingRule{}
	}
	return output
}

func encodePricingMap(input map[string]PricingRule) (string, error) {
	if len(input) == 0 {
		return "{}", nil
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func decodePricingMap(raw string) (map[string]PricingRule, error) {
	if raw == "" {
		return map[string]PricingRule{}, nil
	}
	decoded := make(map[string]PricingRule)
	if err := json.Unmarshal([]byte(raw), &decoded); err == nil {
		return sanitizePricingMap(decoded), nil
	}

	legacy := make(map[string]map[string]float64)
	if err := json.Unmarshal([]byte(raw), &legacy); err != nil {
		return nil, err
	}
	converted := make(map[string]PricingRule, len(legacy))
	for key, item := range legacy {
		converted[key] = PricingRule{
			InputPerMillion:  item["input_per_million"],
			OutputPerMillion: item["output_per_million"],
		}
	}
	return sanitizePricingMap(converted), nil
}

func AccountPricingKey(accountID int64) string {
	return strconv.FormatInt(accountID, 10)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func sanitizeOptionalString(value string) string {
	return strings.TrimSpace(value)
}

func sanitizeIPWhitelist(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}

	seen := make(map[string]struct{})
	items := make([]string, 0)
	for _, raw := range strings.FieldsFunc(value, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';'
	}) {
		entry := strings.TrimSpace(raw)
		if entry == "" || isLoopbackWhitelistEntry(entry) {
			continue
		}
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		items = append(items, entry)
	}
	return strings.Join(items, "\n")
}

func isLoopbackWhitelistEntry(value string) bool {
	if strings.EqualFold(value, "localhost") {
		return true
	}
	if ip, err := strconv.Unquote(value); err == nil {
		value = ip
	}
	parsed := strings.TrimSpace(value)
	ip := net.ParseIP(parsed)
	if ip != nil {
		return ip.IsLoopback()
	}
	if strings.Contains(parsed, "/") {
		if _, cidr, err := net.ParseCIDR(parsed); err == nil && cidr.IP.IsLoopback() {
			return true
		}
	}
	return false
}
