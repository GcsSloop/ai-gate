import { ArrowLeftOutlined, PlusOutlined, SaveOutlined, SettingOutlined } from "@ant-design/icons";
import { App as AntApp, Button, ConfigProvider, Dropdown, Modal, Spin, Switch, message } from "antd";
import { useEffect, useRef, useState } from "react";
import { invoke } from "@tauri-apps/api/core";

import { AccountsPage } from "./features/accounts/AccountsPage";
import { SettingsPage } from "./features/settings/SettingsPage";
import appLogo from "./assets/aigate_1024_1024.png";
import { type AppSettings, disableProxy, enableProxy, getAppSettings, getProxyStatus } from "./lib/api";
import { loadDesktopShellContext, refreshDesktopTrayState, subscribeDesktopBackendStateChanged } from "./lib/desktop-shell";
import { setAPIBase } from "./lib/paths";
import "./styles.css";

const appSettingsBootstrapRetryDelays = [0, 150, 300, 600, 1_000];

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => {
    window.setTimeout(resolve, ms);
  });
}

export function App() {
  const [messageApi, contextHolder] = message.useMessage();
  const [view, setView] = useState<"accounts" | "settings">("accounts");
  const [addModalMode, setAddModalMode] = useState<"official" | "third_party" | null>(null);
  const [proxyEnabled, setProxyEnabled] = useState(false);
  const [proxyLoading, setProxyLoading] = useState(false);
  const [accountsSyncToken, setAccountsSyncToken] = useState(0);
  const [appSettings, setAppSettings] = useState<AppSettings | null>(null);
  const [shellReady, setShellReady] = useState(false);
  const settingsSaveActionRef = useRef<(() => void) | null>(null);

  async function refreshProxyState() {
    try {
      const status = await getProxyStatus();
      setProxyEnabled(status.enabled);
    } catch {
      // Keep UI usable even if status endpoint is temporarily unavailable.
    }
  }

  async function refreshAppSettingsState() {
    const settings = await getAppSettings();
    setAppSettings(settings);
    return settings;
  }

  async function bootstrapAppSettingsState() {
    let lastError: unknown;
    for (let attempt = 0; attempt < appSettingsBootstrapRetryDelays.length; attempt += 1) {
      if (attempt > 0) {
        await sleep(appSettingsBootstrapRetryDelays[attempt]);
      }
      try {
        return await refreshAppSettingsState();
      } catch (error) {
        lastError = error;
      }
    }
    throw lastError instanceof Error ? lastError : new Error("failed to fetch app settings");
  }

  useEffect(() => {
    let disposed = false;

    async function boot() {
      try {
        const shellContext = await loadDesktopShellContext();
        if (shellContext?.backend_api_base) {
          setAPIBase(shellContext.backend_api_base);
        }
      } catch {
        // Fall back to the default API base in browser mode.
      }

      try {
        await Promise.all([refreshProxyState(), bootstrapAppSettingsState()]);
        void refreshDesktopTrayState();
      } catch (error) {
        if (!disposed) {
          void messageApi.error(error instanceof Error ? error.message : "初始化设置中心失败");
        }
      } finally {
        if (!disposed) {
          setShellReady(true);
        }
      }
    }

    void boot();
    return () => {
      disposed = true;
    };
  }, [messageApi]);

  useEffect(() => {
    let disposed = false;
    let unlisten: undefined | (() => void);
    void subscribeDesktopBackendStateChanged(() => {
      void refreshProxyState();
      void refreshDesktopTrayState();
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
  }, []);

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
              void refreshDesktopTrayState();
              void messageApi.success("代理已关闭");
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

  async function handleSettingsChanged(next: AppSettings) {
    setAppSettings(next);
    await Promise.all([refreshProxyState(), refreshDesktopTrayState()]);
    setAccountsSyncToken((value) => value + 1);
  }

  const showProxySwitch = appSettings?.show_proxy_switch_on_home ?? true;

  return (
    <ConfigProvider
      theme={{
        token: {
          colorPrimary: "#3e5be8",
          borderRadius: 14,
          colorBgLayout: "#ffffff",
          colorBgContainer: "#ffffff",
          colorBorderSecondary: "#e5e7eb",
        },
      }}
    >
      <AntApp>
        {contextHolder}
        {!shellReady || !appSettings ? (
          <div className="app-loading">
            <Spin size="large" />
            <span>正在载入设置中心…</span>
          </div>
        ) : view === "settings" ? (
          <div className="settings-screen">
            <header className="settings-top-bar">
              <div className="settings-top-left">
                <Button type="text" icon={<ArrowLeftOutlined />} aria-label="返回首页" onClick={() => setView("accounts")} />
                <span className="settings-top-title">设置</span>
              </div>
              <Button
                aria-label="保存设置"
                type="primary"
                size="large"
                icon={<SaveOutlined />}
                onClick={() => settingsSaveActionRef.current?.()}
              >
                保存设置
              </Button>
            </header>
            <div className="settings-content-scroll">
              <SettingsPage
                initialSettings={appSettings}
                proxyEnabled={proxyEnabled}
                onSettingsChanged={(next) => void handleSettingsChanged(next)}
                onToggleProxy={(checked) => handleToggleProxy(checked)}
                hideLocalSaveButton
                onRegisterSaveHandler={(handler) => {
                  settingsSaveActionRef.current = handler;
                }}
              />
            </div>
          </div>
        ) : (
          <div className="app-shell">
            <header className="top-menu">
              <div className="brand-block">
                <img src={appLogo} alt="AI Gate" className="brand-logo" />
                <div className="brand">AI Gate</div>
                <Button
                  type="text"
                  icon={<SettingOutlined />}
                  aria-label="打开设置"
                  className="top-settings-button"
                  onClick={() => setView("settings")}
                />
              </div>
              <div className="top-menu-right">
                {showProxySwitch ? (
                  <div className="proxy-panel">
                    <span className="proxy-label">开启代理</span>
                    <Switch checked={proxyEnabled} loading={proxyLoading} onChange={(checked) => void handleToggleProxy(checked)} />
                  </div>
                ) : null}
                <Dropdown
                  trigger={["click"]}
                  menu={{
                    items: [
                      { key: "official", label: "官方账户" },
                      { key: "third_party", label: "第三方账户" },
                    ],
                    onClick: ({ key }) => setAddModalMode(key as "official" | "third_party"),
                  }}
                >
                  <Button type="primary" shape="circle" icon={<PlusOutlined />} aria-label="添加账户" className="global-add-button" />
                </Dropdown>
              </div>
            </header>
            <AccountsPage
              syncToken={accountsSyncToken}
              showAddButton={false}
              addModalMode={addModalMode}
              onAddModalModeConsumed={() => setAddModalMode(null)}
            />
          </div>
        )}
      </AntApp>
    </ConfigProvider>
  );
}
