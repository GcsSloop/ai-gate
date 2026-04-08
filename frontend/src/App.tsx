import { BarChartOutlined, CloudDownloadOutlined, DeploymentUnitOutlined, PlusOutlined, ReadOutlined, SettingOutlined, UserOutlined } from "@ant-design/icons";
import { App as AntApp, Button, ConfigProvider, Dropdown, Modal, Spin, Switch, Tooltip, message, theme as antdTheme } from "antd";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { invoke } from "@tauri-apps/api/core";

import { AccountsPage } from "./features/accounts/AccountsPage";
import { ToolingPage } from "./features/tooling/ToolingPage";
import { SettingsPage } from "./features/settings/SettingsPage";
import { StatsPage } from "./features/stats/StatsPage";
import { HomeUpdatePanel } from "./features/updates/HomeUpdatePanel";
import { createDesktopUpdateService, type DesktopUpdateInfo } from "./features/updates/updateService";
import appLogo from "./assets/aigate_1024_1024.png";
import { type AppSettings, disableProxy, enableProxy, getAppSettings, getProxyStatus, refreshAccountUsage, subscribeAccountRoutingStateChanged } from "./lib/api";
import { loadDesktopShellContext, refreshDesktopTrayState, subscribeDesktopBackendStateChanged } from "./lib/desktop-shell";
import { createTranslator, getAntdLocale, normalizeLanguage } from "./lib/i18n";
import { setAPIBase } from "./lib/paths";
import "./styles.css";

const appSettingsBootstrapRetryDelays = [0, 150, 300, 600, 1_000];
const homeUpdateCheckIntervalMs = 60 * 60 * 1_000;
const defaultStatusRefreshIntervalSeconds = 60;
const immediateUsageRefreshDebounceMs = 400;
const resumeGapWatchIntervalMs = 5_000;
const resumeGapThresholdMs = 30_000;
const hiddenResumeThresholdMs = 15_000;

type AppView = "accounts" | "stats" | "skills" | "mcp" | "settings";

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => {
    window.setTimeout(resolve, ms);
  });
}

