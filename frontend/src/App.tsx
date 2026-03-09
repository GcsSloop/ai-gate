import { App as AntApp, ConfigProvider, Modal, Switch, message } from "antd";
import { useCallback, useEffect, useState } from "react";
import { invoke } from "@tauri-apps/api/core";

import { AccountsPage } from "./features/accounts/AccountsPage";
import { SettingsPage } from "./features/settings/SettingsPage";
import { disableProxy, enableProxy, getProxyStatus } from "./lib/api";
import { refreshDesktopTrayState, subscribeDesktopBackendStateChanged } from "./lib/desktop-shell";
import "./styles.css";

export function App() {
  const [messageApi, contextHolder] = message.useMessage();
  const [tab, setTab] = useState<"accounts" | "settings">("accounts");
  const [proxyEnabled, setProxyEnabled] = useState(false);
  const [lastBackupID, setLastBackupID] = useState("");
  const [proxyLoading, setProxyLoading] = useState(false);
  const [accountsSyncToken, setAccountsSyncToken] = useState(0);

  const refreshProxyState = useCallback(async () => {
    try {
      const status = await getProxyStatus();
      setProxyEnabled(status.enabled);
      setLastBackupID(status.last_backup_id || "");
    } catch {
      // Keep UI usable even if status endpoint is temporarily unavailable.
    }
  }, []);

  useEffect(() => {
    void refreshProxyState();
  }, [refreshProxyState]);

  useEffect(() => {
    let disposed = false;
    let unlisten: undefined | (() => void);
    void subscribeDesktopBackendStateChanged(() => {
      void refreshProxyState();
      setAccountsSyncToken((value) => value + 1);
    }).then((cleanup) => {
      if (disposed) {
        cleanup();
        return;
      }
      unlisten = cleanup;
    });
    return () => {
      disposed = true;
      unlisten?.();
    };
  }, [refreshProxyState]);

  useEffect(() => {
    const handler = (event: Event) => {
      const custom = event as CustomEvent<{ message?: string }>;
      const details = custom.detail?.message || "当前配置已被外部修改，无法自动恢复备份。";
      Modal.confirm({
        title: "退出前恢复失败",
        content: `${details} 是否强制退出？`,
        okText: "强制退出",
        cancelText: "取消",
        onOk: async () => {
          try {
            await disableProxy({ force: true });
          } catch (error) {
            void messageApi.warning(error instanceof Error ? `恢复失败，仍将退出：${error.message}` : "恢复失败，仍将退出");
          } finally {
            await invoke("force_exit_app");
          }
        },
      });
    };
    window.addEventListener("aigate-exit-conflict", handler as EventListener);
    return () => {
      window.removeEventListener("aigate-exit-conflict", handler as EventListener);
    };
  }, [messageApi]);

  async function handleToggleProxy(checked: boolean) {
    setProxyLoading(true);
    try {
      const status = checked ? await enableProxy() : await disableProxy();
      setProxyEnabled(status.enabled);
      setLastBackupID(status.last_backup_id || "");
      void refreshDesktopTrayState();
    } catch (error) {
      if (!checked && error instanceof Error && error.message.includes("config.toml changed externally")) {
        Modal.confirm({
          title: "检测到配置冲突",
          content: "当前 config.toml 已被外部修改。请选择关闭方式：覆盖恢复后关闭，或不覆盖直接关闭代理。",
          okText: "覆盖并关闭",
          cancelText: "不覆盖直接关闭",
          onOk: async () => {
            setProxyLoading(true);
            try {
              const status = await disableProxy({ force: true });
              setProxyEnabled(status.enabled);
              setLastBackupID(status.last_backup_id || "");
              void refreshDesktopTrayState();
              void messageApi.success("代理已关闭并恢复备份");
            } catch (forceError) {
              void messageApi.error(forceError instanceof Error ? forceError.message : "覆盖恢复失败");
              setProxyEnabled(true);
            } finally {
              setProxyLoading(false);
            }
          },
          onCancel: async () => {
            setProxyLoading(true);
            try {
              const status = await disableProxy({ skipRestore: true });
              setProxyEnabled(status.enabled);
              setLastBackupID(status.last_backup_id || "");
              void refreshDesktopTrayState();
              void messageApi.success("代理已关闭（未覆盖当前配置）");
            } catch (cancelError) {
              void messageApi.error(cancelError instanceof Error ? cancelError.message : "关闭代理失败");
              setProxyEnabled(true);
            } finally {
              setProxyLoading(false);
            }
          },
        });
        return;
      }
      void messageApi.error(error instanceof Error ? error.message : "代理切换失败，请检查配置冲突后重试");
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
        {contextHolder}
        <div className="app-shell">
          <header className="top-menu">
            <div className="brand">AI Gate</div>
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
          <div style={{ display: tab === "accounts" ? "block" : "none" }}>
            <AccountsPage syncToken={accountsSyncToken} />
          </div>
          <div style={{ display: tab === "settings" ? "block" : "none" }}>
            <SettingsPage />
          </div>
        </div>
      </AntApp>
    </ConfigProvider>
  );
}
