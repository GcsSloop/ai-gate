CREATE TABLE IF NOT EXISTS install_events (
  event_key TEXT PRIMARY KEY,
  event_day TEXT NOT NULL,
  user_hash TEXT NOT NULL,
  anonymous_id TEXT DEFAULT '',
  skill_name TEXT NOT NULL,
  source_repo TEXT DEFAULT '',
  client_version TEXT DEFAULT '',
  client_platform TEXT DEFAULT '',
  client_arch TEXT DEFAULT '',
  client_app TEXT DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_install_events_day
  ON install_events(event_day);

CREATE INDEX IF NOT EXISTS idx_install_events_day_skill
  ON install_events(event_day, skill_name);

CREATE INDEX IF NOT EXISTS idx_install_events_user_created
  ON install_events(user_hash, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_install_events_created
  ON install_events(created_at DESC);

CREATE TABLE IF NOT EXISTS daily_skill_rank (
  event_day TEXT NOT NULL,
  skill_name TEXT NOT NULL,
  source_repo TEXT DEFAULT '',
  installs INTEGER NOT NULL DEFAULT 0,
  unique_users INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (event_day, skill_name, source_repo)
);

CREATE INDEX IF NOT EXISTS idx_daily_skill_rank_day_installs
  ON daily_skill_rank(event_day, installs DESC, unique_users DESC);

CREATE TABLE IF NOT EXISTS tracked_repos (
  repo_key TEXT PRIMARY KEY,
  platform TEXT NOT NULL,
  owner TEXT NOT NULL,
  name TEXT NOT NULL,
  branch TEXT NOT NULL DEFAULT 'main',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_tracked_repos_order
  ON tracked_repos(sort_order, owner, name);

CREATE TABLE IF NOT EXISTS skills_audit_cache (
  skill_key TEXT NOT NULL,
  source_repo TEXT NOT NULL,
  source_skill_id TEXT NOT NULL,
  provider TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'info',
  label TEXT NOT NULL DEFAULT '',
  source_url TEXT NOT NULL DEFAULT '',
  fetched_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  PRIMARY KEY (skill_key, provider)
);

CREATE INDEX IF NOT EXISTS idx_skills_audit_cache_expires
  ON skills_audit_cache(expires_at);

CREATE TABLE IF NOT EXISTS skills_external_map (
  skill_key TEXT PRIMARY KEY,
  source_repo TEXT NOT NULL,
  source_path TEXT NOT NULL,
  source_name_normalized TEXT NOT NULL,
  skills_sh_slug TEXT NOT NULL,
  skills_sh_url TEXT NOT NULL,
  match_confidence REAL NOT NULL DEFAULT 0,
  matched_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_skills_external_map_repo_name
  ON skills_external_map(source_repo, source_name_normalized);
