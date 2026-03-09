import { apiPath } from "./paths";

export type AccountRecord = {
  id: number;
  provider_type: string;
  account_name: string;
  auth_mode: string;
  base_url: string;
  status: string;
  priority: number;
  is_active: boolean;
  allow_chat_fallback?: boolean;
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
  auth_mode: string;
  base_url: string;
  credential_ref: string;
  allow_chat_fallback?: boolean;
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
  payload: Partial<CreateAccountPayload> & { account_name?: string; status?: string; priority?: number; is_active?: boolean; allow_chat_fallback?: boolean },
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
