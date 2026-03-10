import { apiPath } from "./paths";

export type AccountRecord = {
  id: number;
  provider_type: string;
  account_name: string;
  source_icon?: "openai" | "claude_code" | "ppchat";
  auth_mode: string;
  base_url: string;
  status: string;
  priority: number;
  is_active: boolean;
  supports_responses?: boolean;
  cooldown_remaining_seconds?: number;
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
};

export type CreateAccountPayload = {
  provider_type: string;
  account_name: string;
  source_icon?: "openai" | "claude_code" | "ppchat";
  auth_mode: string;
  base_url: string;
  credential_ref: string;
  supports_responses?: boolean;
};

export type DashboardSummary = {
  total_conversations: number;
  active_conversations: number;
  total_runs: number;
  failover_runs: number;
};

export type AccountCallStats = {
  account_id: number;
  total_calls: number;
  models: Record<string, number>;
};

export type AccountTestResult = {
  ok: boolean;
  message: string;
  details?: string;
  content?: string;
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
  proxy_host: string;
  proxy_port: number;
  auto_failover_enabled: boolean;
  auto_backup_interval_hours: number;
  backup_retention_count: number;
};

export type DatabaseBackupItem = {
  backup_id: string;
  created_at: string;
  size_bytes: number;
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

export async function createAccount(payload: CreateAccountPayload): Promise<void> {
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
  payload: Partial<CreateAccountPayload> & { account_name?: string; status?: string; priority?: number; is_active?: boolean; supports_responses?: boolean },
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

export async function getDashboardSummary(): Promise<DashboardSummary> {
  const response = await fetch(apiPath("/dashboard/summary"));
  if (!response.ok) {
    throw new Error("failed to load dashboard summary");
  }
  return response.json();
}

export async function getAccountCallStats(): Promise<AccountCallStats[]> {
  const response = await fetch(apiPath("/dashboard/account-stats"));
  if (!response.ok) {
    throw new Error("failed to load account call stats");
  }
  return response.json();
}

export async function testAccount(id: number, payload: AccountChatTestPayload): Promise<AccountTestResult> {
  const response = await fetch(apiPath(`/accounts/${id}/test`), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  const data = (await response.json()) as AccountTestResult;
  if (!response.ok) {
    return {
      ok: false,
      message: data.message || "测试失败",
      details: data.details || "请求账户测试接口失败",
      content: data.content,
    };
  }
  return data;
}

export async function fetchPPChatTokenLogs(accountID: number, page = 1, pageSize = 10): Promise<PPChatTokenLogsPayload> {
  const response = await fetch(apiPath(`/accounts/${accountID}/ppchat-token-logs?page=${page}&page_size=${pageSize}`));
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

export async function importLocalCodexAuth(file: File, accountName = "local-codex"): Promise<void> {
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

export async function importCurrentCodexAuth(accountName = "local-codex"): Promise<void> {
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

export async function getCodexBackupFiles(backupID: string): Promise<CodexBackupFiles> {
  const response = await fetch(apiPath(`/settings/codex/backups/${backupID}/files`));
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

export async function saveAppSettings(payload: AppSettings): Promise<AppSettings> {
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
  const response = await fetch(apiPath("/settings/database/json-export"));
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to export database json");
  }
  return response.text();
}

export async function importDatabaseSQL(raw: string): Promise<void> {
  const response = await fetch(apiPath("/settings/database/json-import"), {
    method: "POST",
    headers: { "Content-Type": "application/json; charset=utf-8" },
    body: raw,
  });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to import database json");
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

export async function enableProxy(): Promise<ProxyStatus> {
  const response = await fetch(apiPath("/settings/proxy/enable"), { method: "POST" });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to enable proxy");
  }
  return response.json();
}

export async function disableProxy(options?: { force?: boolean; skipRestore?: boolean }): Promise<ProxyStatus> {
  const params = new URLSearchParams();
  if (options?.force) {
    params.set("force", "1");
  }
  if (options?.skipRestore) {
    params.set("skip_restore", "1");
  }
  const suffix = params.toString() ? `?${params.toString()}` : "";
  const response = await fetch(apiPath(`/settings/proxy/disable${suffix}`), { method: "POST" });
  if (!response.ok) {
    const details = await response.text();
    throw new Error(details || "failed to disable proxy");
  }
  return response.json();
}
