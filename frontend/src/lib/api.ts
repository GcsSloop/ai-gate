import { runtimeTranslate } from "./i18n";
import { apiPath } from "./paths";

export type AccountRecord = {
  id: number;
  provider_type: string;
  account_name: string;
  source_icon?: "openai" | "claude_code" | "ppchat";
  auth_mode: string;
  base_url: string;
  account_driver: string;
  usage_driver: string;
  usage_config_json: string;
  status: string;
  priority: number;
  is_active: boolean;
  supports_responses?: boolean;
  cooldown_remaining_seconds?: number;
  routing_cooldown_remaining_seconds?: number;
  routing_cooldown_reason?: string;
  balance: number;
  quota_remaining: number;
  rpm_remaining: number;
  tpm_remaining: number;
  health_score: number;
  recent_error_rate: number;
  last_total_tokens: number;
  last_input_tokens: number;
  last_output_tokens: number;
  model_context_window: number;
  primary_used_percent: number;
  secondary_used_percent: number;
  primary_resets_at?: string;
  secondary_resets_at?: string;
  checked_at?: string;
  stale?: boolean;
  last_error?: string;
  ppchat_today_used_quota?: number;
  ppchat_today_added_quota?: number;
  ppchat_today_remaining_quota?: number;
};

export type AccountUsageRecord = {
  account_id: number;
  balance: number;
  quota_remaining: number;
  rpm_remaining: number;
  tpm_remaining: number;
  health_score: number;
  recent_error_rate: number;
  last_total_tokens: number;
  last_input_tokens: number;
  last_output_tokens: number;
  model_context_window: number;
  primary_used_percent: number;
  secondary_used_percent: number;
  primary_resets_at?: string;
  secondary_resets_at?: string;
  checked_at?: string;
  stale?: boolean;
  last_error?: string;
  ppchat_today_used_quota?: number;
  ppchat_today_added_quota?: number;
  ppchat_today_remaining_quota?: number;
};

export type CreateAccountPayload = {
  provider_type: string;
  account_name: string;
  source_icon?: "openai" | "claude_code" | "ppchat";
  auth_mode: string;
  base_url: string;
  credential_ref: string;
  account_driver?: string;
  usage_driver?: string;
  usage_config_json?: string;
  supports_responses?: boolean;
};

export type ShareAccountResponse = {
  payload: string;
};

export type UsageDashboardSummary = {
  request_count: number;
  success_count: number;
  failure_count: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  estimated_cost: number;
  balance_delta: number;
  quota_delta: number;
};

export type UsageTrendPoint = {
  bucket: string;
  request_count: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  estimated_cost: number;
  balance_delta: number;
  quota_delta: number;
};

export type UsageModelDistributionPoint = {
  model: string;
  request_count: number;
};

export type UsageEventRecord = {
  id: number;
  account_id: number;
  provider_type: string;
  request_kind: string;
  model: string;
  status: string;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  estimated_cost: number;
  balance_before?: number;
  balance_after?: number;
  quota_before?: number;
  quota_after?: number;
  latency_ms: number;
  created_at: string;
};

export type PricingRule = {
  input_per_million: number;
  output_per_million: number;
};

export type AccountTestResult = {
  ok: boolean;
  message: string;
  details?: string;
  content?: string;
};

export type LuaUsageScriptRecord = {
  key: string;
  content: string;
};

export type LuaUsageScriptList = {
  items: string[];
};

export type LuaUsageTestPayload = {
  usage_config_json: string;
  script_content: string;
};

export type PPChatTokenLogsPayload = {
  data: {
    logs: Array<{
      cache_creation_input_tokens: number;
      cache_read_input_tokens: number;
      completion_tokens: number;
      created_at: number;
      created_time: string;
      model_name: string;
      prompt_tokens: number;
      quota: number;
    }>;
    pagination: {
      page: number;
      page_size: number;
      total: number;
      total_pages: number;
    };
    token_info: {
      name: string;
      today_usage_count: number;
      today_used_quota: number;
      remain_quota_display: number;
      today_added_quota?: number;
      today_opus_usage?: number;
      today_big_token_requests?: number;
      expired_time_formatted: string;
      expiry?: {
        raw_timestamp: number;
        status: string;
        time: string;
      };
      status?: {
        code: number;
        text: string;
        type: string;
      };
    };
  };
  success: boolean;
};

