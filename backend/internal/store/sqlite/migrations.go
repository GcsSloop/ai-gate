package sqlite

var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS providers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`,
	`CREATE TABLE IF NOT EXISTS accounts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		provider_type TEXT NOT NULL,
		account_name TEXT NOT NULL,
		source_icon TEXT NOT NULL DEFAULT 'openai',
		auth_mode TEXT NOT NULL,
		credential_ref TEXT NOT NULL,
		base_url TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'active',
		priority INTEGER NOT NULL DEFAULT 0,
		is_active INTEGER NOT NULL DEFAULT 0,
		supports_responses INTEGER NOT NULL DEFAULT 0,
		cooldown_until DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`,
	`CREATE TABLE IF NOT EXISTS account_usage_snapshots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id INTEGER NOT NULL,
		balance REAL,
		quota_remaining REAL,
		rpm_remaining REAL,
		tpm_remaining REAL,
		health_score REAL,
		recent_error_rate REAL,
		avg_latency_ms REAL,
		throttled_recently INTEGER NOT NULL DEFAULT 0,
		last_total_tokens REAL,
		last_input_tokens REAL,
		last_output_tokens REAL,
		model_context_window REAL,
		primary_used_percent REAL,
		secondary_used_percent REAL,
		primary_resets_at DATETIME,
		secondary_resets_at DATETIME,
		checked_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`,
	`CREATE TABLE IF NOT EXISTS routing_policies (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		payload TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`,
	`CREATE TABLE IF NOT EXISTS app_settings (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		launch_at_login INTEGER NOT NULL DEFAULT 0,
		silent_start INTEGER NOT NULL DEFAULT 0,
		close_to_tray INTEGER NOT NULL DEFAULT 1,
		show_proxy_switch_on_home INTEGER NOT NULL DEFAULT 1,
		proxy_host TEXT NOT NULL DEFAULT '127.0.0.1',
		proxy_port INTEGER NOT NULL DEFAULT 6789,
		auto_failover_enabled INTEGER NOT NULL DEFAULT 0,
		auto_backup_interval_hours INTEGER NOT NULL DEFAULT 24,
		backup_retention_count INTEGER NOT NULL DEFAULT 10,
		language TEXT NOT NULL DEFAULT 'zh-CN',
		theme_mode TEXT NOT NULL DEFAULT 'system',
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`,
	`CREATE TABLE IF NOT EXISTS failover_queue_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id INTEGER NOT NULL,
		position INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`,
	`CREATE TABLE IF NOT EXISTS conversations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		client_id TEXT,
		target_provider_family TEXT NOT NULL DEFAULT '',
		default_model TEXT NOT NULL DEFAULT '',
		current_account_id INTEGER,
		state TEXT NOT NULL DEFAULT 'active',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`,
	`CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		conversation_id INTEGER NOT NULL,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		item_type TEXT NOT NULL DEFAULT 'message',
		raw_item_json TEXT NOT NULL DEFAULT '',
		sequence_no INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`,
	`CREATE TABLE IF NOT EXISTS runs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		conversation_id INTEGER NOT NULL,
		account_id INTEGER,
		model TEXT NOT NULL DEFAULT '',
		fallback_from_run_id INTEGER,
		stream_offset INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL,
		started_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`,
}