export function App() {
  const [messageApi, contextHolder] = message.useMessage();
  const [view, setView] = useState<AppView>("accounts");
  const [settingsInitialTab, setSettingsInitialTab] = useState<"general" | "proxy" | "advanced" | "about">("general");
  const [addModalMode, setAddModalMode] = useState<"official" | "third_party" | "shared_import" | null>(null);
  const [proxyEnabled, setProxyEnabled] = useState(false);
  const [proxyLoading, setProxyLoading] = useState(false);
  const [accountsSyncToken, setAccountsSyncToken] = useState(0);
  const [appSettings, setAppSettings] = useState<AppSettings | null>(null);
  const [shellReady, setShellReady] = useState(false);
  const [systemPrefersDark, setSystemPrefersDark] = useState(false);
  const [homeUpdate, setHomeUpdate] = useState<DesktopUpdateInfo | null>(null);
  const [homeUpdateModalOpen, setHomeUpdateModalOpen] = useState(false);
  const updateService = useMemo(() => createDesktopUpdateService(), []);
  const previousViewRef = useRef<AppView>("accounts");
  const immediateUsageRefreshInFlightRef = useRef(false);
  const immediateUsageRefreshPendingRef = useRef(false);
  const immediateUsageRefreshTimerRef = useRef<number | null>(null);
  const hiddenSinceRef = useRef<number | null>(null);
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

  const checkForHomeUpdate = useCallback(async () => {
    try {
      const result = await updateService.check();
      setHomeUpdate(result.update);
    } catch {
      setHomeUpdate(null);
    }
  }, [updateService]);

  const runImmediateUsageRefresh = useCallback(async () => {
    if (immediateUsageRefreshInFlightRef.current) {
      immediateUsageRefreshPendingRef.current = true;
      return;
    }
    immediateUsageRefreshInFlightRef.current = true;
    try {
      do {
        immediateUsageRefreshPendingRef.current = false;
        try {
          await refreshAccountUsage();
        } catch {
          // Network and wake-up recovery should stay silent and retry on the next trigger.
        }
        setAccountsSyncToken((value) => value + 1);
        void refreshDesktopTrayState();
      } while (immediateUsageRefreshPendingRef.current);
    } finally {
      immediateUsageRefreshInFlightRef.current = false;
    }
  }, []);

  const queueImmediateUsageRefresh = useCallback(
    (trigger: "online" | "resume_gap" | "visibility_resume") => {
      if (!shellReady || typeof window === "undefined") {
        return;
      }
      if (trigger === "online" && typeof navigator !== "undefined" && navigator.onLine === false) {
        return;
      }
      if (immediateUsageRefreshTimerRef.current !== null) {
        window.clearTimeout(immediateUsageRefreshTimerRef.current);
      }
      immediateUsageRefreshTimerRef.current = window.setTimeout(() => {
        immediateUsageRefreshTimerRef.current = null;
        void runImmediateUsageRefresh();
      }, immediateUsageRefreshDebounceMs);
    },
    [runImmediateUsageRefresh, shellReady],
  );

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
          void messageApi.error(error instanceof Error ? t(error.message) : t("初始化设置中心失败"));
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
    document.documentElement.lang = language;
    return () => {
      targets.forEach((target) => {
        delete target.dataset.themeMode;
        delete target.dataset.themePreference;
      });
    };
  }, [language, resolvedThemeMode, themeMode]);

  useEffect(() => {
    if (!shellReady) {
      return;
    }
    let disposed = false;
    let unlisten: undefined | (() => void);
    const handleBackendStateChanged = () => {
      void refreshProxyState();
      void refreshDesktopTrayState();
      setAccountsSyncToken((value) => value + 1);
    };
    const disposeBackendEvents = subscribeAccountRoutingStateChanged(handleBackendStateChanged);
    void subscribeDesktopBackendStateChanged(handleBackendStateChanged).then((cleanup) => {
      if (disposed) {
        cleanup();
        return;
      }
      unlisten = cleanup;
    });
    return () => {
      disposed = true;
      disposeBackendEvents();
      unlisten?.();
    };
  }, [shellReady]);

  useEffect(() => {
    if (!shellReady || typeof window === "undefined") {
      return;
    }

    let lastTickAt = Date.now();
    const handleOnline = () => {
      queueImmediateUsageRefresh("online");
    };
    const handleVisibilityChange = () => {
      if (document.visibilityState === "hidden") {
        hiddenSinceRef.current = Date.now();
        return;
      }
      const hiddenSince = hiddenSinceRef.current;
      hiddenSinceRef.current = null;
      if (hiddenSince !== null && Date.now() - hiddenSince >= hiddenResumeThresholdMs) {
        queueImmediateUsageRefresh("visibility_resume");
      }
    };
    const timer = window.setInterval(() => {
      const now = Date.now();
      const elapsed = now - lastTickAt;
      lastTickAt = now;
      if (elapsed >= resumeGapThresholdMs) {
        queueImmediateUsageRefresh("resume_gap");
      }
    }, resumeGapWatchIntervalMs);

    window.addEventListener("online", handleOnline);
    document.addEventListener("visibilitychange", handleVisibilityChange);
    return () => {
      window.removeEventListener("online", handleOnline);
      document.removeEventListener("visibilitychange", handleVisibilityChange);
      window.clearInterval(timer);
      if (immediateUsageRefreshTimerRef.current !== null) {
        window.clearTimeout(immediateUsageRefreshTimerRef.current);
        immediateUsageRefreshTimerRef.current = null;
      }
      hiddenSinceRef.current = null;
    };
  }, [queueImmediateUsageRefresh, shellReady]);

  useEffect(() => {
    if (!appSettings) {
      return;
    }

    const refreshIntervalSeconds = Math.min(Math.max(appSettings.status_refresh_interval_seconds ?? defaultStatusRefreshIntervalSeconds, 5), 3600);
    const timer = window.setInterval(() => {
      void refreshProxyState();
      void refreshDesktopTrayState();
      setAccountsSyncToken((value) => value + 1);
    }, refreshIntervalSeconds * 1_000);

    return () => {
      window.clearInterval(timer);
    };
  }, [appSettings]);

  useEffect(() => {
    if (!appSettings?.show_home_update_indicator) {
      setHomeUpdate(null);
      return;
    }

    void checkForHomeUpdate();
    const timer = window.setInterval(() => {
      void checkForHomeUpdate();
    }, homeUpdateCheckIntervalMs);

    return () => {
      window.clearInterval(timer);
    };
  }, [appSettings?.show_home_update_indicator, checkForHomeUpdate]);

  useEffect(() => {
    const previousView = previousViewRef.current;
    previousViewRef.current = view;
    if (!appSettings?.show_home_update_indicator) {
      return;
    }
    if (view === "accounts" && previousView !== "accounts") {
      void checkForHomeUpdate();
    }
  }, [appSettings?.show_home_update_indicator, checkForHomeUpdate, view]);

  useEffect(() => {
    const translate = createTranslator(language);
    const handler = (event: Event) => {
      const custom = event as CustomEvent<{ message?: string }>;
      const details = custom.detail?.message || translate("当前配置已被外部修改，无法自动恢复备份。");
      Modal.confirm({
        title: translate("退出前恢复失败"),
        content: `${details} ${translate("是否强制退出？")}`,
        okText: translate("强制退出"),
        cancelText: translate("取消"),
        centered: true,
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
          content: t("当前 config.toml 已被外部修改。请选择关闭方式：覆盖恢复后关闭，或不覆盖直接关闭代理。"),
          okText: t("覆盖并关闭"),
          cancelText: t("不覆盖直接关闭"),
          centered: true,
          onOk: async () => {
            setProxyLoading(true);
            try {
              const status = await disableProxy({ force: true });
              setProxyEnabled(status.enabled);
              void refreshDesktopTrayState();
              void messageApi.success(t("代理已关闭"));
            } catch (forceError) {
              void messageApi.error(forceError instanceof Error ? t(forceError.message) : t("覆盖恢复失败"));
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
              void messageApi.error(cancelError instanceof Error ? t(cancelError.message) : t("关闭代理失败"));
              setProxyEnabled(true);
            } finally {
              setProxyLoading(false);
            }
          },
        });
        return;
      }
      void messageApi.error(error instanceof Error ? t(error.message) : t("代理切换失败，请检查配置冲突后重试"));
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
                  <a href="https://github.com/GcsSloop/ai-gate" target="_blank" rel="noreferrer" className="brand-block" aria-label="AI Gate">
                    <img src={appLogo} alt="AI Gate" className="brand-logo" />
                    <div className="brand">AI Gate</div>
                  </a>
                  <div className="menu-pill-group top-view-switcher" role="tablist" aria-label={t("主导航")}>
                    <Tooltip title={t("账户")} placement="bottom">
                      <button
                        type="button"
                        role="tab"
                        aria-label={t("账户")}
                        aria-selected={view === "accounts"}
                        className={view === "accounts" ? "menu-pill-button menu-pill-button-icon is-active" : "menu-pill-button menu-pill-button-icon"}
                        onClick={() => setView("accounts")}
                      >
                        <UserOutlined />
                      </button>
                    </Tooltip>
                    <Tooltip title={t("统计")} placement="bottom">
                      <button
                        type="button"
                        role="tab"
                        aria-label={t("统计")}
                        aria-selected={view === "stats"}
                        className={view === "stats" ? "menu-pill-button menu-pill-button-icon is-active" : "menu-pill-button menu-pill-button-icon"}
                        onClick={() => setView("stats")}
                      >
                        <BarChartOutlined />
                      </button>
                    </Tooltip>
                    <Tooltip title="Skill" placement="bottom">
                      <button
                        type="button"
                        role="tab"
                        aria-label="Skill"
                        aria-selected={view === "skills"}
                        className={view === "skills" ? "menu-pill-button menu-pill-button-icon is-active" : "menu-pill-button menu-pill-button-icon"}
                        onClick={() => setView("skills")}
                      >
                        <ReadOutlined />
                      </button>
                    </Tooltip>
                    <Tooltip title="MCP" placement="bottom">
                      <button
                        type="button"
                        role="tab"
                        aria-label="MCP"
                        aria-selected={view === "mcp"}
                        className={view === "mcp" ? "menu-pill-button menu-pill-button-icon is-active" : "menu-pill-button menu-pill-button-icon"}
                        onClick={() => setView("mcp")}
                      >
                        <DeploymentUnitOutlined />
                      </button>
                    </Tooltip>
                    <Tooltip title={t("设置")} placement="bottom">
                      <button
                        type="button"
                        role="tab"
                        aria-label={t("设置")}
                        aria-selected={view === "settings"}
                        className={view === "settings" ? "menu-pill-button menu-pill-button-icon is-active" : "menu-pill-button menu-pill-button-icon"}
                        onClick={() => {
                          setSettingsInitialTab("general");
                          setView("settings");
                        }}
                      >
                        <SettingOutlined />
                      </button>
                    </Tooltip>
                  </div>
                </div>

                <div className="top-menu-section top-menu-right">
                  {showHomeUpdateIndicator ? (
                    <Button
                      type="text"
                      icon={
                        <span className="top-home-update-icon" aria-hidden="true">
                          <CloudDownloadOutlined />
                          <span className="top-home-update-dot" />
                        </span>
                      }
                      aria-label={t("打开更新")}
                      className="top-home-update-button"
                      onClick={() => setHomeUpdateModalOpen(true)}
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
                        { key: "shared_import", label: t("导入账户") },
                      ],
                      onClick: ({ key }) => setAddModalMode(key as "official" | "third_party" | "shared_import"),
                    }}
                  >
                    <Button type="primary" shape="circle" icon={<PlusOutlined />} aria-label={t("添加账户")} className="global-add-button" />
                  </Dropdown>
                </div>
              </header>

              <div className="app-content-scroll">
                {view === "stats" ? (
                  <StatsPage language={language} t={t} />
                ) : view === "skills" ? (
                  <ToolingPage mode="skills" t={t} />
                ) : view === "mcp" ? (
                  <ToolingPage mode="mcp" t={t} />
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
              <Modal
                open={homeUpdateModalOpen}
                title={t("应用更新")}
                footer={null}
                onCancel={() => setHomeUpdateModalOpen(false)}
                destroyOnHidden
                centered
                width={720}
              >
                <HomeUpdatePanel
                  currentVersion={homeUpdate?.currentVersion ?? ""}
                  initialUpdate={homeUpdate}
                  language={language}
                  t={t}
                  service={updateService}
                />
              </Modal>
            </div>
          )}
        </div>
      </AntApp>
    </ConfigProvider>
  );
}