export type AccountChatTestPayload = {
  model: string;
  input: string;
};

export type CodexBackupItem = {
  backup_id: string;
  created_at: string;
};

export type CodexBackupFiles = {
  backup_id: string;
  files: Record<string, string>;
};

export type ProxyStatus = {
  enabled: boolean;
  last_backup_id?: string;
  host?: string;
  port?: number;
};

export type AppSettings = {
  launch_at_login: boolean;
  silent_start: boolean;
  close_to_tray: boolean;
  show_proxy_switch_on_home: boolean;
  show_home_update_indicator: boolean;
  status_refresh_interval_seconds: number;
  usage_request_timeout_seconds?: number;
  proxy_host: string;
  proxy_port: number;
  lan_share_enabled: boolean;
  lan_share_ip_whitelist: string;
  upstream_proxy_mode?: "system" | "direct" | "manual";
  upstream_proxy_url?: string;
  upstream_proxy_username?: string;
  upstream_proxy_password?: string;
  auto_failover_enabled: boolean;
  auto_backup_interval_hours: number;
  backup_retention_count: number;
  audit_limit_message: number;
  audit_limit_function_call: number;
  audit_limit_function_call_output: number;
  audit_limit_reasoning: number;
  audit_limit_custom_tool_call: number;
  audit_limit_custom_tool_call_output: number;
  language: "zh-CN" | "en-US";
  theme_mode: "system" | "light" | "dark";
  provider_pricing?: Record<string, PricingRule>;
  account_pricing?: Record<string, PricingRule>;
};

export type DatabaseBackupItem = {
  backup_id: string;
  created_at: string;
  size_bytes: number;
};

export type ToolingClientStatus = {
  app: "codex";
  label: string;
  skills_dir: string;
  mcp_path: string;
  skills_count: number;
  mcp_status: "missing" | "ready";
};

export type ToolingSkillStats = {
  total: number;
  by_source: Record<string, number>;
};

export type ToolingSkillRepo = {
  platform?: "github" | "gitlab";
  owner: string;
  name: string;
  branch: string;
  enabled: boolean;
  skill_count: number;
  star_count?: number;
};

export type ToolingRepoOrderItem = {
  platform?: "github" | "gitlab";
  owner: string;
  name: string;
};

export type ToolingRepoSearchResult = {
  platform?: "github" | "gitlab";
  owner: string;
  name: string;
  branch: string;
  url: string;
  description?: string;
};

export type ToolingResolvedRepo = {
  platform: "github" | "gitlab";
  owner: string;
  name: string;
  repo_url: string;
  branch_options: string[];
  selected_branch: string;
};

export type ToolingDiscoveredSkill = {
  id: string;
  name: string;
  description?: string;
  platform: "github" | "gitlab";
  repo_owner: string;
  repo_name: string;
  branch: string;
  repo_url: string;
  source_path: string;
  source_url: string;
  managed_name: string;
  skills_sh_url?: string;
  audits_summary?: {
    match_confidence: number;
    providers: Array<{
      provider: "agent_trust_hub" | "socket" | "snyk";
      label: string;
      status: "pass" | "warn" | "fail" | "info";
      url: string;
    }>;
  };
  content_hash?: string;
  installed_hash?: string;
  installed_apps: Record<string, boolean>;
  update_available?: boolean;
};

export type ToolingSkillDiscoveryResponse = {
  cached: boolean;
  fetched_at?: string;
  indexed_total?: number;
  total: number;
  offset: number;
  limit: number;
  query?: string;
  items: ToolingDiscoveredSkill[];
};

