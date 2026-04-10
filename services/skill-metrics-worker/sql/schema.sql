CREATE TABLE IF NOT EXISTS install_events (
  event_key TEXT PRIMARY KEY,
  event_day TEXT NOT NULL,
  user_hash TEXT NOT NULL,
  skill_name TEXT NOT NULL,
  source_repo TEXT DEFAULT '',
  client_version TEXT DEFAULT '',
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
