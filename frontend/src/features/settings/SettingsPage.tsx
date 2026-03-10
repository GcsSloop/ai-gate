import {
  ApiOutlined,
  ArrowDownOutlined,
  ArrowUpOutlined,
  CheckCircleFilled,
  CloudDownloadOutlined,
  CloudUploadOutlined,
  ControlOutlined,
  DatabaseOutlined,
  DesktopOutlined,
  EyeInvisibleOutlined,
  InfoCircleOutlined,
  PoweroffOutlined,
  SaveOutlined,
  SwapOutlined,
  ThunderboltOutlined,
} from "@ant-design/icons";
import { Avatar, Button, Card, Input, InputNumber, Switch, Tabs, Tag, Typography, message } from "antd";
import type { ReactNode } from "react";
import { useEffect, useRef, useState } from "react";

import {
  type AccountRecord,
  type AppSettings,
  type DatabaseBackupItem,
  createDatabaseBackup,
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
import { applyDesktopAppSettings, getAppMetadata, type AppMetadata } from "../../lib/desktop-shell";
import { setAPIBase } from "../../lib/paths";
import appLogo from "../../assets/aigate_1024_1024.png";

const { Text, Title } = Typography;

type SettingsPageProps = {
  initialSettings: AppSettings;
  proxyEnabled: boolean;
  onSettingsChanged: (next: AppSettings) => void | Promise<void>;
  onToggleProxy?: (checked: boolean) => void | Promise<void>;
  hideLocalSaveButton?: boolean;
  onRegisterSaveHandler?: (handler: () => void) => void;
};

function formatDateTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString("zh-CN", { hour12: false });
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

function SectionHeader(props: { icon: ReactNode; title: string; description: string }) {
  return (
    <div className="settings-section-header">
      <div className="settings-section-icon">{props.icon}</div>
      <div>
        <div className="settings-section-title">{props.title}</div>
        <div className="settings-section-description">{props.description}</div>
      </div>
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
      <Switch
        aria-label={props.label}
        checked={props.checked}
        loading={props.loading}
        onChange={props.onChange}
      />
    </div>
  );
}

export function SettingsPage({
  initialSettings,
  proxyEnabled,
  onSettingsChanged,
  onToggleProxy,
  hideLocalSaveButton = false,
  onRegisterSaveHandler,
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
  const [savingSettings, setSavingSettings] = useState(false);
  const [savingQueue, setSavingQueue] = useState(false);
  const [proxySwitchBusy, setProxySwitchBusy] = useState(false);
  const [backupBusy, setBackupBusy] = useState("");
  const [importingSQL, setImportingSQL] = useState(false);
  const sqlInputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    setDraftSettings(initialSettings);
  }, [initialSettings]);

  useEffect(() => {
    async function loadSettingsPageData() {
      try {
        const [accountList, queue, backups, about] = await Promise.all([
          listAccounts(),
          getFailoverQueue(),
          listDatabaseBackups(),
          getAppMetadata(),
        ]);
        setAccounts(accountList);
        setFailoverQueue(normalizeQueue(accountList, queue));
        setDbBackups(backups);
        setMetadata(about);
      } catch (error) {
        void messageApi.error(error instanceof Error ? error.message : "加载设置数据失败");
      }
    }

    void loadSettingsPageData();
  }, [messageApi]);

  const orderedAccounts = failoverQueue
    .map((accountID) => accounts.find((account) => account.id === accountID))
    .filter((account): account is AccountRecord => Boolean(account));

  function updateDraft(patch: Partial<AppSettings>) {
    setDraftSettings((current) => ({
      ...current,
      ...patch,
    }));
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
      void messageApi.success("设置已保存");
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : "保存设置失败");
    } finally {
      setSavingSettings(false);
    }
  }

  async function handleSaveQueue() {
    setSavingQueue(true);
    try {
      await saveFailoverQueue(failoverQueue);
      void messageApi.success("故障转移队列已更新");
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : "保存故障转移队列失败");
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
      const filename = `aigate-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, "-")}.json`;
      triggerTextDownload(filename, raw);
      void messageApi.success("JSON 已导出");
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : "导出 JSON 失败");
    }
  }

  async function handleImportSQL(file: File | null) {
    if (!file) {
      return;
    }
    setImportingSQL(true);
    try {
      const raw = await file.text();
      await importDatabaseSQL(raw);
      try {
        const refreshed = await getAppSettings();
        setDraftSettings(refreshed);
        await onSettingsChanged(refreshed);
      } catch {
        // Keep the current draft when the imported payload doesn't include app settings yet.
      }
      setDbBackups(await listDatabaseBackups());
      void messageApi.success("JSON 导入完成");
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : "导入 JSON 失败");
    } finally {
      setImportingSQL(false);
      if (sqlInputRef.current) {
        sqlInputRef.current.value = "";
      }
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
      void messageApi.success("数据库备份已创建");
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : "创建数据库备份失败");
    } finally {
      setBackupBusy("");
    }
  }

  async function handleRestoreBackup(item: DatabaseBackupItem) {
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
      void messageApi.success("数据库已恢复到所选备份");
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : "恢复数据库备份失败");
    } finally {
      setBackupBusy("");
    }
  }

  useEffect(() => {
    onRegisterSaveHandler?.(() => {
      void handleSaveSettings();
    });
  }, [onRegisterSaveHandler, draftSettings]);

  return (
    <div className="settings-page">
      {contextHolder}
      {!hideLocalSaveButton ? (
        <div className="settings-toolbar">
          <Button
            aria-label="保存设置"
            type="primary"
            size="large"
            icon={<SaveOutlined />}
            loading={savingSettings}
            onClick={() => void handleSaveSettings()}
          >
            保存设置
          </Button>
        </div>
      ) : null}

      <Tabs
        className="settings-tabs"
        items={[
          {
            key: "general",
            label: "通用",
            children: (
              <div className="settings-grid">
                <Card className="settings-card" variant="borderless">
                  <SectionHeader icon={<DesktopOutlined />} title="窗口行为" description="控制桌面应用的启动与关闭方式。" />
                  <div className="settings-stack">
                    <ToggleRow
                      icon={<PoweroffOutlined />}
                      title="开机自启"
                      description="通过 macOS LaunchAgent 在登录系统后自动启动 AI Gate。"
                      label="开机自启"
                      checked={draftSettings.launch_at_login}
                      onChange={(checked) => updateDraft({ launch_at_login: checked })}
                    />
                    <ToggleRow
                      icon={<EyeInvisibleOutlined />}
                      title="静默启动"
                      description="启动应用时不抢占前台窗口，保留托盘常驻。"
                      label="静默启动"
                      checked={draftSettings.silent_start}
                      onChange={(checked) => updateDraft({ silent_start: checked })}
                    />
                    <ToggleRow
                      icon={<ThunderboltOutlined />}
                      title="关闭时最小化到托盘"
                      description="关闭主窗口时保留后端与托盘继续运行，默认开启。"
                      label="关闭时最小化到托盘"
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
            label: "代理",
            children: (
              <div className="settings-grid">
                <Card className="settings-card" variant="borderless">
                  <SectionHeader icon={<ApiOutlined />} title="本地代理" description="管理主界面显示、代理状态与监听地址。" />
                  <div className="proxy-status-strip">
                    <Tag color={proxyEnabled ? "success" : "default"} icon={<CheckCircleFilled />}>
                      本地代理 {proxyEnabled ? "已开启" : "未开启"}
                    </Tag>
                    <Text type="secondary">
                      当前地址 {draftSettings.proxy_host}:{draftSettings.proxy_port}
                    </Text>
                  </div>
                  <div className="settings-stack">
                    <ToggleRow
                      icon={<ControlOutlined />}
                      title="在主界面显示代理开关"
                      description="决定首页右上角是否显示实时代理总开关。"
                      label="在主界面显示代理开关"
                      checked={draftSettings.show_proxy_switch_on_home}
                      onChange={(checked) => updateDraft({ show_proxy_switch_on_home: checked })}
                    />
                    <ToggleRow
                      icon={<PoweroffOutlined />}
                      title="代理总开关"
                      description="即时启停本地代理，并同步桌面托盘状态。"
                      label="代理总开关"
                      checked={proxyEnabled}
                      loading={proxySwitchBusy}
                      onChange={(checked) => void handleProxyToggle(checked)}
                    />
                  </div>
                  <div className="settings-field-grid">
                    <label className="settings-field">
                      <span className="settings-field-label">代理主机</span>
                      <Input
                        aria-label="代理主机"
                        value={draftSettings.proxy_host}
                        onChange={(event) => updateDraft({ proxy_host: event.target.value })}
                        placeholder="127.0.0.1"
                      />
                    </label>
                    <label className="settings-field">
                      <span className="settings-field-label">代理端口</span>
                      <InputNumber
                        aria-label="代理端口"
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
                  <SectionHeader icon={<SwapOutlined />} title="自动故障转移" description="当当前账号失效时，按队列顺序尝试下一个候选账号。" />
                  <ToggleRow
                    icon={<SwapOutlined />}
                    title="自动故障转移开关"
                    description="开启后，网关与 responses 路由会优先使用你指定的显式故障转移队列。"
                    label="自动故障转移开关"
                    checked={draftSettings.auto_failover_enabled}
                    onChange={(checked) => updateDraft({ auto_failover_enabled: checked })}
                  />
                  <div className="queue-shell">
                    <div className="queue-header">
                      <span>自动故障转移队列</span>
                      <Button onClick={() => void handleSaveQueue()} loading={savingQueue}>
                        保存队列
                      </Button>
                    </div>
                    <div className="queue-list">
                      {orderedAccounts.length === 0 ? (
                        <div className="settings-empty">暂无可用账号</div>
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
                              {account.is_active ? <Tag color="success">当前激活</Tag> : <Tag>候选</Tag>}
                              <Button
                                type="text"
                                icon={<ArrowUpOutlined />}
                                aria-label={`上移 ${account.account_name}`}
                                disabled={index === 0}
                                onClick={() => moveQueueItem(index, -1)}
                              />
                              <Button
                                type="text"
                                icon={<ArrowDownOutlined />}
                                aria-label={`下移 ${account.account_name}`}
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
            label: "高级",
            children: (
              <div className="settings-grid">
                <Card className="settings-card" variant="borderless">
                  <SectionHeader icon={<DatabaseOutlined />} title="数据管理" description="直接导出或导入 JSON，以便迁移或排查数据。" />
                  <div className="settings-action-row">
                    <Button icon={<CloudDownloadOutlined />} onClick={() => void handleExportSQL()}>
                      导出 JSON
                    </Button>
                    <Button icon={<CloudUploadOutlined />} loading={importingSQL} onClick={() => sqlInputRef.current?.click()}>
                      导入 JSON
                    </Button>
                    <input
                      ref={sqlInputRef}
                      type="file"
                      accept=".json,application/json,text/plain"
                      style={{ display: "none" }}
                      onChange={(event) => void handleImportSQL(event.target.files?.[0] || null)}
                    />
                  </div>
                </Card>

                <Card className="settings-card" variant="borderless">
                  <SectionHeader icon={<DatabaseOutlined />} title="备份与恢复" description="按设定的时间间隔自动做数据库快照，并控制保留数量。" />
                  <div className="settings-field-grid">
                    <label className="settings-field">
                      <span className="settings-field-label">自动备份间隔（小时）</span>
                      <InputNumber
                        aria-label="自动备份间隔"
                        min={1}
                        value={draftSettings.auto_backup_interval_hours}
                        onChange={(value) => updateDraft({ auto_backup_interval_hours: Number(value) || 24 })}
                        className="settings-number"
                      />
                    </label>
                    <label className="settings-field">
                      <span className="settings-field-label">备份保留数量</span>
                      <InputNumber
                        aria-label="备份保留数量"
                        min={1}
                        value={draftSettings.backup_retention_count}
                        onChange={(value) => updateDraft({ backup_retention_count: Number(value) || 10 })}
                        className="settings-number"
                      />
                    </label>
                  </div>
                  <div className="settings-action-row">
                    <Button type="primary" onClick={() => void handleCreateBackup()} loading={backupBusy === "create"}>
                      立即备份
                    </Button>
                  </div>
                  <div className="backup-list">
                    {dbBackups.length === 0 ? (
                      <div className="settings-empty">暂无数据库备份</div>
                    ) : (
                      dbBackups.map((item) => (
                        <div className="backup-row" key={item.backup_id}>
                          <div>
                            <div className="backup-row-title">{item.backup_id}</div>
                            <div className="backup-row-description">
                              创建于 {formatDateTime(item.created_at)} · {formatBytes(item.size_bytes)}
                            </div>
                          </div>
                          <Button onClick={() => void handleRestoreBackup(item)} loading={backupBusy === item.backup_id}>
                            恢复此备份
                          </Button>
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
            label: "关于",
            children: (
              <Card className="settings-card about-card" variant="borderless">
                <div className="about-layout">
                  <div className="about-brand">
                    <img src={appLogo} alt="AI Gate icon" className="about-logo" />
                    <div>
                      <Title level={3} style={{ marginBottom: 4 }}>
                        {metadata.name}
                      </Title>
                      <Text type="secondary">版本 {metadata.version}</Text>
                    </div>
                  </div>
                  <div className="about-copy">
                    <SectionHeader icon={<InfoCircleOutlined />} title="程序介绍" description={metadata.description} />
                    <div className="about-meta">
                      <div className="about-meta-row">
                        <span>程序作者</span>
                        <strong>{metadata.author}</strong>
                      </div>
                      <div className="about-meta-row">
                        <span>程序版本</span>
                        <strong>{metadata.version}</strong>
                      </div>
                    </div>
                  </div>
                </div>
              </Card>
            ),
          },
        ]}
      />
    </div>
  );
}