export type ToolingSkillRecord = {
  name: string;
  description?: string;
  directory: string;
  source_client?: string;
  source_repo?: string;
  source_kind: string;
  managed_path: string;
  installed_apps: Record<string, boolean>;
  update_available?: boolean;
};

export type ToolingMcpTemplate = {
  id: string;
  name: string;
  description: string;
  type: string;
  command?: string;
  args?: string[];
  url?: string;
};

export type ToolingMcpServer = {
  id: string;
  name: string;
  description?: string;
  template_id?: string;
  enabled_apps: Record<string, boolean>;
  client_status: Record<string, "missing" | "disabled" | "enabled">;
  delete_allowed?: boolean;
  delete_reason?: string;
  delete_targets?: string[];
  spec: Record<string, unknown>;
};

export type ToolingDiscoveredMcpServer = {
  id: string;
  name: string;
  description?: string;
  source_apps: Record<string, boolean>;
  client_status: Record<string, "missing" | "enabled">;
  spec: Record<string, unknown>;
};

export type ToolingState = {
  skill_sync_method: "symlink" | "copy";
  clients: ToolingClientStatus[];
  skill_stats: ToolingSkillStats;
  skill_repos: ToolingSkillRepo[];
  installed_skills: ToolingSkillRecord[];
  repo_search_results: ToolingRepoSearchResult[];
  discovered_mcp_servers: ToolingDiscoveredMcpServer[];
  mcp_templates: ToolingMcpTemplate[];
  mcp_servers: ToolingMcpServer[];
};

export async function listAccounts(): Promise<AccountRecord[]> {
  const response = await fetch(apiPath("/accounts"));
  if (!response.ok) {
    throw new Error("failed to load accounts");
  }
  return response.json();
}

export async function listAccountUsage(): Promise<AccountUsageRecord[]> {
  const response = await fetch(apiPath("/accounts/usage"));
  if (!response.ok) {
    throw new Error("failed to load account usage");
  }
  return response.json();
}

