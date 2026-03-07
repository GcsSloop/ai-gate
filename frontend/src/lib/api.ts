import { apiPath } from "./paths";

export type AccountRecord = {
  id: number;
  provider_type: string;
  account_name: string;
  auth_mode: string;
  base_url: string;
  status: string;
  priority: number;
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

export type CreateAccountPayload = {
  provider_type: string;
  account_name: string;
  auth_mode: string;
  base_url: string;
  credential_ref: string;
};

export type DashboardSummary = {
  total_conversations: number;
  active_conversations: number;
  total_runs: number;
  failover_runs: number;
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

export async function listAccounts(): Promise<AccountRecord[]> {
  const response = await fetch(apiPath("/accounts"));
  if (!response.ok) {
    throw new Error("failed to load accounts");
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
  payload: Partial<CreateAccountPayload> & { account_name?: string; status?: string; priority?: number },
): Promise<void> {
  const response = await fetch(apiPath(`/accounts/${id}`), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    throw new Error("failed to update account");
  }
}

export async function getDashboardSummary(): Promise<DashboardSummary> {
  const response = await fetch(apiPath("/dashboard/summary"));
  if (!response.ok) {
    throw new Error("failed to load dashboard summary");
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
