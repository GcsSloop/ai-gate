import { FormEvent, useEffect, useState } from "react";

import { createAccount, listAccounts, startOfficialAuth, type AccountRecord } from "../../lib/api";

const defaultBaseURL = "https://code.ppchat.vip/v1";

export function AccountsPage() {
  const [accounts, setAccounts] = useState<AccountRecord[]>([]);
  const [accountName, setAccountName] = useState("");
  const [baseURL, setBaseURL] = useState(defaultBaseURL);
  const [apiKey, setAPIKey] = useState("");

  useEffect(() => {
    void listAccounts().then(setAccounts);
  }, []);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    await createAccount({
      provider_type: "openai-compatible",
      account_name: accountName,
      auth_mode: "api_key",
      base_url: baseURL,
      credential_ref: apiKey,
    });
  }

  async function handleOfficialAuth() {
    await startOfficialAuth();
  }

  return (
    <div className="page-grid">
      <section className="panel">
        <h2>Accounts</h2>
        <p>Manage official authorization and OpenAI-compatible third-party endpoints.</p>
        <button type="button" onClick={handleOfficialAuth}>
          Connect official account
        </button>
        <ul className="account-list">
          {accounts.map((account) => (
            <li key={account.id} className="account-card">
              <strong>{account.account_name}</strong>
              <span>{account.status}</span>
              <input readOnly value={account.base_url} aria-label={`Base URL for ${account.account_name}`} />
            </li>
          ))}
        </ul>
      </section>

      <section className="panel">
        <h2>Add third-party endpoint</h2>
        <form className="stack" onSubmit={(event) => void handleSubmit(event)}>
          <label>
            Account name
            <input value={accountName} onChange={(event) => setAccountName(event.target.value)} />
          </label>
          <label>
            Base URL
            <input value={baseURL} onChange={(event) => setBaseURL(event.target.value)} />
          </label>
          <label>
            API key
            <input value={apiKey} onChange={(event) => setAPIKey(event.target.value)} />
          </label>
          <button type="submit">Save third-party account</button>
        </form>
      </section>
    </div>
  );
}
