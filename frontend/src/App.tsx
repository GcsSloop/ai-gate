import { CloudDownloadOutlined, PlusOutlined } from "@ant-design/icons";
import { App as AntApp, Button, ConfigProvider, Dropdown, Modal, Spin, Switch, message, theme as antdTheme } from "antd";
import { useEffect, useMemo, useState } from "react";
import { invoke } from "@tauri-apps/api/core";

import { AccountsPage } from "./features/accounts/AccountsPage";
import { SettingsPage } from "./features/settings/SettingsPage";
import { StatsPage } from "./features/stats/StatsPage";
import { createDesktopUpdateService, type DesktopUpdateInfo } from "./features/updates/updateService";
import appLogo from "./assets/aigate_1024_1024.png";
import { type AppSettings, disableProxy, enableProxy, getAppSettings, getProxyStatus } from "./lib/api";
import { loadDesktopShellContext, refreshDesktopTrayState, subscribeDesktopBackendStateChanged } from "./lib/desktop-shell";
import { createTranslator, getAntdLocale, normalizeLanguage } from "./lib/i18n";
import { setAPIBase } from "./lib/paths";
import "./styles.css";

const appSettingsBootstrapRetryDelays = [0, 150, 300, 600, 1_000];
const homeUpdateCheckIntervalMs = 6 * 60 * 60 * 1_000;

type AppView = "accounts" | "stats" | "settings";

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => {
    window.setTimeout(resolve, ms);
  });
}

