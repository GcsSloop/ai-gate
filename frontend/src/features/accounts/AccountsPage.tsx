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
  Space,
  Statistic,
  Switch,
  Tag,
  Typography,
  message,
} from "antd";
import { useEffect, useMemo, useState } from "react";

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

function formatResetAt(value?: string) {
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
    return date.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit", hour12: false });
  }
  return date.toLocaleDateString("zh-CN", { month: "numeric", day: "numeric" });
}

type AccountsPageProps = {
  syncToken?: number;
  addModalMode?: AddModalMode;
  onAddModalModeConsumed?: () => void;
  showAddButton?: boolean;
};

export function AccountsPage({
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
  const [dragOverAccountID, setDragOverAccountID] = useState<number | null>(null);

  const [thirdPartyForm] = Form.useForm();
  const [officialForm] = Form.useForm();
  const [editForm] = Form.useForm();
  const [testForm] = Form.useForm();
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
    setAccounts(accountItems);
    void refreshUsage();
  }

  async function refreshUsage() {
    try {
      const usageItems = await listAccountUsage();
      const usageByAccount = new Map(usageItems.map((item) => [item.account_id, item]));
      setAccounts((items) =>
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

  async function handleCreateThirdParty(values: { account_name: string; base_url: string; credential_ref: string; supports_responses?: boolean }) {
    await createAccount({
      provider_type: "openai-compatible",
      account_name: values.account_name,
      source_icon: inferSourceIconByBaseURL(values.base_url || ""),
      auth_mode: "api_key",
      base_url: values.base_url,
      credential_ref: values.credential_ref,
      supports_responses: !!values.supports_responses,
    });
    setInternalAddModalMode(null);
    thirdPartyForm.resetFields();
    await refreshAll();
    void refreshDesktopTrayState();
    void messageApi.success("第三方账户已添加");
  }

  async function handleCreateOfficial(values: { account_name: string }) {
    await importCurrentCodexAuth(values.account_name || "local-codex");
    officialForm.resetFields();
    setInternalAddModalMode(null);
    await refreshAll();
    void refreshDesktopTrayState();
    void messageApi.success("官方账户已导入");
  }

  function openEditModal(account: AccountRecord) {
    setEditingAccount(account);
    setTestResult(null);
    editForm.setFieldsValue({
      account_name: account.account_name,
      source_icon: normalizeSourceIcon(account.source_icon),
      base_url: account.base_url,
      credential_ref: "",
      supports_responses: !!account.supports_responses,
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
    supports_responses?: boolean;
  }) {
    if (!editingAccount) {
      return;
    }
    await updateAccount(editingAccount.id, {
      account_name: values.account_name,
      source_icon: normalizeSourceIcon(values.source_icon),
      base_url: values.base_url,
      credential_ref: values.credential_ref || undefined,
      supports_responses: !!values.supports_responses,
    });
    setEditingAccount(null);
    editForm.resetFields();
    await refreshAll();
    void refreshDesktopTrayState();
    void messageApi.success("账户已更新");
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
    setAccounts((items) =>
      items.map((item) => ({
        ...item,
        is_active: item.id === account.id,
      })),
    );
    try {
      await updateAccount(account.id, { is_active: true });
      void refreshDesktopTrayState();
      void messageApi.success(`已切换当前激活账户为 ${account.account_name}`);
    } catch (error) {
      setAccounts(previous);
      void messageApi.error(error instanceof Error ? error.message : "切换激活账户失败");
    }
  }

  async function handleCopyAccount(record: AccountRecord) {
    const copied = `${record.account_name}\n${record.base_url || "OpenAI 官方"}`;
    try {
      await navigator.clipboard.writeText(copied);
      void messageApi.success("已复制账户名称和 API 地址");
    } catch {
      void messageApi.warning("复制失败，请检查系统剪贴板权限");
    }
  }

  function moveAccountCard(fromID: number, toID: number) {
    if (fromID === toID) {
      return;
    }
    setAccounts((items) => {
      const fromIndex = items.findIndex((item) => item.id === fromID);
      const toIndex = items.findIndex((item) => item.id === toID);
      if (fromIndex < 0 || toIndex < 0) {
        return items;
      }
      const next = [...items];
      const [moved] = next.splice(fromIndex, 1);
      next.splice(toIndex, 0, moved);
      void persistAccountPriority(next);
      return next;
    });
  }

  async function persistAccountPriority(items: AccountRecord[]) {
    try {
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
    } catch {
      void messageApi.warning("排序已更新到界面，但保存顺序失败，请稍后重试");
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

  return (
    <div className="dashboard-page">
      {contextHolder}
      {showAddButton ? (
        <div className="dashboard-header">
          <div>
            <Title level={2} style={{ marginBottom: 8 }}>
              账户列表
            </Title>
            <Text type="secondary">主表仅展示核心状态，详细信息请通过详情查看。</Text>
          </div>
          <Dropdown
            menu={{
              items: [
                { key: "official", label: "官方账户" },
                { key: "third_party", label: "第三方账户" },
              ],
              onClick: ({ key }) => setInternalAddModalMode(key as AddModalMode),
            }}
            trigger={["click"]}
          >
            <Button type="primary" icon={<PlusOutlined />}>
              添加账户
            </Button>
          </Dropdown>
        </div>
      ) : null}

      {accounts.length === 0 ? (
        <Card className="accounts-card" variant="borderless">
          <div className="accounts-empty">
            <Empty description="暂无账户" />
          </div>
        </Card>
      ) : (
        <div className="account-cards">
          {accounts.map((record) => {
            const sourceIcon = sourceIconMap[normalizeSourceIcon(record.source_icon)];
            return (
              <Card
                key={record.id}
                className={`account-card-item ${record.is_active ? "active-account-card" : ""} ${
                  dragOverAccountID === record.id ? "drag-over-account-card" : ""
                }`}
                variant="borderless"
                draggable
                onDragStart={() => setDraggingAccountID(record.id)}
                onDragOver={(event) => {
                  event.preventDefault();
                  if (draggingAccountID && draggingAccountID !== record.id) {
                    setDragOverAccountID(record.id);
                  }
                }}
                onDrop={(event) => {
                  event.preventDefault();
                  if (draggingAccountID) {
                    moveAccountCard(draggingAccountID, record.id);
                  }
                  setDraggingAccountID(null);
                  setDragOverAccountID(null);
                }}
                onDragEnd={() => {
                  setDraggingAccountID(null);
                  setDragOverAccountID(null);
                }}
              >
                <div className="account-card-shell">
                  <Button type="text" icon={<HolderOutlined />} className="account-drag-handle" aria-label={`拖拽排序-${record.account_name}`} />
                  <div className="account-main">
                    <Avatar src={sourceIcon.icon} size={42} shape="square" className="account-source-icon" />
                    <div className="account-main-text">
                      <div className="account-title-row">
                        <Text strong>{record.account_name}</Text>
                        <Tag color={statusColorMap[record.status] ?? "default"}>{statusTextMap[record.status] ?? record.status}</Tag>
                        {record.is_active ? <Tag color="green">当前激活</Tag> : null}
                      </div>
                      <Text type="secondary" className="account-base-url">
                        {record.base_url || "OpenAI 官方"}
                      </Text>
                    </div>
                  </div>
                  <div className="account-actions">
                    <Button
                      type="primary"
                      className="account-enable-button"
                      aria-label={`设为激活-${record.account_name}`}
                      icon={<CheckCircleOutlined />}
                      disabled={record.is_active}
                      onClick={() => void handleSetActive(record)}
                    >
                      启用
                    </Button>
                    <Button type="text" className="account-action-button" aria-label={`编辑-${record.account_name}`} icon={<EditOutlined />} onClick={() => openEditModal(record)} />
                    <Button
                      type="text"
                      className="account-action-button"
                      aria-label={`复制-${record.account_name}`}
                      icon={<CopyOutlined />}
                      onClick={() => void handleCopyAccount(record)}
                    />
                    <Button
                      type="text"
                      className="account-action-button"
                      aria-label={`详情-${record.account_name}`}
                      icon={<InfoCircleOutlined />}
                      onClick={() => setDetailAccount(record)}
                    />
                    <Button
                      type="text"
                      danger
                      className="account-action-button"
                      aria-label={`删除-${record.account_name}`}
                      icon={<DeleteOutlined />}
                      onClick={() =>
                        void Modal.confirm({
                          title: `确认删除账户「${record.account_name}」吗？`,
                          okText: "删除",
                          cancelText: "取消",
                          okButtonProps: { danger: true },
                          onOk: () => handleDelete(record),
                        })
                      }
                    />
                  </div>
                </div>
              </Card>
            );
          })}
        </div>
      )}

      <Modal open={!!detailAccount} title="账户详情" onCancel={() => setDetailAccount(null)} footer={null} destroyOnHidden width={880}>
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
              <Card variant="borderless" className="account-detail-chart-card" title="PPChat Token 日志" extra={<BarChartOutlined />}>
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
                  <Empty description="暂无 PPChat 日志数据" image={Empty.PRESENTED_IMAGE_SIMPLE} />
                )}
              </Card>
            </div>
          ) : (
            <div className="account-detail-layout">
              <div className="account-detail-stats">
                <Card variant="borderless">
                  <Statistic title="额度余额" value={Math.round(detailAccount.quota_remaining)} />
                </Card>
                <Card variant="borderless">
                  <Statistic title="健康分" value={detailAccount.health_score.toFixed(2)} />
                </Card>
                <Card variant="borderless">
                  <Statistic title="最近 Token" value={Math.round(detailAccount.last_total_tokens)} />
                </Card>
                <Card variant="borderless">
                  <Statistic title="错误率" value={(detailAccount.recent_error_rate * 100).toFixed(1)} suffix="%" />
                </Card>
              </div>
              <Card variant="borderless" className="account-detail-meta">
                <Descriptions column={2} size="small">
                  <Descriptions.Item label="账户">{detailAccount.account_name}</Descriptions.Item>
                  <Descriptions.Item label="来源">{sourceIconMap[normalizeSourceIcon(detailAccount.source_icon)].label}</Descriptions.Item>
                  <Descriptions.Item label="状态">{statusTextMap[detailAccount.status] ?? detailAccount.status}</Descriptions.Item>
                  <Descriptions.Item label="认证方式">{authModeTextMap[detailAccount.auth_mode] ?? detailAccount.auth_mode}</Descriptions.Item>
                  <Descriptions.Item label="接口地址" span={2}>
                    {detailAccount.base_url || "OpenAI 官方"}
                  </Descriptions.Item>
                  <Descriptions.Item label="5 小时剩余">
                    {(100 - detailAccount.primary_used_percent).toFixed(0)}% · {formatResetAt(detailAccount.primary_resets_at)}
                  </Descriptions.Item>
                  <Descriptions.Item label="1 周剩余">
                    {(100 - detailAccount.secondary_used_percent).toFixed(0)}% · {formatResetAt(detailAccount.secondary_resets_at)}
                  </Descriptions.Item>
                </Descriptions>
              </Card>
            </div>
          )
        ) : null}
      </Modal>

      <Modal
        open={internalAddModalMode === "third_party"}
        title="添加第三方账户"
        onCancel={() => setInternalAddModalMode(null)}
        footer={null}
        destroyOnHidden
      >
        <Form
          form={thirdPartyForm}
          layout="vertical"
          initialValues={{ base_url: defaultBaseURL, supports_responses: true }}
          onFinish={(values) => void handleCreateThirdParty(values)}
        >
          <Form.Item label="账户名称" name="account_name" rules={[{ required: true, message: "请输入账户名称" }]}>
            <Input />
          </Form.Item>
          <Form.Item label="接口地址" name="base_url" rules={[{ required: true, message: "请输入接口地址" }]}>
            <Input />
          </Form.Item>
          <Form.Item label="API Key" name="credential_ref" rules={[{ required: true, message: "请输入 API Key" }]}>
            <Input.Password />
          </Form.Item>
          <Form.Item
            label="原生 /responses"
            name="supports_responses"
            valuePropName="checked"
            extra="仅在第三方供应商原生支持 /responses 时开启。薄网关模式不会做任何协议补偿。"
          >
            <Switch checkedChildren="已支持" unCheckedChildren="未支持" />
          </Form.Item>
          <div className="modal-footer">
            <Button onClick={() => setInternalAddModalMode(null)}>取消</Button>
            <Button type="primary" htmlType="submit">
              保存
            </Button>
          </div>
        </Form>
      </Modal>

      <Modal
        open={internalAddModalMode === "official"}
        title="添加官方账户"
        onCancel={() => setInternalAddModalMode(null)}
        footer={null}
        destroyOnHidden
      >
        <Form form={officialForm} layout="vertical" onFinish={(values) => void handleCreateOfficial(values)}>
          <Form.Item label="账户名称" name="account_name" initialValue="local-codex">
            <Input />
          </Form.Item>
          <Text type="secondary">将直接读取当前用户目录下的 <code>~/.codex/auth.json</code>。</Text>
          <div className="modal-footer">
            <Button onClick={() => setInternalAddModalMode(null)}>取消</Button>
            <Button type="primary" htmlType="submit">
              导入
            </Button>
          </div>
        </Form>
      </Modal>

      <Modal
        open={!!editingAccount}
        title="编辑账户"
        onCancel={() => setEditingAccount(null)}
        footer={null}
        destroyOnHidden
      >
        <Form form={editForm} layout="vertical" onFinish={(values) => void handleEdit(values)}>
          <Form.Item label="账户名称" name="account_name" rules={[{ required: true, message: "请输入账户名称" }]}>
            <Input />
          </Form.Item>
          <Form.Item label="来源图标" name="source_icon" rules={[{ required: true, message: "请选择来源图标" }]}>
            <Select
              options={(Object.keys(sourceIconMap) as SourceIcon[]).map((key) => ({
                value: key,
                label: (
                  <span className="source-option">
                    <Avatar src={sourceIconMap[key].icon} size={20} shape="square" />
                    <span>{sourceIconMap[key].label}</span>
                  </span>
                ),
              }))}
            />
          </Form.Item>
          <Form.Item label="接口地址" name="base_url" rules={[{ required: true, message: "请输入接口地址" }]}>
            <Input />
          </Form.Item>
          <Form.Item label="API Key / Token" name="credential_ref">
            <Input.Password placeholder="留空表示不修改" />
          </Form.Item>
          <Form.Item
            label="原生 /responses"
            name="supports_responses"
            valuePropName="checked"
            extra="仅在供应商原生支持 /responses 时开启。薄网关模式不会做任何协议补偿。"
          >
            <Switch checkedChildren="已支持" unCheckedChildren="未支持" />
          </Form.Item>
          <div className="modal-footer">
            <Button onClick={() => setEditingAccount(null)}>取消</Button>
            <Button type="primary" htmlType="submit">
              保存
            </Button>
          </div>
        </Form>
        <div className="edit-test-panel">
          <Text strong>连接测试</Text>
          <Form form={testForm} layout="vertical" initialValues={{ model: "gpt-5.4", input: "ping" }} onFinish={(values) => void handleTest(values)}>
            <Form.Item label="模型" name="model" rules={[{ required: true, message: "请选择模型" }]}>
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
            <Form.Item label="输入内容" name="input" rules={[{ required: true, message: "请输入测试内容" }]}>
              <Input.TextArea rows={3} />
            </Form.Item>
            <div className="modal-footer">
              <Button htmlType="submit">测试</Button>
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
