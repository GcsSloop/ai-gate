import { CheckCircleOutlined, InfoCircleOutlined, MoreOutlined, PlusOutlined } from "@ant-design/icons";
import {
  Button,
  Card,
  Descriptions,
  Dropdown,
  Form,
  Input,
  Modal,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
  message,
} from "antd";
import type { ColumnsType } from "antd/es/table";
import { useEffect, useState } from "react";

import {
  createAccount,
  deleteAccount,
  importCurrentCodexAuth,
  listAccountUsage,
  listAccounts,
  testAccount,
  updateAccount,
  type AccountRecord,
  type AccountTestResult,
} from "../../lib/api";
import { refreshDesktopTrayState } from "../../lib/desktop-shell";

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
  const [testingAccount, setTestingAccount] = useState<AccountRecord | null>(null);
  const [testResult, setTestResult] = useState<AccountTestResult | null>(null);

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
    editForm.setFieldsValue({
      account_name: account.account_name,
      base_url: account.base_url,
      credential_ref: "",
      supports_responses: !!account.supports_responses,
    });
  }

  async function handleEdit(values: { account_name: string; base_url: string; credential_ref?: string; supports_responses?: boolean }) {
    if (!editingAccount) {
      return;
    }
    await updateAccount(editingAccount.id, {
      account_name: values.account_name,
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

  function openTestModal(account: AccountRecord) {
    setTestingAccount(account);
    setTestResult(null);
    testForm.setFieldsValue({
      model: getDefaultTestModel(account),
      input: "ping",
    });
  }

  async function handleTest(values: { model: string; input: string }) {
    if (!testingAccount) {
      return;
    }
    const result = await testAccount(testingAccount.id, values);
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

  const columns: ColumnsType<AccountRecord> = [
    {
      title: "账户",
      dataIndex: "account_name",
      render: (_, record) => (
        <div>
          <Text strong>{record.account_name}</Text>
          {record.is_active ? (
            <>
              {" "}
              <Tag color="green">当前激活</Tag>
            </>
          ) : null}
        </div>
      ),
    },
    {
      title: "认证方式",
      dataIndex: "auth_mode",
      render: (value: string) => authModeTextMap[value] ?? value,
    },
    {
      title: "状态",
      dataIndex: "status",
      render: (value: string) => <Tag color={statusColorMap[value] ?? "default"}>{statusTextMap[value] ?? value}</Tag>,
    },
    {
      title: "操作",
      key: "actions",
      width: 180,
      render: (_, record) => (
        <Space>
          <Button
            type="text"
            aria-label={`设为激活-${record.account_name}`}
            icon={<CheckCircleOutlined />}
            disabled={record.is_active}
            onClick={() => void handleSetActive(record)}
          />
          <Button type="text" aria-label={`详情-${record.account_name}`} icon={<InfoCircleOutlined />} onClick={() => setDetailAccount(record)} />
          <Dropdown
            trigger={["click"]}
            menu={{
              items: [
                { key: "edit", label: "编辑" },
                { key: "test", label: "测试" },
                { key: "delete", label: "删除", danger: true },
              ],
              onClick: ({ key }) => {
                if (key === "edit") {
                  openEditModal(record);
                  return;
                }
                if (key === "test") {
                  openTestModal(record);
                  return;
                }
                if (key === "delete") {
                  void Modal.confirm({
                    title: `确认删除账户「${record.account_name}」吗？`,
                    okText: "删除",
                    cancelText: "取消",
                    okButtonProps: { danger: true },
                    onOk: () => handleDelete(record),
                  });
                }
              },
            }}
          >
            <Button type="text" aria-label={`更多-${record.account_name}`} icon={<MoreOutlined />} />
          </Dropdown>
        </Space>
      ),
    },
  ];

  return (
    <div className="dashboard-page">
      {contextHolder}
      <div className="dashboard-header">
        <div>
          <Title level={2} style={{ marginBottom: 8 }}>
            账户列表
          </Title>
          <Text type="secondary">主表仅展示核心状态，详细信息请通过详情查看。</Text>
        </div>
        {showAddButton ? (
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
        ) : null}
      </div>

      <Card className="accounts-card" variant="borderless">
        <Table
          rowKey="id"
          columns={columns}
          dataSource={accounts}
          pagination={false}
          rowClassName={(record) => (record.is_active ? "active-account-row" : "")}
        />
      </Card>

      <Modal open={!!detailAccount} title="账户详情" onCancel={() => setDetailAccount(null)} footer={null} destroyOnHidden>
        {detailAccount ? (
          <Descriptions column={1} size="small">
            <Descriptions.Item label="账户">{detailAccount.account_name}</Descriptions.Item>
            <Descriptions.Item label="平台">{detailAccount.provider_type}</Descriptions.Item>
            <Descriptions.Item label="认证方式">{authModeTextMap[detailAccount.auth_mode] ?? detailAccount.auth_mode}</Descriptions.Item>
            <Descriptions.Item label="状态">{statusTextMap[detailAccount.status] ?? detailAccount.status}</Descriptions.Item>
            <Descriptions.Item label="接口地址">{detailAccount.base_url || "OpenAI 官方"}</Descriptions.Item>
            <Descriptions.Item label="能力">{detailAccount.supports_responses ? "/responses" : "/responses 未启用"}</Descriptions.Item>
            <Descriptions.Item label="最近 Token">
              {Math.round(detailAccount.last_total_tokens)}
              {detailAccount.model_context_window > 0 ? ` / 上下文 ${Math.round(detailAccount.model_context_window)}` : ""}
            </Descriptions.Item>
            <Descriptions.Item label="5 小时剩余">
              {(100 - detailAccount.primary_used_percent).toFixed(0)}% · {formatResetAt(detailAccount.primary_resets_at)}
            </Descriptions.Item>
            <Descriptions.Item label="1 周剩余">
              {(100 - detailAccount.secondary_used_percent).toFixed(0)}% · {formatResetAt(detailAccount.secondary_resets_at)}
            </Descriptions.Item>
            <Descriptions.Item label="余额">{detailAccount.balance.toFixed(2)}</Descriptions.Item>
            <Descriptions.Item label="额度">{Math.round(detailAccount.quota_remaining)}</Descriptions.Item>
            <Descriptions.Item label="RPM">{Math.round(detailAccount.rpm_remaining)}</Descriptions.Item>
            <Descriptions.Item label="TPM">{Math.round(detailAccount.tpm_remaining)}</Descriptions.Item>
            <Descriptions.Item label="健康分">{detailAccount.health_score.toFixed(2)}</Descriptions.Item>
            <Descriptions.Item label="错误率">{(detailAccount.recent_error_rate * 100).toFixed(1)}%</Descriptions.Item>
          </Descriptions>
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
      </Modal>

      <Modal
        open={!!testingAccount}
          title="对话测试"
        onCancel={() => setTestingAccount(null)}
        footer={null}
        destroyOnHidden
      >
        <Form
          form={testForm}
          layout="vertical"
          initialValues={{ model: testingAccount ? getDefaultTestModel(testingAccount) : "gpt-5.4", input: "ping" }}
          onFinish={(values) => void handleTest(values)}
        >
          <Descriptions size="small" column={1} className="test-account-meta">
            <Descriptions.Item label="账户">{testingAccount?.account_name}</Descriptions.Item>
            <Descriptions.Item label="类型">{testingAccount ? authModeTextMap[testingAccount.auth_mode] ?? testingAccount.auth_mode : "-"}</Descriptions.Item>
          </Descriptions>
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
            <Input.TextArea rows={5} />
          </Form.Item>
          <div className="modal-footer">
            <Button onClick={() => setTestingAccount(null)}>关闭</Button>
            <Button type="primary" htmlType="submit">
              发送测试
            </Button>
          </div>
        </Form>
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