export async function refreshAccountUsage(): Promise<void> {
  const response = await fetch(apiPath("/accounts/usage/refresh"), {
    method: "POST",
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to refresh account usage");
  }
}

export function subscribeAccountRoutingStateChanged(
  handler: () => void,
): () => void {
  if (typeof window === "undefined" || typeof EventSource === "undefined") {
    return () => {};
  }
  const eventSource = new EventSource(apiPath("/dashboard/state-events"));
  eventSource.onmessage = () => {
    handler();
  };
  eventSource.onerror = () => {
    eventSource.close();
  };
  return () => {
    eventSource.close();
  };
}

export async function createAccount(
  payload: CreateAccountPayload,
): Promise<void> {
  const response = await fetch(apiPath("/accounts"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    throw new Error("failed to create account");
  }
}

export async function updateAccount(
  id: number,
  payload: Partial<CreateAccountPayload> & {
    account_name?: string;
    status?: string;
    priority?: number;
    is_active?: boolean;
    supports_responses?: boolean;
  },
): Promise<void> {
  const response = await fetch(apiPath(`/accounts/${id}`), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to update account");
  }
}

export async function duplicateAccount(id: number): Promise<void> {
  const response = await fetch(apiPath(`/accounts/${id}/duplicate`), {
    method: "POST",
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to duplicate account");
  }
}

export async function shareAccount(id: number): Promise<ShareAccountResponse> {
  const response = await fetch(apiPath(`/accounts/${id}/share`), {
    method: "POST",
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to share account");
  }
  return response.json();
}

export async function importSharedAccount(payload: string): Promise<void> {
  const response = await fetch(apiPath("/accounts/import-shared"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ payload }),
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to import shared account");
  }
}

export type DashboardRangeKey = "24h" | "7d" | "30d";

function dashboardQuery(
  range: DashboardRangeKey = "24h",
  accountID?: number,
  model?: string,
  limit?: number,
): string {
  const params = new URLSearchParams();
  params.set("range", range);
  if (accountID && accountID > 0) {
    params.set("account_id", String(accountID));
  }
  if (model && model.trim() !== "") {
    params.set("model", model.trim());
  }
  if (limit && limit > 0) {
    params.set("limit", String(limit));
  }
  return params.toString();
}

export async function getDashboardSummary(
  range: DashboardRangeKey = "24h",
  accountID?: number,
  model?: string,
): Promise<UsageDashboardSummary> {
  const response = await fetch(
    apiPath(`/dashboard/summary?${dashboardQuery(range, accountID, model)}`),
  );
  if (!response.ok) {
    throw new Error("failed to load dashboard summary");
  }
  return response.json();
}

export async function getDashboardTrends(
  range: DashboardRangeKey = "24h",
  accountID?: number,
  model?: string,
): Promise<UsageTrendPoint[]> {
  const response = await fetch(
    apiPath(`/dashboard/trends?${dashboardQuery(range, accountID, model)}`),
  );
  if (!response.ok) {
    throw new Error("failed to load dashboard trends");
  }
  return response.json();
}

export async function getDashboardRecentEvents(
  range: DashboardRangeKey = "24h",
  accountID?: number,
  model?: string,
  limit = 20,
): Promise<UsageEventRecord[]> {
  const response = await fetch(
    apiPath(
      `/dashboard/recent-events?${dashboardQuery(range, accountID, model, limit)}`,
    ),
  );
  if (!response.ok) {
    throw new Error("failed to load recent usage events");
  }
  return response.json();
}

export async function getDashboardModelDistribution(
  range: DashboardRangeKey = "24h",
  accountID?: number,
  model?: string,
): Promise<UsageModelDistributionPoint[]> {
  const response = await fetch(
    apiPath(
      `/dashboard/model-distribution?${dashboardQuery(range, accountID, model)}`,
    ),
  );
  if (!response.ok) {
    throw new Error("failed to load model distribution");
  }
  return response.json();
}

export async function testAccount(
  id: number,
  payload: AccountChatTestPayload,
): Promise<AccountTestResult> {
  const response = await fetch(apiPath(`/accounts/${id}/test`), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  const data = (await response.json()) as AccountTestResult;
  if (!response.ok) {
    return {
      ok: false,
      message: data.message || runtimeTranslate("测试失败"),
      details: data.details || runtimeTranslate("请求账户测试接口失败"),
      content: data.content,
    };
  }
  return data;
}

export async function getLuaUsageScript(
  key: string,
): Promise<LuaUsageScriptRecord> {
  const response = await fetch(
    apiPath(`/accounts/usage-scripts/${encodeURIComponent(key)}`),
  );
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to load lua usage script");
  }
  return response.json();
}

export async function listLuaUsageScripts(): Promise<LuaUsageScriptList> {
  const response = await fetch(apiPath("/accounts/usage-scripts"));
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to list lua usage scripts");
  }
  return response.json();
}

export async function saveLuaUsageScript(
  key: string,
  content: string,
): Promise<void> {
  const response = await fetch(
    apiPath(`/accounts/usage-scripts/${encodeURIComponent(key)}`),
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ content }),
    },
  );
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to save lua usage script");
  }
}

export async function testLuaUsage(
  id: number,
  payload: LuaUsageTestPayload,
): Promise<AccountTestResult> {
  const response = await fetch(apiPath(`/accounts/${id}/usage-lua-test`), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  const data = (await response.json()) as AccountTestResult;
  if (!response.ok) {
    return {
      ok: false,
      message: data.message || runtimeTranslate("测试失败"),
      details: data.details || runtimeTranslate("请求 Lua 测试接口失败"),
      content: data.content,
    };
  }
  return data;
}

export async function fetchPPChatTokenLogs(
  accountID: number,
  page = 1,
  pageSize = 10,
): Promise<PPChatTokenLogsPayload> {
  const response = await fetch(
    apiPath(
      `/accounts/${accountID}/ppchat-token-logs?page=${page}&page_size=${pageSize}`,
    ),
  );
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to fetch ppchat token logs");
  }
  return response.json();
}

