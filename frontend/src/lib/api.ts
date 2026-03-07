export type AccountRecord = {
  id: number;
  provider_type: string;
  account_name: string;
  auth_mode: string;
  base_url: string;
  status: string;
  cooldown_remaining_seconds?: number;
};

export type CreateAccountPayload = {
  provider_type: string;
  account_name: string;
  auth_mode: string;
  base_url: string;
  credential_ref: string;
};

export async function listAccounts(): Promise<AccountRecord[]> {
  const response = await fetch("/accounts");
  if (!response.ok) {
    throw new Error("failed to load accounts");
  }
  return response.json();
}

export async function createAccount(payload: CreateAccountPayload): Promise<void> {
  const response = await fetch("/accounts", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    throw new Error("failed to create account");
  }
}

export async function startOfficialAuth(): Promise<void> {
  const response = await fetch("/accounts/auth/authorize", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: "{}",
  });
  if (!response.ok) {
    throw new Error("failed to start official auth");
  }
}

export async function importLocalCodexAuth(path?: string): Promise<void> {
  const response = await fetch("/accounts/import-local", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      path,
      account_name: "local-codex",
    }),
  });
  if (!response.ok) {
    throw new Error("failed to import local codex auth");
  }
}
