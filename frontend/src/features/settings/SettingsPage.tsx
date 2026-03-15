import {
  ApiOutlined,
  ArrowDownOutlined,
  ArrowUpOutlined,
  CheckCircleFilled,
  CloudDownloadOutlined,
  CloudUploadOutlined,
  ControlOutlined,
  DatabaseOutlined,
  DeleteOutlined,
  DesktopOutlined,
  EyeInvisibleOutlined,
  FileTextOutlined,
  InfoCircleOutlined,
  MoreOutlined,
  PoweroffOutlined,
  RollbackOutlined,
  SaveOutlined,
  SwapOutlined,
  ThunderboltOutlined,
} from "@ant-design/icons";
import { Avatar, Button, Card, Input, InputNumber, Modal, Radio, Switch, Tag, Typography, message } from "antd";
import type { ReactNode } from "react";
import { useEffect, useState } from "react";

import {
  type AccountRecord,
  type AppSettings,
  type DatabaseBackupItem,
  createDatabaseBackup,
  deleteDatabaseBackup,
  exportDatabaseSQL,
  getAppSettings,
  getFailoverQueue,
  importDatabaseSQL,
  listAccounts,
  listDatabaseBackups,
  restoreDatabaseBackup,
  saveAppSettings,
  saveFailoverQueue,
} from "../../lib/api";
import {
  applyDesktopAppSettings,
  getAppMetadata,
  getRecentDesktopLogs,
  type AppMetadata,
  type DesktopRecentLog,
} from "../../lib/desktop-shell";
import type { AppLanguage, Translator } from "../../lib/i18n";
import { setAPIBase } from "../../lib/paths";
import appLogo from "../../assets/aigate_1024_1024.png";
import { UpdateCard } from "../updates/UpdateCard";

const { Text, Title } = Typography;

type SettingsTabKey = "general" | "proxy" | "advanced" | "about";

type SettingsPageProps = {
  initialSettings: AppSettings;
  initialTab?: SettingsTabKey;
  language: AppLanguage;
  t: Translator;
  proxyEnabled: boolean;
  onSettingsChanged: (next: AppSettings) => void | Promise<void>;
  onToggleProxy?: (checked: boolean) => void | Promise<void>;
};

function formatDateTime(value: string, language: AppLanguage): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString(language, { hour12: false });
}

function formatBytes(value: number): string {
  if (value <= 0) {
    return "0 B";
  }
  if (value < 1024) {
    return `${value} B`;
  }
  if (value < 1024 * 1024) {
    return `${(value / 1024).toFixed(1)} KB`;
  }
  return `${(value / 1024 / 1024).toFixed(1)} MB`;
}

function normalizeQueue(accounts: AccountRecord[], explicitOrder: number[]): number[] {
  const accountIDs = new Set(accounts.map((account) => account.id));
  const seen = new Set<number>();
  const ordered: number[] = [];

  explicitOrder.forEach((accountID) => {
    if (accountIDs.has(accountID) && !seen.has(accountID)) {
      seen.add(accountID);
      ordered.push(accountID);
    }
  });

  accounts.forEach((account) => {
    if (!seen.has(account.id)) {
      seen.add(account.id);
      ordered.push(account.id);
    }
  });

  return ordered;
}