export async function deleteAccount(id: number): Promise<void> {
  const response = await fetch(apiPath(`/accounts/${id}`), {
    method: "DELETE",
  });
  if (!response.ok) {
    throw new Error("failed to delete account");
  }
}

export async function startOfficialAuth(): Promise<void> {
  const response = await fetch(apiPath("/accounts/auth/authorize"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: "{}",
  });
  if (!response.ok) {
    throw new Error("failed to start official auth");
  }
}

export async function importLocalCodexAuth(
  file: File,
  accountName = "local-codex",
): Promise<void> {
  const formData = new FormData();
  formData.append("account_name", accountName);
  formData.append("auth_file", file);
  const response = await fetch(apiPath("/accounts/import-local"), {
    method: "POST",
    body: formData,
  });
  if (!response.ok) {
    throw new Error("failed to import local codex auth");
  }
}

export async function importCurrentCodexAuth(
  accountName = "local-codex",
): Promise<void> {
  const response = await fetch(apiPath("/accounts/import-current"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ account_name: accountName }),
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to import current codex auth");
  }
}

export async function listCodexBackups(): Promise<CodexBackupItem[]> {
  const response = await fetch(apiPath("/settings/codex/backups"));
  if (!response.ok) {
    throw new Error("failed to list codex backups");
  }
  return response.json();
}

export async function createCodexBackup(): Promise<void> {
  const response = await fetch(apiPath("/settings/codex/backup"), {
    method: "POST",
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to create codex backup");
  }
}

export async function restoreCodexBackup(backupID: string): Promise<void> {
  const response = await fetch(apiPath("/settings/codex/restore"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ backup_id: backupID }),
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to restore codex backup");
  }
}

export async function getCodexBackupFiles(
  backupID: string,
): Promise<CodexBackupFiles> {
  const response = await fetch(
    apiPath(`/settings/codex/backups/${backupID}/files`),
  );
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to fetch backup files");
  }
  return response.json();
}

export async function getProxyStatus(): Promise<ProxyStatus> {
  const response = await fetch(apiPath("/settings/proxy/status"));
  if (!response.ok) {
    throw new Error("failed to fetch proxy status");
  }
  return response.json();
}

export async function getAppSettings(): Promise<AppSettings> {
  const response = await fetch(apiPath("/settings/app"));
  if (!response.ok) {
    throw new Error("failed to fetch app settings");
  }
  return response.json();
}

export async function saveAppSettings(
  payload: AppSettings,
): Promise<AppSettings> {
  const response = await fetch(apiPath("/settings/app"), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to save app settings");
  }
  return response.json();
}

export async function getFailoverQueue(): Promise<number[]> {
  const response = await fetch(apiPath("/settings/failover-queue"));
  if (!response.ok) {
    throw new Error("failed to fetch failover queue");
  }
  return response.json();
}

export async function saveFailoverQueue(accountIDs: number[]): Promise<void> {
  const response = await fetch(apiPath("/settings/failover-queue"), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ account_ids: accountIDs }),
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to save failover queue");
  }
}

export async function exportDatabaseSQL(): Promise<string> {
  const jsonResponse = await fetch(apiPath("/settings/database/json-export"));
  if (jsonResponse.ok) {
    return jsonResponse.text();
  }

  if (jsonResponse.status !== 404) {
    const details = await jsonResponse.text();
    throw new Error(details || "failed to export database json");
  }

  // Backward compatibility for legacy backends that only expose SQL export routes.
  const sqlResponse = await fetch(apiPath("/settings/database/sql-export"));
  if (!sqlResponse.ok) {
    const details = await sqlResponse.text();
    throw new Error(details || "failed to export database sql");
  }
  return sqlResponse.text();
}