export function App() {
  const [messageApi, contextHolder] = message.useMessage();
  const [view, setView] = useState<AppView>("accounts");
  const [settingsInitialTab, setSettingsInitialTab] = useState<"general" | "proxy" | "advanced" | "about">("general");
  const [addModalMode, setAddModalMode] = useState<"official" | "third_party" | null>(null);
  const [proxyEnabled, setProxyEnabled] = useState(false);
  const [proxyLoading, setProxyLoading] = useState(false);
  const [accountsSyncToken, setAccountsSyncToken] = useState(0);
  const [appSettings, setAppSettings] = useState<AppSettings | null>(null);
  const [shellReady, setShellReady] = useState(false);
  const [systemPrefersDark, setSystemPrefersDark] = useState(false);
  const [homeUpdate, setHomeUpdate] = useState<DesktopUpdateInfo | null>(null);
  const updateService = useMemo(() => createDesktopUpdateService(), []);
  const language = normalizeLanguage(appSettings?.language);
  const t = createTranslator(language);
  const themeMode = appSettings?.theme_mode ?? "system";
  const resolvedThemeMode = themeMode === "system" ? (systemPrefersDark ? "dark" : "light") : themeMode;

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
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
      return;
    }
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    setSystemPrefersDark(media.matches);
    const handleChange = (event: MediaQueryListEvent) => {
      setSystemPrefersDark(event.matches);
    };
    media.addEventListener("change", handleChange);
    return () => {
      media.removeEventListener("change", handleChange);
    };
  }, []);

  useEffect(() => {
    const targets = [document.documentElement, document.body];
    targets.forEach((target) => {
      target.dataset.themeMode = resolvedThemeMode;
      target.dataset.themePreference = themeMode;
    });
    return () => {
      targets.forEach((target) => {
        delete target.dataset.themeMode;
        delete target.dataset.themePreference;
      });
    };
  }, [resolvedThemeMode, themeMode]);

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
    if (!appSettings?.show_home_update_indicator) {
      setHomeUpdate(null);
      return;
    }

    let disposed = false;

    async function checkForHomeUpdate() {
      try {
        const result = await updateService.check();
        if (!disposed) {
          setHomeUpdate(result.update);
        }
      } catch {
        if (!disposed) {
          setHomeUpdate(null);
        }
      }
    }

    void checkForHomeUpdate();
    const timer = window.setInterval(() => {
      void checkForHomeUpdate();
    }, homeUpdateCheckIntervalMs);

    return () => {
      disposed = true;
      window.clearInterval(timer);
    };
  }, [appSettings?.show_home_update_indicator, updateService]);

  useEffect(() => {
    const translate = createTranslator(language);
    const handler = (event: Event) => {
      const custom = event as CustomEvent<{ message?: string }>;
      const details = custom.detail?.message || "当前配置已被外部修改，无法自动恢复备份。";
      Modal.confirm({
        title: translate("退出前恢复失败"),
        content: language === "en-US" ? `${details} Force quit anyway?` : `${details} 是否强制退出？`,
        okText: translate("强制退出"),
        cancelText: translate("取消"),
        onOk: async () => {
          try {
            await disableProxy({ force: true });
          } catch (error) {
            void messageApi.warning(
              error instanceof Error ? `${translate("恢复失败，仍将退出")}: ${error.message}` : translate("恢复失败，仍将退出"),
            );
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
  }, [language, messageApi]);

  async function handleToggleProxy(checked: boolean) {
    setProxyLoading(true);
    try {
      const status = checked ? await enableProxy() : await disableProxy();
      setProxyEnabled(status.enabled);
      void refreshDesktopTrayState();
    } catch (error) {
      if (!checked && error instanceof Error && error.message.includes("config.toml changed externally")) {
        Modal.confirm({
          title: t("检测到配置冲突"),
          content:
            language === "en-US"
              ? "The current config.toml was modified externally. Choose whether to overwrite and disable, or disable the proxy without overwriting."
              : "当前 config.toml 已被外部修改。请选择关闭方式：覆盖恢复后关闭，或不覆盖直接关闭代理。",
          okText: t("覆盖并关闭"),
          cancelText: t("不覆盖直接关闭"),
          onOk: async () => {
            setProxyLoading(true);
            try {
              const status = await disableProxy({ force: true });
              setProxyEnabled(status.enabled);
              void refreshDesktopTrayState();
              void messageApi.success(t("代理已关闭"));
            } catch (forceError) {
              void messageApi.error(forceError instanceof Error ? forceError.message : t("覆盖恢复失败"));
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
              void messageApi.success(t("代理已关闭（未覆盖当前配置）"));
            } catch (cancelError) {
              void messageApi.error(cancelError instanceof Error ? cancelError.message : t("关闭代理失败"));
              setProxyEnabled(true);
            } finally {
              setProxyLoading(false);
            }
          },
        });
        return;
      }
      void messageApi.error(error instanceof Error ? error.message : t("代理切换失败，请检查配置冲突后重试"));
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
  const showHomeUpdateIndicator = Boolean(appSettings?.show_home_update_indicator && homeUpdate);
  const pageTitle = view === "accounts" ? t("账户") : view === "stats" ? t("统计") : t("设置");

  return (
    <ConfigProvider
      locale={getAntdLocale(language)}
      theme={{
        algorithm: resolvedThemeMode === "dark" ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
        token: {
          colorPrimary: "#3e5be8",
          borderRadius: 14,
          colorBgLayout: resolvedThemeMode === "dark" ? "#0f172a" : "#ffffff",
          colorBgContainer: resolvedThemeMode === "dark" ? "#111827" : "#ffffff",
          colorBorderSecondary: resolvedThemeMode === "dark" ? "#334155" : "#e5e7eb",
        },
      }}
    >
      <AntApp>
        <div className="app-theme-shell" data-theme-mode={resolvedThemeMode} data-theme-preference={themeMode}>
          {contextHolder}
          {!shellReady || !appSettings ? (
            <div className="app-loading">
              <Spin size="large" />
              <span>{t("正在载入设置中心…")}</span>
            </div>
          ) : (
            <div className="app-shell">
              <header className="top-menu">
                <div className="top-menu-section top-menu-section-left">
                  <div className="brand-block">
                    <img src={appLogo} alt="AI Gate" className="brand-logo" />
                    <div className="brand">AI Gate</div>
                  </div>
                  <div className="pill-switcher top-view-switcher" role="tablist" aria-label={t("主导航")}>
                    <button
                      type="button"
                      role="tab"
                      aria-selected={view === "accounts"}
                      className={view === "accounts" ? "pill-tab-button is-active" : "pill-tab-button"}
                      onClick={() => setView("accounts")}
                    >
                      {t("账户")}
                    </button>
                    <button
                      type="button"
                      role="tab"
                      aria-selected={view === "stats"}
                      className={view === "stats" ? "pill-tab-button is-active" : "pill-tab-button"}
                      onClick={() => setView("stats")}
                    >
                      {t("统计")}
                    </button>
                    <button
                      type="button"
                      role="tab"
                      aria-selected={view === "settings"}
                      className={view === "settings" ? "pill-tab-button is-active" : "pill-tab-button"}
                      onClick={() => {
                        setSettingsInitialTab("general");
                        setView("settings");
                      }}
                    >
                      {t("设置")}
                    </button>
                  </div>
                </div>

                <div className="top-menu-title">{pageTitle}</div>

                <div className="top-menu-section top-menu-right">
                  {showHomeUpdateIndicator ? (
                    <Button
                      type="text"
                      icon={<CloudDownloadOutlined />}
                      aria-label={t("打开更新")}
                      className="top-home-update-button"
                      onClick={() => {
                        setSettingsInitialTab("about");
                        setView("settings");
                      }}
                    />
                  ) : null}
                  {showProxySwitch ? (
                    <div className="proxy-panel">
                      <span className="proxy-label">{t("开启代理")}</span>
                      <Switch checked={proxyEnabled} loading={proxyLoading} onChange={(checked) => void handleToggleProxy(checked)} />
                    </div>
                  ) : null}
                  <Dropdown
                    trigger={["click"]}
                    menu={{
                      items: [
                        { key: "official", label: t("官方账户") },
                        { key: "third_party", label: t("第三方账户") },
                      ],
                      onClick: ({ key }) => setAddModalMode(key as "official" | "third_party"),
                    }}
                  >
                    <Button type="primary" shape="circle" icon={<PlusOutlined />} aria-label={t("添加账户")} className="global-add-button" />
                  </Dropdown>
                </div>
              </header>

              <div className="app-content-scroll">
                {view === "stats" ? (
                  <StatsPage language={language} t={t} />
                ) : view === "settings" ? (
                  <SettingsPage
                    initialSettings={appSettings}
                    initialTab={settingsInitialTab}
                    language={language}
                    t={t}
                    proxyEnabled={proxyEnabled}
                    onSettingsChanged={(next) => void handleSettingsChanged(next)}
                    onToggleProxy={(checked) => handleToggleProxy(checked)}
                  />
                ) : (
                  <AccountsPage
                    language={language}
                    t={t}
                    syncToken={accountsSyncToken}
                    showAddButton={false}
                    addModalMode={addModalMode}
                    onAddModalModeConsumed={() => setAddModalMode(null)}
                  />
                )}
              </div>
            </div>
          )}
        </div>
      </AntApp>
    </ConfigProvider>
  );
}