function triggerTextDownload(filename: string, content: string) {
  if (typeof document === "undefined" || typeof URL === "undefined" || typeof URL.createObjectURL !== "function") {
    return;
  }

  const blob = new Blob([content], { type: "text/plain;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  anchor.click();
  URL.revokeObjectURL(url);
}

function validateExchangePayload(raw: string): void {
  let payload: { format?: string; version?: number };
  try {
    payload = JSON.parse(raw) as { format?: string; version?: number };
  } catch {
    throw new Error("当前后端导出格式不受支持，请升级后端后重试");
  }
  if (payload.format !== "aigate-db-exchange" || payload.version !== 1) {
    throw new Error("当前后端导出格式不受支持，请升级后端后重试");
  }
}

function SectionHeader(props: { icon: ReactNode; title: string; description: string; actions?: ReactNode }) {
  return (
    <div className="settings-section-header">
      <div className="settings-section-main">
        <div className="settings-section-icon">{props.icon}</div>
        <div>
          <div className="settings-section-title">{props.title}</div>
          <div className="settings-section-description">{props.description}</div>
        </div>
      </div>
      {props.actions ? <div className="settings-section-actions">{props.actions}</div> : null}
    </div>
  );
}

function ToggleRow(props: {
  icon: ReactNode;
  title: string;
  description: string;
  checked: boolean;
  label: string;
  onChange: (checked: boolean) => void;
  loading?: boolean;
}) {
  return (
    <div className="settings-toggle-row">
      <div className="settings-toggle-copy">
        <div className="settings-toggle-icon">{props.icon}</div>
        <div>
          <div className="settings-toggle-title">{props.title}</div>
          <div className="settings-toggle-description">{props.description}</div>
        </div>
      </div>
      <Switch aria-label={props.label} checked={props.checked} loading={props.loading} onChange={props.onChange} />
    </div>
  );
}

export function SettingsPage({
  initialSettings,
  initialTab = "general",
  language,
  t,
  proxyEnabled,
  onSettingsChanged,
  onToggleProxy,
}: SettingsPageProps) {
  const [messageApi, contextHolder] = message.useMessage();
  const [draftSettings, setDraftSettings] = useState<AppSettings>(initialSettings);
  const [accounts, setAccounts] = useState<AccountRecord[]>([]);
  const [failoverQueue, setFailoverQueue] = useState<number[]>([]);
  const [dbBackups, setDbBackups] = useState<DatabaseBackupItem[]>([]);
  const [metadata, setMetadata] = useState<AppMetadata>({
    name: "AI Gate",
    version: "0.1.0",
    description: "AI Gate 是一个本地桌面代理与账号编排工具，用于统一管理路由、故障转移与数据备份。",
    author: "GcsSloop",
  });
  const [recentDesktopLogs, setRecentDesktopLogs] = useState<DesktopRecentLog[]>([]);
  const [savingSettings, setSavingSettings] = useState(false);
  const [autoSavingPreference, setAutoSavingPreference] = useState(false);
  const [savingQueue, setSavingQueue] = useState(false);
  const [proxySwitchBusy, setProxySwitchBusy] = useState(false);
  const [backupBusy, setBackupBusy] = useState("");
  const [openBackupMenuID, setOpenBackupMenuID] = useState<string | null>(null);
  const [importingSQL, setImportingSQL] = useState(false);
  const [importModalOpen, setImportModalOpen] = useState(false);
  const [importFile, setImportFile] = useState<File | null>(null);
  const [activeTab, setActiveTab] = useState<SettingsTabKey>(initialTab);

  useEffect(() => {
    setDraftSettings(initialSettings);
  }, [initialSettings]);

  useEffect(() => {
    setActiveTab(initialTab);
  }, [initialTab]);

  useEffect(() => {
    async function loadSettingsPageData() {
      try {
        const [accountList, queue, backups, about] = await Promise.all([
          listAccounts(),
          getFailoverQueue(),
          listDatabaseBackups(),
          getAppMetadata(),
        ]);
        const logs = await getRecentDesktopLogs(50);
        setAccounts(accountList);
        setFailoverQueue(normalizeQueue(accountList, queue));
        setDbBackups(backups);
        setMetadata(about);
        setRecentDesktopLogs(logs);
      } catch (error) {
        void messageApi.error(error instanceof Error ? error.message : t("加载设置数据失败"));
      }
    }

    void loadSettingsPageData();
  }, [messageApi, t]);

  useEffect(() => {
    if (!openBackupMenuID) {
      return;
    }
    const handlePointerDown = (event: MouseEvent) => {
      const target = event.target;
      if (!(target instanceof Element)) {
        setOpenBackupMenuID(null);
        return;
      }
      const owner = target.closest(`[data-backup-menu-id="${openBackupMenuID}"]`);
      if (!owner) {
        setOpenBackupMenuID(null);
      }
    };
    document.addEventListener("mousedown", handlePointerDown);
    return () => {
      document.removeEventListener("mousedown", handlePointerDown);
    };
  }, [openBackupMenuID]);

  const orderedAccounts = failoverQueue
    .map((accountID) => accounts.find((account) => account.id === accountID))
    .filter((account): account is AccountRecord => Boolean(account));

  function updateDraft(patch: Partial<AppSettings>) {
    setDraftSettings((current) => ({
      ...current,
      ...patch,
    }));
  }

  function formatDesktopLogTime(value: number): string {
    if (!Number.isFinite(value) || value <= 0) {
      return "--";
    }
    return new Date(value).toLocaleString(language, { hour12: false });
  }

  function moveQueueItem(index: number, direction: -1 | 1) {
    const target = index + direction;
    if (target < 0 || target >= failoverQueue.length) {
      return;
    }
    const next = [...failoverQueue];
    [next[index], next[target]] = [next[target], next[index]];
    setFailoverQueue(next);
  }

  async function handleSaveSettings() {
    setSavingSettings(true);
    try {
      const saved = await saveAppSettings(draftSettings);
      const shellContext = await applyDesktopAppSettings(saved);
      if (shellContext?.backend_api_base) {
        setAPIBase(shellContext.backend_api_base);
      }
      setDraftSettings(saved);
      await onSettingsChanged(saved);
      void messageApi.success(t("设置已保存"));
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : t("保存设置失败"));
    } finally {
      setSavingSettings(false);
    }
  }

  async function handleAutoSavePreference(patch: Partial<AppSettings>) {
    const previous = draftSettings;
    const next = {
      ...previous,
      ...patch,
    };
    setDraftSettings(next);
    setAutoSavingPreference(true);
    try {
      const saved = await saveAppSettings(next);
      const shellContext = await applyDesktopAppSettings(saved);
      if (shellContext?.backend_api_base) {
        setAPIBase(shellContext.backend_api_base);
      }
      setDraftSettings(saved);
      await onSettingsChanged(saved);
    } catch (error) {
      setDraftSettings(previous);
      void messageApi.error(error instanceof Error ? error.message : t("保存设置失败"));
    } finally {
      setAutoSavingPreference(false);
    }
  }

  async function handleSaveQueue() {
    setSavingQueue(true);
    try {
      await saveFailoverQueue(failoverQueue);
      void messageApi.success(t("故障转移队列已更新"));
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : t("保存故障转移队列失败"));
    } finally {
      setSavingQueue(false);
    }
  }

  async function handleProxyToggle(checked: boolean) {
    if (!onToggleProxy) {
      return;
    }
    setProxySwitchBusy(true);
    try {
      await onToggleProxy(checked);
    } finally {
      setProxySwitchBusy(false);
    }
  }

  async function handleExportSQL() {
    try {
      const raw = await exportDatabaseSQL();
      validateExchangePayload(raw);
      const filename = `aigate-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, "-")}.json`;
      triggerTextDownload(filename, raw);
      void messageApi.success(t("JSON 已导出"));
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : t("导出 JSON 失败"));
    }
  }

  async function handleImportSQL() {
    if (!importFile) {
      void messageApi.error(t("请选择导入文件"));
      return;
    }
    setImportingSQL(true);
    try {
      const raw = await importFile.text();
      validateExchangePayload(raw);
      await importDatabaseSQL(raw);
      const [latestAccounts, latestQueue, latestBackups] = await Promise.all([
        listAccounts(),
        getFailoverQueue(),
        listDatabaseBackups(),
      ]);
      setAccounts(latestAccounts);
      setFailoverQueue(normalizeQueue(latestAccounts, latestQueue));
      setDbBackups(latestBackups);

      let nextSettings = draftSettings;
      try {
        const refreshed = await getAppSettings();
        setDraftSettings(refreshed);
        nextSettings = refreshed;
      } catch {
        // Import payload currently focuses on account-domain tables. Keep existing settings if fetch fails.
      }
      await onSettingsChanged(nextSettings);
      void messageApi.success(t("JSON 导入完成"));
      setImportModalOpen(false);
      setImportFile(null);
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : t("导入 JSON 失败"));
    } finally {
      setImportingSQL(false);
    }
  }

  async function refreshBackups() {
    setDbBackups(await listDatabaseBackups());
  }

  async function handleCreateBackup() {
    setBackupBusy("create");
    try {
      await createDatabaseBackup();
      await refreshBackups();
      void messageApi.success(t("数据库备份已创建"));
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : t("创建数据库备份失败"));
    } finally {
      setBackupBusy("");
    }
  }

  async function handleRestoreBackup(item: DatabaseBackupItem) {
    setOpenBackupMenuID(null);
    setBackupBusy(item.backup_id);
    try {
      await restoreDatabaseBackup(item.backup_id);
      try {
        const refreshed = await getAppSettings();
        setDraftSettings(refreshed);
        await onSettingsChanged(refreshed);
      } catch {
        // Keep current settings draft if the restored snapshot doesn't change app settings.
      }
      await refreshBackups();
      void messageApi.success(t("数据库已恢复到所选备份"));
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : t("恢复数据库备份失败"));
    } finally {
      setBackupBusy("");
    }
  }

  function handleDeleteBackup(item: DatabaseBackupItem) {
    setOpenBackupMenuID(null);
    Modal.confirm({
      title: t("删除备份"),
      content: `${t("确认删除此备份")} ${item.backup_id}？`,
      okText: t("确认删除"),
      cancelText: t("取消"),
      okButtonProps: { danger: true },
      onOk: async () => {
        setBackupBusy(`delete:${item.backup_id}`);
        try {
          await deleteDatabaseBackup(item.backup_id);
          await refreshBackups();
          void messageApi.success(t("数据库备份已删除"));
        } catch (error) {
          void messageApi.error(error instanceof Error ? error.message : t("删除数据库备份失败"));
        } finally {
          setBackupBusy("");
        }
      },
    });
  }

  const tabItems: Array<{ key: SettingsTabKey; label: string; content: ReactNode }> = [
    {
      key: "general",
      label: t("通用"),
      content: (
        <div className="settings-grid">
          <Card className="settings-card settings-card-overflow-visible" variant="borderless">
            <SectionHeader icon={<DesktopOutlined />} title={t("界面偏好")} description={t("即时切换界面语言与主题，自动保存并立即生效。")} />
            <div className="settings-stack">
              <label className="settings-field">
                <span className="settings-field-label">{t("界面语言")}</span>
                <Radio.Group
                  aria-label={t("界面语言")}
                  buttonStyle="solid"
                  optionType="button"
                  options={[
                    { label: "中文", value: "zh-CN" },
                    { label: "English", value: "en-US" },
                  ]}
                  value={draftSettings.language}
                  onChange={(event) => void handleAutoSavePreference({ language: event.target.value })}
                  disabled={autoSavingPreference}
                />
              </label>
              <label className="settings-field">
                <span className="settings-field-label">{language === "en-US" ? "Theme" : "主题模式"}</span>
                <Radio.Group
                  aria-label={language === "en-US" ? "Theme" : "主题模式"}
                  buttonStyle="solid"
                  optionType="button"
                  options={[
                    { label: t("跟随系统"), value: "system" },
                    { label: t("浅色模式"), value: "light" },
                    { label: t("深色模式"), value: "dark" },
                  ]}
                  value={draftSettings.theme_mode}
                  onChange={(event) => void handleAutoSavePreference({ theme_mode: event.target.value })}
                  disabled={autoSavingPreference}
                />
              </label>
              <ToggleRow
                icon={<CloudDownloadOutlined />}
                title={t("首页更新提示")}
                description={t("定时检查 GitHub 新版本，并在首页顶栏显示更新图标提示。")}
                label={t("首页更新提示")}
                checked={draftSettings.show_home_update_indicator}
                onChange={(checked) => updateDraft({ show_home_update_indicator: checked })}
              />
            </div>
          </Card>

          <Card className="settings-card settings-card-overflow-visible" variant="borderless" data-testid="backup-settings-card">
            <SectionHeader icon={<DesktopOutlined />} title={t("窗口行为")} description={t("控制桌面应用的启动与关闭方式。")} />
            <div className="settings-stack">
              <ToggleRow
                icon={<PoweroffOutlined />}
                title={t("开机自启")}
                description={t("通过 macOS LaunchAgent 在登录系统后自动启动 AI Gate。")}
                label={t("开机自启")}
                checked={draftSettings.launch_at_login}
                onChange={(checked) => updateDraft({ launch_at_login: checked })}
              />
              <ToggleRow
                icon={<EyeInvisibleOutlined />}
                title={t("静默启动")}
                description={t("启动应用时不抢占前台窗口，保留托盘常驻。")}
                label={t("静默启动")}
                checked={draftSettings.silent_start}
                onChange={(checked) => updateDraft({ silent_start: checked })}
              />
              <ToggleRow
                icon={<ThunderboltOutlined />}
                title={t("关闭时最小化到托盘")}
                description={t("关闭主窗口时保留后端与托盘继续运行，默认开启。")}
                label={t("关闭时最小化到托盘")}
                checked={draftSettings.close_to_tray}
                onChange={(checked) => updateDraft({ close_to_tray: checked })}
              />
            </div>
          </Card>
        </div>
      ),
    },
    {
      key: "proxy",
      label: t("代理"),
      content: (
        <div className="settings-grid">
          <Card className="settings-card" variant="borderless">
            <SectionHeader icon={<ApiOutlined />} title={t("本地代理")} description={t("管理主界面显示、代理状态与监听地址。")} />
            <div className="proxy-status-strip">
              <Tag color={proxyEnabled ? "success" : "default"} icon={<CheckCircleFilled />}>
                {t("本地代理")} {proxyEnabled ? t("已开启") : t("未开启")}
              </Tag>
              <Text type="secondary">
                {t("当前地址")} {draftSettings.proxy_host}:{draftSettings.proxy_port}
              </Text>
            </div>
            <div className="settings-stack">
              <ToggleRow
                icon={<ControlOutlined />}
                title={t("在主界面显示代理开关")}
                description={t("决定首页右上角是否显示实时代理总开关。")}
                label={t("在主界面显示代理开关")}
                checked={draftSettings.show_proxy_switch_on_home}
                onChange={(checked) => updateDraft({ show_proxy_switch_on_home: checked })}
              />
              <ToggleRow
                icon={<PoweroffOutlined />}
                title={t("代理总开关")}
                description={t("即时启停本地代理，并同步桌面托盘状态。")}
                label={t("代理总开关")}
                checked={proxyEnabled}
                loading={proxySwitchBusy}
                onChange={(checked) => void handleProxyToggle(checked)}
              />
            </div>
            <div className="settings-field-grid">
              <label className="settings-field">
                <span className="settings-field-label">{t("代理主机")}</span>
                <Input
                  aria-label={t("代理主机")}
                  value={draftSettings.proxy_host}
                  onChange={(event) => updateDraft({ proxy_host: event.target.value })}
                  placeholder="127.0.0.1"
                />
              </label>
              <label className="settings-field">
                <span className="settings-field-label">{t("代理端口")}</span>
                <InputNumber
                  aria-label={t("代理端口")}
                  min={1}
                  max={65535}
                  value={draftSettings.proxy_port}
                  onChange={(value) => updateDraft({ proxy_port: Number(value) || 6789 })}
                  className="settings-number"
                />
              </label>
            </div>
          </Card>

          <Card className="settings-card" variant="borderless">
            <SectionHeader icon={<SwapOutlined />} title={t("自动故障转移")} description={t("当当前账号失效时，按队列顺序尝试下一个候选账号。")} />
            <ToggleRow
              icon={<SwapOutlined />}
              title={t("自动故障转移开关")}
              description={t("开启后，网关与 responses 路由会优先使用你指定的显式故障转移队列。")}
              label={t("自动故障转移开关")}
              checked={draftSettings.auto_failover_enabled}
              onChange={(checked) => updateDraft({ auto_failover_enabled: checked })}
            />
            <div className="queue-shell">
              <div className="queue-header">
                <span>{t("自动故障转移队列")}</span>
                <Button onClick={() => void handleSaveQueue()} loading={savingQueue}>
                  {t("保存队列")}
                </Button>
              </div>
              <div className="queue-list">
                {orderedAccounts.length === 0 ? (
                  <div className="settings-empty">{t("暂无可用账号")}</div>
                ) : (
                  orderedAccounts.map((account, index) => (
                    <div className="queue-row" key={account.id}>
                      <div className="queue-row-main">
                        <Avatar>{index + 1}</Avatar>
                        <div>
                          <div className="queue-row-title">{account.account_name}</div>
                          <div className="queue-row-description">
                            {account.provider_type.toUpperCase()} · 优先级 {account.priority}
                          </div>
                        </div>
                      </div>
                      <div className="queue-row-actions">
                        {account.is_active ? <Tag color="success">{t("当前激活")}</Tag> : <Tag>{t("候选")}</Tag>}
                        <Button
                          type="text"
                          icon={<ArrowUpOutlined />}
                          aria-label={`${language === "en-US" ? "Move up" : "上移"} ${account.account_name}`}
                          disabled={index === 0}
                          onClick={() => moveQueueItem(index, -1)}
                        />
                        <Button
                          type="text"
                          icon={<ArrowDownOutlined />}
                          aria-label={`${language === "en-US" ? "Move down" : "下移"} ${account.account_name}`}
                          disabled={index === orderedAccounts.length - 1}
                          onClick={() => moveQueueItem(index, 1)}
                        />
                      </div>
                    </div>
                  ))
                )}
              </div>
            </div>
          </Card>
        </div>
      ),
    },
    {
      key: "advanced",
      label: t("高级"),
      content: (
        <div className="settings-grid">
          <Card className="settings-card" variant="borderless">
            <SectionHeader icon={<DatabaseOutlined />} title={t("数据管理")} description={t("导出与导入均为明文 JSON（仅账户域数据）。")} />
            <div className="settings-action-row">
              <Button icon={<CloudDownloadOutlined />} onClick={() => void handleExportSQL()}>
                {t("导出 JSON")}
              </Button>
              <Button icon={<CloudUploadOutlined />} loading={importingSQL} onClick={() => setImportModalOpen(true)}>
                {t("导入 JSON")}
              </Button>
            </div>
          </Card>

          <Card className="settings-card settings-card-overflow-visible" variant="borderless" data-testid="backup-settings-card">
            <SectionHeader
              icon={<DatabaseOutlined />}
              title={t("备份与恢复")}
              description={t("按设定的时间间隔自动做数据库快照，并控制保留数量。")}
              actions={
                <Button type="primary" onClick={() => void handleCreateBackup()} loading={backupBusy === "create"}>
                  {t("立即备份")}
                </Button>
              }
            />
            <div className="settings-field-grid settings-field-grid-spacious" data-testid="backup-settings-grid">
              <label className="settings-field">
                <span className="settings-field-label">{t("自动备份间隔（小时）")}</span>
                <InputNumber
                  aria-label={t("自动备份间隔（小时）")}
                  min={1}
                  value={draftSettings.auto_backup_interval_hours}
                  onChange={(value) => updateDraft({ auto_backup_interval_hours: Number(value) || 24 })}
                  className="settings-number"
                />
              </label>
              <label className="settings-field">
                <span className="settings-field-label">{t("备份保留数量")}</span>
                <InputNumber
                  aria-label={t("备份保留数量")}
                  min={1}
                  value={draftSettings.backup_retention_count}
                  onChange={(value) => updateDraft({ backup_retention_count: Number(value) || 10 })}
                  className="settings-number"
                />
              </label>
            </div>
            <div className="backup-list" data-testid="backup-list">
              {dbBackups.length === 0 ? (
                <div className="settings-empty">{t("暂无数据库备份")}</div>
              ) : (
                dbBackups.map((item) => (
                  <div className="backup-row" key={item.backup_id}>
                    <div>
                      <div className="backup-row-title">{item.backup_id}</div>
                      <div className="backup-row-description">
                        {t("创建于")} {formatDateTime(item.created_at, language)} · {formatBytes(item.size_bytes)}
                      </div>
                    </div>
                    <div className="backup-row-actions">
                      <div className="backup-menu-shell" data-backup-menu-id={item.backup_id}>
                        <button
                          type="button"
                          className="backup-menu-button"
                          aria-label={`${t("备份操作")} ${item.backup_id}`}
                          aria-haspopup="menu"
                          aria-expanded={openBackupMenuID === item.backup_id}
                          onClick={() => setOpenBackupMenuID((current) => (current === item.backup_id ? null : item.backup_id))}
                          disabled={backupBusy === item.backup_id || backupBusy === `delete:${item.backup_id}`}
                        >
                          <MoreOutlined />
                        </button>
                        {openBackupMenuID === item.backup_id ? (
                          <div className="backup-menu-popover" role="menu" aria-label={`${t("更多操作")} ${item.backup_id}`}>
                            <button
                              type="button"
                              role="menuitem"
                              className="backup-menu-item"
                              disabled={backupBusy === item.backup_id || backupBusy === `delete:${item.backup_id}`}
                              onClick={() => void handleRestoreBackup(item)}
                            >
                              <RollbackOutlined />
                              <span>{t("恢复此备份")}</span>
                            </button>
                            <button
                              type="button"
                              role="menuitem"
                              className="backup-menu-item is-danger"
                              disabled={backupBusy === item.backup_id || backupBusy === `delete:${item.backup_id}`}
                              onClick={() => handleDeleteBackup(item)}
                            >
                              <DeleteOutlined />
                              <span>{t("删除此备份")}</span>
                            </button>
                          </div>
                        ) : null}
                      </div>
                    </div>
                  </div>
                ))
              )}
            </div>
          </Card>

          <Card className="settings-card" variant="borderless">
            <SectionHeader icon={<FileTextOutlined />} title={t("最近日志")} description={t("查看桌面端 sidecar 与自动恢复事件。")} />
            <div className="settings-log-panel" data-testid="settings-recent-logs">
              {recentDesktopLogs.length === 0 ? (
                <div className="settings-empty">{t("暂无桌面日志")}</div>
              ) : (
                recentDesktopLogs.map((entry, index) => (
                  <div className="settings-log-row" key={`${entry.timestamp_ms}-${index}`}>
                    <div className="settings-log-row-main">
                      <div className="settings-log-message">{entry.message}</div>
                      <div className="settings-log-meta">
                        <Tag>{entry.category}</Tag>
                        <Tag color={entry.level === "error" ? "error" : entry.level === "warn" ? "warning" : "default"}>
                          {entry.level}
                        </Tag>
                        <span>{formatDesktopLogTime(entry.timestamp_ms)}</span>
                      </div>
                    </div>
                  </div>
                ))
              )}
            </div>
          </Card>
        </div>
      ),
    },
    {
      key: "about",
      label: t("关于"),
      content: (
        <Card className="settings-card about-card" variant="borderless">
          <div className="about-layout">
            <div className="about-brand">
              <img src={appLogo} alt="AI Gate icon" className="about-logo" />
              <div>
                <Title level={3} style={{ marginBottom: 4 }}>
                  {metadata.name}
                </Title>
                <Text type="secondary">{language === "en-US" ? `Version ${metadata.version}` : `版本 ${metadata.version}`}</Text>
              </div>
            </div>
            <div className="about-copy">
              <SectionHeader icon={<InfoCircleOutlined />} title={t("程序介绍")} description={t(metadata.description)} />
              <div className="about-meta">
                <div className="about-meta-row">
                  <span>{language === "en-US" ? "Author" : "程序作者"}</span>
                  <strong>{metadata.author}</strong>
                </div>
                <div className="about-meta-row">
                  <span>{language === "en-US" ? "Version" : "程序版本"}</span>
                  <strong>{metadata.version}</strong>
                </div>
                <div className="about-meta-row">
                  <span>GitHub</span>
                  <a href="https://github.com/GcsSloop/ai-gate" target="_blank" rel="noreferrer">
                    GcsSloop/ai-gate
                  </a>
                </div>
              </div>
              <UpdateCard currentVersion={metadata.version} language={language} t={t} />
            </div>
          </div>
        </Card>
      ),
    },
  ];

  const activeTabItem = tabItems.find((item) => item.key === activeTab) ?? tabItems[0];

  return (
    <div className="settings-page">
      {contextHolder}
      <div className="settings-tab-toolbar" data-testid="settings-tab-toolbar">
        <div className="menu-pill-group settings-tab-switcher" role="tablist" aria-label={t("设置标签页")}>
          {tabItems.map((item) => (
            <button
              key={item.key}
              type="button"
              role="tab"
              aria-selected={activeTabItem.key === item.key}
              className={activeTabItem.key === item.key ? "menu-pill-button is-active" : "menu-pill-button"}
              onClick={() => setActiveTab(item.key)}
            >
              {item.label}
            </button>
          ))}
        </div>
        <div className="settings-toolbar">
          <Button
            aria-label={t("保存设置")}
            type="primary"
            size="large"
            className="menu-action-button"
            icon={<SaveOutlined />}
            loading={savingSettings}
            onClick={() => void handleSaveSettings()}
          >
            {t("保存设置")}
          </Button>
        </div>
      </div>

      <div className="settings-tab-panel">{activeTabItem.content}</div>

      <Modal
        title={t("导入 JSON")}
        open={importModalOpen}
        okText={t("验证并导入")}
        confirmLoading={importingSQL}
        onOk={() => void handleImportSQL()}
        onCancel={() => {
          if (importingSQL) {
            return;
          }
          setImportModalOpen(false);
          setImportFile(null);
        }}
      >
        <div style={{ display: "grid", gap: 12 }}>
          <Input type="file" accept=".json,application/json,text/plain" onChange={(event) => setImportFile(event.target.files?.[0] || null)} />
        </div>
      </Modal>
    </div>
  );
}