export async function importDatabaseSQL(raw: string): Promise<void> {
  const jsonResponse = await fetch(apiPath("/settings/database/json-import"), {
    method: "POST",
    headers: { "Content-Type": "application/json; charset=utf-8" },
    body: raw,
  });
  if (jsonResponse.ok) {
    return;
  }
  if (jsonResponse.status !== 404) {
    const details = await jsonResponse.text();
    throw new Error(details || "failed to import database json");
  }

  const trimmed = raw.trimStart();
  const isAIGateJSONExchange =
    trimmed.startsWith("{") && raw.includes('"format":"aigate-db-exchange"');
  if (isAIGateJSONExchange) {
    throw new Error(
      runtimeTranslate("当前后端版本不支持 JSON 导入，请升级到最新版本后重试"),
    );
  }

  // Backward compatibility for legacy SQL import route.
  const sqlResponse = await fetch(apiPath("/settings/database/sql-import"), {
    method: "POST",
    headers: { "Content-Type": "text/plain; charset=utf-8" },
    body: raw,
  });
  if (!sqlResponse.ok) {
    const details = await sqlResponse.text();
    throw new Error(details || "failed to import database sql");
  }
}

export async function listDatabaseBackups(): Promise<DatabaseBackupItem[]> {
  const response = await fetch(apiPath("/settings/database/backups"));
  if (!response.ok) {
    throw new Error("failed to list database backups");
  }
  return response.json();
}

export async function createDatabaseBackup(): Promise<DatabaseBackupItem> {
  const response = await fetch(apiPath("/settings/database/backup"), {
    method: "POST",
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to create database backup");
  }
  return response.json();
}

export async function restoreDatabaseBackup(backupID: string): Promise<void> {
  const response = await fetch(apiPath("/settings/database/restore"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ backup_id: backupID }),
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to restore database backup");
  }
}

export async function deleteDatabaseBackup(backupID: string): Promise<void> {
  const response = await fetch(
    apiPath(`/settings/database/backups/${encodeURIComponent(backupID)}`),
    {
      method: "DELETE",
    },
  );
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to delete database backup");
  }
}

export async function enableProxy(): Promise<ProxyStatus> {
  const response = await fetch(apiPath("/settings/proxy/enable"), {
    method: "POST",
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to enable proxy");
  }
  return response.json();
}

export async function disableProxy(options?: {
  force?: boolean;
  skipRestore?: boolean;
}): Promise<ProxyStatus> {
  const params = new URLSearchParams();
  if (options?.force) {
    params.set("force", "1");
  }
  if (options?.skipRestore) {
    params.set("skip_restore", "1");
  }
  const suffix = params.toString() ? `?${params.toString()}` : "";
  const response = await fetch(apiPath(`/settings/proxy/disable${suffix}`), {
    method: "POST",
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to disable proxy");
  }
  return response.json();
}

export async function getToolingState(): Promise<ToolingState> {
  const response = await fetch(apiPath("/tooling/state"));
  if (!response.ok) {
    throw new Error("failed to load tooling state");
  }
  return response.json();
}

export async function saveToolingSkillSyncMethod(skill_sync_method: "symlink" | "copy"): Promise<void> {
  const response = await fetch(apiPath("/tooling/settings"), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ skill_sync_method }),
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to save tooling settings");
  }
}

export async function importToolingSkills(source: string): Promise<{ imported: number }> {
  const response = await fetch(apiPath("/tooling/skills/import"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ source }),
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to import tooling skills");
  }
  return response.json();
}

export async function applyToolingSkills(apps: string[], method?: "symlink" | "copy"): Promise<{ applied: number; skill_sync_method: "symlink" | "copy" }> {
  const response = await fetch(apiPath("/tooling/skills/apply"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ apps, method }),
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to apply tooling skills");
  }
  return response.json();
}

export async function updateToolingSkill(
  name: string,
  payload: { apps?: string[]; method?: "symlink" | "copy"; enabled: boolean },
): Promise<{ applied: number; enabled: boolean; skill_sync_method?: "symlink" | "copy" }> {
  const response = await fetch(apiPath(`/tooling/skills/${encodeURIComponent(name)}`), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to update tooling skill");
  }
  return response.json();
}

