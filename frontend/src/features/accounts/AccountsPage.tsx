import {
  BarChartOutlined,
  CheckCircleOutlined,
  CopyOutlined,
  DeleteOutlined,
  EditOutlined,
  HolderOutlined,
  InfoCircleOutlined,
  PlusOutlined,
} from "@ant-design/icons";
import {
  Avatar,
  Button,
  Card,
  Descriptions,
  Dropdown,
  Empty,
  Form,
  Input,
  Modal,
  Select,
  Skeleton,
  Statistic,
  Tag,
  Typography,
  message,
} from "antd";
import {
  DndContext,
  DragOverlay,
  MouseSensor,
  PointerSensor,
  closestCenter,
  type DragEndEvent,
  type DragOverEvent,
  type DragStartEvent,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import {
  SortableContext,
  arrayMove,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type ButtonHTMLAttributes,
  type CSSProperties,
  type ReactNode,
} from "react";

import {
  createAccount,
  deleteAccount,
  importCurrentCodexAuth,
  fetchPPChatTokenLogs,
  listAccountUsage,
  listAccounts,
  testAccount,
  updateAccount,
  type AccountRecord,
  type PPChatTokenLogsPayload,
  type AccountTestResult,
} from "../../lib/api";
import { refreshDesktopTrayState } from "../../lib/desktop-shell";
import type { AppLanguage, Translator } from "../../lib/i18n";
import sourceClaudeCodeIcon from "../../assets/providers/claude-code.png";
import sourceOpenAIIcon from "../../assets/providers/openai.png";
import sourcePPChatIcon from "../../assets/providers/ppchat.png";

const { Title, Text } = Typography;

const defaultBaseURL = "https://code.ppchat.vip/v1";
type AddModalMode = "official" | "third_party" | null;

const statusColorMap: Record<string, string> = {
  active: "green",
  cooldown: "gold",
  degraded: "orange",
  invalid: "red",
  disabled: "default",
};

const statusTextMap: Record<string, string> = {
  active: "可用",
  cooldown: "冷却中",
  degraded: "降级",
  invalid: "失效",
  disabled: "已停用",
};

const authModeTextMap: Record<string, string> = {
  api_key: "API Key",
  oauth: "官方授权",
  codex_local_import: "本地导入",
};

type SourceIcon = "openai" | "claude_code" | "ppchat";

const sourceIconMap: Record<SourceIcon, { label: string; icon: string }> = {
  openai: { label: "OpenAI", icon: sourceOpenAIIcon },
  claude_code: { label: "Claude Code", icon: sourceClaudeCodeIcon },
  ppchat: { label: "PPChat", icon: sourcePPChatIcon },
};

function normalizeSourceIcon(raw?: string): SourceIcon {
  if (raw === "ppchat") {
    return "ppchat";
  }
  return raw === "claude_code" ? "claude_code" : "openai";
}

function inferSourceIconByBaseURL(baseURL: string): SourceIcon {
  if (/ppchat\.vip/i.test(baseURL)) {
    return "ppchat";
  }
  return "openai";
}

function sameAccountOrder(left: AccountRecord[], right: AccountRecord[]): boolean {
  return left.length === right.length && left.every((item, index) => item.id === right[index]?.id);
}

function formatResetAt(value: string | undefined, language: AppLanguage) {
  if (!value) {
    return "--";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "--";
  }
  const now = new Date();
  const sameDay =
    date.getFullYear() === now.getFullYear() &&
    date.getMonth() === now.getMonth() &&
    date.getDate() === now.getDate();
  if (sameDay) {
    return date.toLocaleTimeString(language, { hour: "2-digit", minute: "2-digit", hour12: false });
  }
  return date.toLocaleDateString(language, { month: "numeric", day: "numeric" });
}

type AccountsPageProps = {
  language?: AppLanguage;
  t?: Translator;
  syncToken?: number;
  addModalMode?: AddModalMode;
  onAddModalModeConsumed?: () => void;
  showAddButton?: boolean;
};

type AccountCardRenderOptions = {
  className?: string;
  actionsDisabled?: boolean;
  cardRef?: (node: HTMLDivElement | null) => void;
  handleAttributes?: Record<string, unknown>;
  handleListeners?: Record<string, unknown>;
  setHandleRef?: (node: HTMLButtonElement | null) => void;
  style?: CSSProperties;
};

type SortableAccountCardProps = {
  id: number;
  record: AccountRecord;
  renderCard: (record: AccountRecord, options?: AccountCardRenderOptions) => ReactNode;
};

function SortableAccountCard({ id, record, renderCard }: SortableAccountCardProps) {
  const { attributes, listeners, setNodeRef, setActivatorNodeRef, transform, transition, isDragging } =
    useSortable({ id });

  const style: CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition,
  };

  return renderCard(record, {
    cardRef: setNodeRef,
    className: isDragging ? "account-card-item-placeholder" : undefined,
    handleAttributes: attributes as Record<string, unknown>,
    handleListeners: listeners as Record<string, unknown>,
    setHandleRef: setActivatorNodeRef,
    style,
  });
}

