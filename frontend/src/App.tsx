import { App as AntApp, ConfigProvider, Switch } from "antd";
import { useEffect, useState } from "react";

import { AccountsPage } from "./features/accounts/AccountsPage";
import { SettingsPage } from "./features/settings/SettingsPage";
import { disableProxy, enableProxy, getProxyStatus } from "./lib/api";
import "./styles.css";

export function App() {
  const [tab, setTab] = useState<"accounts" | "settings">("accounts");
  const [proxyEnabled, setProxyEnabled] = useState(false);
  const [lastBackupID, setLastBackupID] = useState("");
  const [proxyLoading, setProxyLoading] = useState(false);

  useEffect(() => {
    void (async () => {
      try {
        const status = await getProxyStatus();
        setProxyEnabled(status.enabled);
        setLastBackupID(status.last_backup_id || "");
      } catch {
        // Keep UI usable even if status endpoint is temporarily unavailable.
      }
    })();
  }, []);

  async function handleToggleProxy(checked: boolean) {
    setProxyLoading(true);
    try {
      const status = checked ? await enableProxy() : await disableProxy();
      setProxyEnabled(status.enabled);
      setLastBackupID(status.last_backup_id || "");
    } catch {
      setProxyEnabled(!checked);
    } finally {
      setProxyLoading(false);
    }
  }

  return (
    <ConfigProvider
      theme={{
        token: {
          colorPrimary: "#1677ff",
          borderRadius: 14,
          colorBgLayout: "#f4f7fb",
          colorBgContainer: "#ffffff",
          colorBorderSecondary: "#e8edf5",
        },
      }}
    >
      <AntApp>
        <div className="app-shell">
          <header className="top-menu">
            <div className="brand">aigate</div>
            <div className="top-menu-right">
              <div className="proxy-panel">
                <span className="proxy-label">开启代理</span>
                <Switch checked={proxyEnabled} loading={proxyLoading} onChange={(checked) => void handleToggleProxy(checked)} />
                {lastBackupID ? <span className="proxy-backup">备份: {lastBackupID}</span> : null}
              </div>
              <div className="menu-actions">
              <button
                type="button"
                className={`menu-button ${tab === "accounts" ? "active" : ""}`}
                onClick={() => setTab("accounts")}
              >
                账号
              </button>
              <button
                type="button"
                className={`menu-button ${tab === "settings" ? "active" : ""}`}
                onClick={() => setTab("settings")}
              >
                设置
              </button>
              </div>
            </div>
          </header>
          {tab === "accounts" ? <AccountsPage /> : <SettingsPage />}
        </div>
      </AntApp>
    </ConfigProvider>
  );
}