export async function deleteToolingSkill(name: string): Promise<void> {
  const response = await fetch(apiPath(`/tooling/skills/${encodeURIComponent(name)}`), {
    method: "DELETE",
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to delete tooling skill");
  }
}

export async function listToolingRepos(): Promise<ToolingSkillRepo[]> {
  const response = await fetch(apiPath("/tooling/skills/repos"));
  if (!response.ok) {
    throw new Error("failed to list tooling repos");
  }
  return response.json();
}

export async function addToolingRepo(
  platformOrOwner: "github" | "gitlab" | string,
  ownerOrName: string,
  nameOrBranch = "main",
  maybeBranch?: string,
): Promise<ToolingSkillRepo> {
  const usesExplicitPlatform = platformOrOwner === "github" || platformOrOwner === "gitlab";
  const platform = usesExplicitPlatform ? platformOrOwner : "github";
  const owner = usesExplicitPlatform ? ownerOrName : platformOrOwner;
  const name = usesExplicitPlatform ? nameOrBranch : ownerOrName;
  const branch = usesExplicitPlatform ? (maybeBranch ?? "main") : (nameOrBranch || "main");
  const response = await fetch(apiPath("/tooling/skills/repos"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ platform, owner, name, branch }),
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to add tooling repo");
  }
  return response.json();
}

export async function updateToolingRepo(
  currentPlatform: "github" | "gitlab",
  currentOwner: string,
  currentName: string,
  payload: {
    platform: "github" | "gitlab";
    owner: string;
    name: string;
    branch: string;
  },
): Promise<ToolingSkillRepo> {
  const response = await fetch(apiPath(`/tooling/skills/repos/${encodeURIComponent(currentPlatform)}/${encodeURIComponent(currentOwner)}/${encodeURIComponent(currentName)}`), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to update tooling repo");
  }
  return response.json();
}

export async function removeToolingRepo(platform: "github" | "gitlab", owner: string, name: string): Promise<void> {
  const response = await fetch(apiPath(`/tooling/skills/repos/${encodeURIComponent(platform)}/${encodeURIComponent(owner)}/${encodeURIComponent(name)}`), {
    method: "DELETE",
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to remove tooling repo");
  }
}

export async function reorderToolingRepos(items: ToolingRepoOrderItem[]): Promise<ToolingSkillRepo[]> {
  const response = await fetch(apiPath("/tooling/skills/repos/order"), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ items }),
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to reorder tooling repos");
  }
  return response.json();
}

export async function searchToolingRepos(query: string): Promise<{ items: ToolingRepoSearchResult[] }> {
  const response = await fetch(apiPath(`/tooling/skills/repos/search?q=${encodeURIComponent(query)}`));
  if (!response.ok) {
    throw new Error("failed to search tooling repos");
  }
  return response.json();
}

export async function resolveToolingRepo(input: string): Promise<ToolingResolvedRepo> {
  const response = await fetch(apiPath("/tooling/skills/repos/resolve"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ input }),
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to resolve tooling repo");
  }
  return response.json();
}

export async function getToolingDiscoveredSkills(params?: { limit?: number; offset?: number; query?: string }): Promise<ToolingSkillDiscoveryResponse> {
  return requestCloudDiscoveredSkills(params);
}

export async function refreshToolingDiscoveredSkills(params?: { limit?: number; offset?: number; query?: string }): Promise<ToolingSkillDiscoveryResponse> {
  return requestCloudDiscoveredSkills(params);
}

function skillMetricsBaseURL(): string {
  const value = (import.meta.env.VITE_SKILL_METRICS_BASE_URL as string | undefined)?.trim();
  return value ? value.replace(/\/$/, "") : "https://aigate-skill-metrics.gcssloop.workers.dev";
}

function buildDiscoverySearch(params?: { limit?: number; offset?: number; query?: string }): URLSearchParams {
  const search = new URLSearchParams();
  if (typeof params?.limit === "number" && params.limit > 0) {
    search.set("limit", String(params.limit));
  }
  if (typeof params?.offset === "number" && params.offset >= 0) {
    search.set("offset", String(params.offset));
  }
  const query = params?.query?.trim();
  if (query) {
    search.set("q", query);
  }
  return search;
}