export function AccountsPage({
  language = "zh-CN",
  t = (value) => value,
  syncToken = 0,
  addModalMode: externalAddModalMode,
  onAddModalModeConsumed,
  showAddButton = true,
}: AccountsPageProps) {
  const [messageApi, contextHolder] = message.useMessage();
  const [accounts, setAccounts] = useState<AccountRecord[]>([]);
  const [internalAddModalMode, setInternalAddModalMode] = useState<AddModalMode>(null);
  const [editingAccount, setEditingAccount] = useState<AccountRecord | null>(null);
  const [detailAccount, setDetailAccount] = useState<AccountRecord | null>(null);
  const [testResult, setTestResult] = useState<AccountTestResult | null>(null);
  const [detailLogsLoading, setDetailLogsLoading] = useState(false);
  const [detailLogs, setDetailLogs] = useState<PPChatTokenLogsPayload["data"] | null>(null);
  const [draggingAccountID, setDraggingAccountID] = useState<number | null>(null);

  const [thirdPartyForm] = Form.useForm();
  const [officialForm] = Form.useForm();
  const [editForm] = Form.useForm();
  const [testForm] = Form.useForm();
  const accountsRef = useRef<AccountRecord[]>([]);
  const dragSnapshotRef = useRef<AccountRecord[] | null>(null);
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: { distance: 4 },
    }),
    useSensor(MouseSensor, {
      activationConstraint: { distance: 4 },
    }),
  );

  function setAccountsState(next: AccountRecord[] | ((items: AccountRecord[]) => AccountRecord[])) {
    setAccounts((items) => {
      const resolved = typeof next === "function" ? next(items) : next;
      accountsRef.current = resolved;
      return resolved;
    });
  }

  useEffect(() => {
    void refreshAll();
  }, [syncToken]);

  useEffect(() => {
    if (!externalAddModalMode) {
      return;
    }
    setInternalAddModalMode(externalAddModalMode);
    onAddModalModeConsumed?.();
  }, [externalAddModalMode, onAddModalModeConsumed]);

  useEffect(() => {
    if (!detailAccount) {
      setDetailLogs(null);
      setDetailLogsLoading(false);
      return;
    }
    if (normalizeSourceIcon(detailAccount.source_icon) !== "ppchat") {
      setDetailLogs(null);
      setDetailLogsLoading(false);
      return;
    }
    let cancelled = false;
    setDetailLogsLoading(true);
    void fetchPPChatTokenLogs(detailAccount.id)
      .then((payload) => {
        if (cancelled) {
          return;
        }
        setDetailLogs(payload.data);
      })
      .catch(() => {
        if (!cancelled) {
          setDetailLogs(null);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setDetailLogsLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [detailAccount]);

  async function refreshAll() {
    const accountItems = await listAccounts();
    accountsRef.current = accountItems;
    setAccounts(accountItems);
    void refreshUsage();
  }

  async function refreshUsage() {
    try {
      const usageItems = await listAccountUsage();
      const usageByAccount = new Map(usageItems.map((item) => [item.account_id, item]));
      setAccountsState((items) =>
        items.map((item) => {
          const usage = usageByAccount.get(item.id);
          if (!usage) {
            return item;
          }
          return {
            ...item,
            ...usage,
          };
        }),
      );
    } catch {
      // Keep base account list responsive even when usage endpoint is unavailable.
    }
  }

  async function handleCreateThirdParty(values: { account_name: string; base_url: string; credential_ref: string }) {
    await createAccount({
      provider_type: "openai-compatible",
      account_name: values.account_name,
      source_icon: inferSourceIconByBaseURL(values.base_url || ""),
      auth_mode: "api_key",
      base_url: values.base_url,
      credential_ref: values.credential_ref,
      supports_responses: true,
    });
    setInternalAddModalMode(null);
    thirdPartyForm.resetFields();
    await refreshAll();
    void refreshDesktopTrayState();
    void messageApi.success(t("第三方账户已添加"));
  }

  async function handleCreateOfficial(values: { account_name: string }) {
    await importCurrentCodexAuth(values.account_name || "local-codex");
    officialForm.resetFields();
    setInternalAddModalMode(null);
    await refreshAll();
    void refreshDesktopTrayState();
    void messageApi.success(t("官方账户已导入"));
  }

  function openEditModal(account: AccountRecord) {
    setEditingAccount(account);
    setTestResult(null);
    editForm.setFieldsValue({
      account_name: account.account_name,
      source_icon: normalizeSourceIcon(account.source_icon),
      base_url: account.base_url,
      credential_ref: "",
    });
    testForm.setFieldsValue({
      model: getDefaultTestModel(account),
      input: "ping",
    });
  }

  async function handleEdit(values: {
    account_name: string;
    source_icon: SourceIcon;
    base_url: string;
    credential_ref?: string;
  }) {
    if (!editingAccount) {
      return;
    }
    await updateAccount(editingAccount.id, {
      account_name: values.account_name,
      source_icon: normalizeSourceIcon(values.source_icon),
      base_url: values.base_url,
      credential_ref: values.credential_ref || undefined,
      supports_responses: true,
    });
    setEditingAccount(null);
    editForm.resetFields();
    await refreshAll();
    void refreshDesktopTrayState();
    void messageApi.success(t("账户已更新"));
  }

  async function handleDelete(account: AccountRecord) {
    await deleteAccount(account.id);
    await refreshAll();
    void refreshDesktopTrayState();
    void messageApi.success(`已删除账户 ${account.account_name}`);
  }

  async function handleTest(values: { model: string; input: string }) {
    if (!editingAccount) {
      return;
    }
    const result = await testAccount(editingAccount.id, values);
    setTestResult(result);
  }

  async function handleSetActive(account: AccountRecord) {
    if (account.is_active) {
      return;
    }
    const previous = [...accounts];
    setAccountsState((items) =>
      items.map((item) => ({
        ...item,
        is_active: item.id === account.id,
      })),
    );
    try {
      await updateAccount(account.id, { is_active: true });
      void refreshDesktopTrayState();
      void messageApi.success(
        language === "en-US" ? `Active account switched to ${account.account_name}` : `已切换当前激活账户为 ${account.account_name}`,
      );
    } catch (error) {
      setAccountsState(previous);
      void messageApi.error(error instanceof Error ? error.message : t("切换激活账户失败"));
    }
  }

  async function handleCopyAccount(record: AccountRecord) {
    const copied = `${record.account_name}\n${record.base_url || t("OpenAI 官方")}`;
    try {
      await navigator.clipboard.writeText(copied);
      void messageApi.success(t("已复制账户名称和 API 地址"));
    } catch {
      void messageApi.warning(t("复制失败，请检查系统剪贴板权限"));
    }
  }

  function handleDragStart(event: DragStartEvent) {
    const activeID = Number(event.active.id);
    dragSnapshotRef.current = accountsRef.current;
    setDraggingAccountID(activeID);
  }

  function handleDragOver(event: DragOverEvent) {
    const overID = event.over ? Number(event.over.id) : null;
    if (overID === null) {
      return;
    }

    setAccountsState((items) => {
      const activeIndex = items.findIndex((item) => item.id === Number(event.active.id));
      const overIndex = items.findIndex((item) => item.id === overID);
      if (activeIndex < 0 || overIndex < 0 || activeIndex === overIndex) {
        return items;
      }
      return arrayMove(items, activeIndex, overIndex);
    });
  }

  async function finishDragSort() {
    const snapshot = dragSnapshotRef.current;
    const current = accountsRef.current;
    dragSnapshotRef.current = null;
    setDraggingAccountID(null);

    if (!snapshot || sameAccountOrder(snapshot, current)) {
      return;
    }

    try {
      await persistAccountPriority(current);
    } catch {
      setAccountsState(snapshot);
      void messageApi.warning(t("排序已更新到界面，但保存顺序失败，请稍后重试"));
    }
  }

  async function handleDragEnd(event: DragEndEvent) {
    if (!event.over) {
      if (dragSnapshotRef.current) {
        setAccountsState(dragSnapshotRef.current);
      }
      dragSnapshotRef.current = null;
      setDraggingAccountID(null);
      return;
    }
    await finishDragSort();
  }

  function handleDragCancel() {
    if (dragSnapshotRef.current) {
      setAccountsState(dragSnapshotRef.current);
    }
    dragSnapshotRef.current = null;
    setDraggingAccountID(null);
  }

  async function persistAccountPriority(items: AccountRecord[]) {
    for (let index = 0; index < items.length; index += 1) {
      const item = items[index];
      const priority = items.length - index;
      let attempt = 0;
      let saved = false;
      while (attempt < 3 && !saved) {
        try {
          await updateAccount(item.id, { priority });
          saved = true;
        } catch (error) {
          attempt += 1;
          if (attempt >= 3) {
            throw error;
          }
          await sleep(120 * attempt);
        }
      }
    }
  }

  const detailLogMaxTokens = useMemo(() => {
    if (!detailLogs?.logs?.length) {
      return 0;
    }
    return Math.max(...detailLogs.logs.map((log) => log.prompt_tokens + log.completion_tokens), 1);
  }, [detailLogs]);

  const ppchatSummaryCards = useMemo(() => {
    const info = detailLogs?.token_info;
    if (!info) {
      return [];
    }
    const expiryTime = info.expiry?.time || info.expired_time_formatted || "-";
    return [
      { key: "token", title: "TOKEN 名称", value: info.name || "-", valueClass: "is-text" },
      { key: "plan", title: "套餐类型", value: inferPPChatPackageType(info.name || ""), valueClass: "is-text" },
      { key: "expiry", title: "到期时间", value: expiryTime, valueClass: "is-datetime" },
      { key: "remain", title: "剩余配额", value: `${info.remain_quota_display ?? 0}` },
      { key: "used", title: "当天已用配额", value: `${info.today_used_quota ?? 0}` },
      { key: "added", title: "当天增加配额", value: `${info.today_added_quota ?? 0}` },
      { key: "count", title: "今日已用次数", value: `${info.today_usage_count ?? 0}` },
      { key: "opus", title: "今日 OPUS 使用次数", value: `${info.today_opus_usage ?? 0}` },
      { key: "big", title: "今日大TOKEN请求数", value: `${info.today_big_token_requests ?? 0}` },
    ];
  }, [detailLogs]);

  const draggingAccount =
    draggingAccountID === null
      ? null
      : accounts.find((item) => item.id === draggingAccountID) ??
        dragSnapshotRef.current?.find((item) => item.id === draggingAccountID) ??
        null;

  function renderAccountCard(record: AccountRecord, options: AccountCardRenderOptions = {}) {
    const sourceIcon = sourceIconMap[normalizeSourceIcon(record.source_icon)];

    return (
      <div
        ref={options.cardRef}
        className={`account-card-item ${record.is_active ? "active-account-card" : ""} ${options.className ?? ""}`.trim()}
        style={options.style}
      >
        <Card variant="borderless" className="account-card-surface">
          <div className="account-card-shell">
            <button
              type="button"
              ref={options.setHandleRef}
              className="account-drag-handle"
              aria-label={`${language === "en-US" ? "Drag sort" : "拖拽排序"}-${record.account_name}`}
              {...(options.handleAttributes as ButtonHTMLAttributes<HTMLButtonElement> | undefined)}
              {...(options.handleListeners as ButtonHTMLAttributes<HTMLButtonElement> | undefined)}
            >
              <HolderOutlined />
            </button>
            <div className="account-main">
              <Avatar src={sourceIcon.icon} size={36} shape="square" className="account-source-icon" />
              <div className="account-main-text">
                <div className="account-title-row">
                  <Text strong>{record.account_name}</Text>
                  <Tag color={statusColorMap[record.status] ?? "default"}>
                    {t(statusTextMap[record.status] ?? record.status)}
                  </Tag>
                  {record.is_active ? <Tag color="green">{t("当前激活")}</Tag> : null}
                </div>
                <Text type="secondary" className="account-base-url">
                  {record.base_url || t("OpenAI 官方")}
                </Text>
              </div>
            </div>
            <div className="account-actions">
              <Button
                type="primary"
                className="account-enable-button"
                aria-label={`${language === "en-US" ? "Set active" : "设为激活"}-${record.account_name}`}
                icon={<CheckCircleOutlined />}
                disabled={record.is_active || options.actionsDisabled}
                onClick={() => void handleSetActive(record)}
              >
                {t("启用")}
              </Button>
              <Button
                type="text"
                className="account-action-button"
                aria-label={`${language === "en-US" ? "Edit" : "编辑"}-${record.account_name}`}
                icon={<EditOutlined />}
                disabled={options.actionsDisabled}
                onClick={() => openEditModal(record)}
              />
              <Button
                type="text"
                className="account-action-button"
                aria-label={`${language === "en-US" ? "Copy" : "复制"}-${record.account_name}`}
                icon={<CopyOutlined />}
                disabled={options.actionsDisabled}
                onClick={() => void handleCopyAccount(record)}
              />
              <Button
                type="text"
                className="account-action-button"
                aria-label={`${language === "en-US" ? "Details" : "详情"}-${record.account_name}`}
                icon={<InfoCircleOutlined />}
                disabled={options.actionsDisabled}
                onClick={() => setDetailAccount(record)}
              />
              <Button
                type="text"
                danger
                className="account-action-button"
                aria-label={`${language === "en-US" ? "Delete" : "删除"}-${record.account_name}`}
                icon={<DeleteOutlined />}
                disabled={options.actionsDisabled}
                onClick={() =>
                  void Modal.confirm({
                    title: language === "en-US" ? `Delete account "${record.account_name}"?` : `确认删除账户「${record.account_name}」吗？`,
                    okText: t("删除"),
                    cancelText: t("取消"),
                    okButtonProps: { danger: true },
                    onOk: () => handleDelete(record),
                  })
                }
              />
            </div>
          </div>
        </Card>
      </div>
    );
  }

  return (
    <div className="dashboard-page">
      {contextHolder}
      {showAddButton ? (
        <div className="dashboard-header">
          <div>
            <Title level={2} style={{ marginBottom: 8 }}>
              {t("账户列表")}
            </Title>
            <Text type="secondary">{t("主表仅展示核心状态，详细信息请通过详情查看。")}</Text>
          </div>
          <Dropdown
            menu={{
              items: [
                { key: "official", label: t("官方账户") },
                { key: "third_party", label: t("第三方账户") },
              ],
              onClick: ({ key }) => setInternalAddModalMode(key as AddModalMode),
            }}
            trigger={["click"]}
          >
            <Button type="primary" icon={<PlusOutlined />}>
              {t("添加账户")}
            </Button>
          </Dropdown>
        </div>
      ) : null}

      {accounts.length === 0 ? (
        <Card className="accounts-card" variant="borderless">
          <div className="accounts-empty">
            <Empty description={t("暂无账户")} />
          </div>
        </Card>
      ) : (
        <DndContext
          sensors={sensors}
          collisionDetection={closestCenter}
          onDragStart={handleDragStart}
          onDragOver={handleDragOver}
          onDragEnd={(event) => void handleDragEnd(event)}
          onDragCancel={handleDragCancel}
        >
          <SortableContext items={accounts.map((record) => record.id)} strategy={verticalListSortingStrategy}>
            <div className="account-cards">
              {accounts.map((record) => (
                <SortableAccountCard
                  key={record.id}
                  id={record.id}
                  record={record}
                  renderCard={renderAccountCard}
                />
              ))}
            </div>
          </SortableContext>
          <DragOverlay>
            {draggingAccount ? (
              <div className="account-drag-overlay">{renderAccountCard(draggingAccount, { actionsDisabled: true })}</div>
            ) : null}
          </DragOverlay>
        </DndContext>
      )}

      <Modal open={!!detailAccount} title={t("账户详情")} onCancel={() => setDetailAccount(null)} footer={null} destroyOnHidden width={880}>
        {detailAccount ? (
          normalizeSourceIcon(detailAccount.source_icon) === "ppchat" ? (
            <div className="account-detail-layout">
              {detailLogsLoading ? (
                <Card variant="borderless" className="account-detail-chart-card">
                  <Skeleton active paragraph={{ rows: 8 }} />
                </Card>
              ) : (
                <div className="ppchat-metrics-grid">
                  {ppchatSummaryCards.map((item) => (
                    <Card key={item.key} variant="borderless" className="ppchat-metric-card">
                      <Statistic
                        title={<span className="ppchat-stat-title">{item.title}</span>}
                        value={item.value}
                        valueRender={(node) => <span className={`ppchat-stat-value ${item.valueClass ?? ""}`}>{node}</span>}
                      />
                    </Card>
                  ))}
                </div>
              )}
              <Card variant="borderless" className="account-detail-chart-card" title={t("PPChat Token 日志")} extra={<BarChartOutlined />}>
                {detailLogsLoading ? (
                  <Skeleton active paragraph={{ rows: 5 }} />
                ) : detailLogs?.logs?.length ? (
                  <div className="token-log-list">
                    {detailLogs.logs.slice(0, 8).map((log, index) => {
                      const total = log.prompt_tokens + log.completion_tokens;
                      const width = detailLogMaxTokens > 0 ? Math.max((total / detailLogMaxTokens) * 100, 6) : 0;
                      return (
                        <div className="token-log-row" key={`${log.created_at}-${index}`}>
                          <div className="token-log-meta">
                            <span>{log.model_name}</span>
                            <span>{log.created_time}</span>
                          </div>
                          <div className="token-log-bar-bg">
                            <div className="token-log-bar" style={{ width: `${width}%` }} />
                          </div>
                          <div className="token-log-values">
                            <span>Prompt {log.prompt_tokens}</span>
                            <span>Completion {log.completion_tokens}</span>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                ) : (
                  <Empty description={t("暂无 PPChat 日志数据")} image={Empty.PRESENTED_IMAGE_SIMPLE} />
                )}
              </Card>
            </div>
          ) : (
            <div className="account-detail-layout">
              <div className="account-detail-stats">
                <Card variant="borderless">
                  <Statistic title={t("额度余额")} value={Math.round(detailAccount.quota_remaining)} />
                </Card>
                <Card variant="borderless">
                  <Statistic title={t("健康分")} value={detailAccount.health_score.toFixed(2)} />
                </Card>
                <Card variant="borderless">
                  <Statistic title={t("最近 Token")} value={Math.round(detailAccount.last_total_tokens)} />
                </Card>
                <Card variant="borderless">
                  <Statistic title={t("错误率")} value={(detailAccount.recent_error_rate * 100).toFixed(1)} suffix="%" />
                </Card>
              </div>
              <Card variant="borderless" className="account-detail-meta">
                <Descriptions column={2} size="small">
                  <Descriptions.Item label={t("账户")}>{detailAccount.account_name}</Descriptions.Item>
                  <Descriptions.Item label={t("来源")}>{sourceIconMap[normalizeSourceIcon(detailAccount.source_icon)].label}</Descriptions.Item>
                  <Descriptions.Item label={t("状态")}>{t(statusTextMap[detailAccount.status] ?? detailAccount.status)}</Descriptions.Item>
                  <Descriptions.Item label={t("认证方式")}>{t(authModeTextMap[detailAccount.auth_mode] ?? detailAccount.auth_mode)}</Descriptions.Item>
                  <Descriptions.Item label={t("接口地址")} span={2}>
                    {detailAccount.base_url || t("OpenAI 官方")}
                  </Descriptions.Item>
                  <Descriptions.Item label={t("5 小时剩余")}>
                    {(100 - detailAccount.primary_used_percent).toFixed(0)}% · {formatResetAt(detailAccount.primary_resets_at, language)}
                  </Descriptions.Item>
                  <Descriptions.Item label={t("1 周剩余")}>
                    {(100 - detailAccount.secondary_used_percent).toFixed(0)}% · {formatResetAt(detailAccount.secondary_resets_at, language)}
                  </Descriptions.Item>
                </Descriptions>
              </Card>
            </div>
          )
        ) : null}
      </Modal>

      <Modal
        open={internalAddModalMode === "third_party"}
        title={t("添加第三方账户")}
        onCancel={() => setInternalAddModalMode(null)}
        footer={null}
        destroyOnHidden
      >
        <Form
          form={thirdPartyForm}
          layout="vertical"
          initialValues={{ base_url: defaultBaseURL }}
          onFinish={(values) => void handleCreateThirdParty(values)}
        >
          <Form.Item label={t("账户名称")} name="account_name" rules={[{ required: true, message: t("请输入账户名称") }]}>
            <Input />
          </Form.Item>
          <Form.Item label={t("接口地址")} name="base_url" rules={[{ required: true, message: t("请输入接口地址") }]}>
            <Input />
          </Form.Item>
          <Form.Item label="API Key" name="credential_ref" rules={[{ required: true, message: t("请输入 API Key") }]}>
            <Input.Password />
          </Form.Item>
          <div className="modal-footer">
            <Button onClick={() => setInternalAddModalMode(null)}>{t("取消")}</Button>
            <Button type="primary" htmlType="submit">
              {t("保存")}
            </Button>
          </div>
        </Form>
      </Modal>

      <Modal
        open={internalAddModalMode === "official"}
        title={t("添加官方账户")}
        onCancel={() => setInternalAddModalMode(null)}
        footer={null}
        destroyOnHidden
      >
        <Form form={officialForm} layout="vertical" onFinish={(values) => void handleCreateOfficial(values)}>
          <Form.Item label={t("账户名称")} name="account_name" initialValue="local-codex">
            <Input />
          </Form.Item>
          <Text type="secondary">
            {language === "en-US"
              ? <>The app reads <code>~/.codex/auth.json</code> directly from the current user directory.</>
              : <>将直接读取当前用户目录下的 <code>~/.codex/auth.json</code>。</>}
          </Text>
          <div className="modal-footer">
            <Button onClick={() => setInternalAddModalMode(null)}>{t("取消")}</Button>
            <Button type="primary" htmlType="submit">
              {t("导入")}
            </Button>
          </div>
        </Form>
      </Modal>

      <Modal
        open={!!editingAccount}
        title={t("编辑账户")}
        onCancel={() => setEditingAccount(null)}
        footer={null}
        destroyOnHidden
      >
        <Form form={editForm} layout="vertical" onFinish={(values) => void handleEdit(values)}>
          <Form.Item label={t("账户名称")} name="account_name" rules={[{ required: true, message: t("请输入账户名称") }]}>
            <Input />
          </Form.Item>
          <Form.Item label={t("来源图标")} name="source_icon" rules={[{ required: true, message: t("请选择来源图标") }]}>
            <Select
              options={(Object.keys(sourceIconMap) as SourceIcon[]).map((key) => ({
                value: key,
                label: (
                  <span className="source-option">
                    <Avatar src={sourceIconMap[key].icon} size={16} shape="square" />
                    <span>{sourceIconMap[key].label}</span>
                  </span>
                ),
              }))}
            />
          </Form.Item>
          <Form.Item label={t("接口地址")} name="base_url" rules={[{ required: true, message: t("请输入接口地址") }]}>
            <Input />
          </Form.Item>
          <Form.Item label={t("API Key / Token")} name="credential_ref">
            <Input.Password placeholder={t("留空表示不修改")} />
          </Form.Item>
          <div className="modal-footer">
            <Button onClick={() => setEditingAccount(null)}>{t("取消")}</Button>
            <Button type="primary" htmlType="submit">
              {t("保存")}
            </Button>
          </div>
        </Form>
        <div className="edit-test-panel">
          <Text strong>{t("连接测试")}</Text>
          <Form form={testForm} layout="vertical" initialValues={{ model: "gpt-5.4", input: "ping" }} onFinish={(values) => void handleTest(values)}>
            <Form.Item label={t("模型")} name="model" rules={[{ required: true, message: t("请选择模型") }]}>
              <Select
                options={[
                  { value: "gpt-5.4", label: "gpt-5.4" },
                  { value: "gpt-5.1-codex-max", label: "gpt-5.1-codex-max" },
                  { value: "gpt-5.2-codex", label: "gpt-5.2-codex" },
                  { value: "gpt-5", label: "gpt-5" },
                  { value: "gpt-4.1", label: "gpt-4.1" },
                ]}
              />
            </Form.Item>
            <Form.Item label={t("输入内容")} name="input" rules={[{ required: true, message: t("请输入测试内容") }]}>
              <Input.TextArea rows={3} />
            </Form.Item>
            <div className="modal-footer">
              <Button htmlType="submit">{t("测试")}</Button>
            </div>
          </Form>
        </div>
        {testResult ? (
          <div className="test-result-panel">
            <Tag color={testResult.ok ? "green" : "red"}>{testResult.message}</Tag>
            {testResult.details ? <Text type="secondary">{testResult.details}</Text> : null}
            {testResult.content ? <pre className="test-output">{testResult.content}</pre> : null}
          </div>
        ) : null}
      </Modal>
    </div>
  );
}

function getDefaultTestModel(account: AccountRecord): string {
  if (account.auth_mode === "codex_local_import" || account.provider_type === "openai-official") {
    return "gpt-5.4";
  }
  return "gpt-5.4";
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => {
    window.setTimeout(resolve, ms);
  });
}

function inferPPChatPackageType(tokenName: string): string {
  const normalized = tokenName.trim();
  if (!normalized) {
    return "-";
  }
  const parts = normalized.split("-");
  const suffix = parts[parts.length - 1] || normalized;
  return suffix.toUpperCase();
}