async function requestCloudDiscoveredSkills(params?: { limit?: number; offset?: number; query?: string }): Promise<ToolingSkillDiscoveryResponse> {
  const search = buildDiscoverySearch(params);
  const hasQuery = Boolean(params?.query?.trim());
  const endpoint = hasQuery ? "/skills/search" : "/skills/final";
  const response = await fetch(`${skillMetricsBaseURL()}${endpoint}?${search.toString()}`);
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to load discovered skills");
  }
  const payload = (await response.json()) as Record<string, unknown>;
  const items = Array.isArray(payload.items) ? (payload.items as ToolingDiscoveredSkill[]) : [];
  return {
    cached: typeof payload.cached === "boolean" ? payload.cached : true,
    fetched_at: typeof payload.fetched_at === "string" ? payload.fetched_at : "",
    indexed_total: typeof payload.indexed_total === "number"
      ? payload.indexed_total
      : typeof payload.total_items === "number"
      ? payload.total_items
      : items.length,
    total: typeof payload.total === "number"
      ? payload.total
      : typeof payload.total_items === "number"
      ? payload.total_items
      : items.length,
    offset: typeof payload.offset === "number" ? payload.offset : 0,
    limit: typeof payload.limit === "number" ? payload.limit : items.length,
    query: typeof payload.query === "string" ? payload.query : params?.query?.trim() ?? "",
    items,
  };
}

export async function installToolingDiscoveredSkill(payload: {
  id: string;
  platform?: "github" | "gitlab";
  repo_owner?: string;
  repo_name?: string;
  branch?: string;
  source_path?: string;
  apps?: string[];
}): Promise<{ applied: number; enabled: boolean; skill_sync_method?: "symlink" | "copy" }> {
  const response = await fetch(apiPath("/tooling/skills/discover/install"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to install discovered skill");
  }
  return response.json();
}

export async function listToolingMcpTemplates(): Promise<ToolingMcpTemplate[]> {
  const response = await fetch(apiPath("/tooling/mcp/templates"));
  if (!response.ok) {
    throw new Error("failed to list mcp templates");
  }
  return response.json();
}

export async function listToolingMcpServers(): Promise<ToolingMcpServer[]> {
  const response = await fetch(apiPath("/tooling/mcp/servers"));
  if (!response.ok) {
    throw new Error("failed to list mcp servers");
  }
  return response.json();
}

export async function importToolingMcpServers(source?: "codex"): Promise<{ imported: number }> {
  const response = await fetch(apiPath("/tooling/mcp/import"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(source ? { source } : {}),
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to import mcp servers");
  }
  return response.json();
}

export async function installToolingMcpServer(payload: {
  id: string;
  template_id: string;
  name?: string;
  description?: string;
  enabled_apps?: Record<string, boolean>;
}): Promise<ToolingMcpServer> {
  const response = await fetch(apiPath("/tooling/mcp/install"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to install mcp server");
  }
  return response.json();
}

export async function updateToolingMcpServer(id: string, payload: Partial<ToolingMcpServer>): Promise<ToolingMcpServer> {
  const response = await fetch(apiPath(`/tooling/mcp/servers/${encodeURIComponent(id)}`), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to update mcp server");
  }
  return response.json();
}

export async function deleteToolingMcpServer(id: string, cleanupLocalFiles = false): Promise<void> {
  const search = cleanupLocalFiles ? "?cleanup_local_files=1" : "";
  const response = await fetch(apiPath(`/tooling/mcp/servers/${encodeURIComponent(id)}${search}`), {
    method: "DELETE",
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to delete mcp server");
  }
}

export async function applyToolingMcpServer(id: string, apps?: string[], enabled?: boolean): Promise<void> {
  const response = await fetch(apiPath("/tooling/mcp/apply"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ id, apps, enabled }),
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to apply mcp server");
  }
}
