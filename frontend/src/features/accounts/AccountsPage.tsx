import { HolderOutlined, PlusOutlined } from "@ant-design/icons";
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
import { useEffect, useRef, useState, type HTMLAttributes } from "react";

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

export function AccountsPage() {
  const [messageApi, contextHolder] = message.useMessage();
  const [accounts, setAccounts] = useState<AccountRecord[]>([]);
  const [addModalMode, setAddModalMode] = useState<AddModalMode>(null);
  const [editingAccount, setEditingAccount] = useState<AccountRecord | null>(null);
  const [testingAccount, setTestingAccount] = useState<AccountRecord | null>(null);
  const [testResult, setTestResult] = useState<AccountTestResult | null>(null);
  const dragIndexRef = useRef<number | null>(null);

  const [thirdPartyForm] = Form.useForm();
  const [officialForm] = Form.useForm();
  const [editForm] = Form.useForm();
  const [testForm] = Form.useForm();
  const editAllowFallback = Form.useWatch("allow_chat_fallback", editForm) ?? false;

  useEffect(() => {
    void refreshAll();
  }, []);

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

  async function handleCreateThirdParty(values: { account_name: string; base_url: string; credential_ref: string; supports_responses?: boolean; allow_chat_fallback?: boolean }) {
    await createAccount({
      provider_type: "openai-compatible",
      account_name: values.account_name,
      auth_mode: "api_key",
      base_url: values.base_url,
      credential_ref: values.credential_ref,
      supports_responses: !!values.supports_responses,
      allow_chat_fallback: !!values.allow_chat_fallback,
    });
    setAddModalMode(null);
    thirdPartyForm.resetFields();
    await refreshAll();
    void messageApi.success("第三方账户已添加");
  }

  async function handleCreateOfficial(values: { account_name: string }) {
    await importCurrentCodexAuth(values.account_name || "local-codex");
    officialForm.resetFields();
    setAddModalMode(null);
    await refreshAll();
    void messageApi.success("官方账户已导入");
  }

  function openEditModal(account: AccountRecord) {
    setEditingAccount(account);
    editForm.setFieldsValue({
      account_name: account.account_name,
      base_url: account.base_url,
      credential_ref: "",
      supports_responses: !!account.supports_responses,
      allow_chat_fallback: !!account.allow_chat_fallback,
    });
  }

  async function handleEdit(values: { account_name: string; base_url: string; credential_ref?: string; supports_responses?: boolean; allow_chat_fallback?: boolean }) {
    if (!editingAccount) {
      return;
    }
    await updateAccount(editingAccount.id, {
      account_name: values.account_name,
      base_url: values.base_url,
      credential_ref: values.credential_ref || undefined,
      supports_responses: !!values.supports_responses,
      allow_chat_fallback: !!values.allow_chat_fallback,
    });
    setEditingAccount(null);
    editForm.resetFields();
    await refreshAll();
    void messageApi.success("账户已更新");
  }

  async function handleDelete(account: AccountRecord) {
    await deleteAccount(account.id);
    await refreshAll();
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

  async function handleReorder(fromIndex: number, toIndex: number) {
    if (fromIndex === toIndex || fromIndex < 0 || toIndex < 0) {
      return;
    }
    const previous = [...accounts];
    const reordered = [...accounts];
    const [moved] = reordered.splice(fromIndex, 1);
    reordered.splice(toIndex, 0, moved);
    setAccounts(reordered);
    try {
      for (let index = 0; index < reordered.length; index += 1) {
        const account = reordered[index];
        await updateAccount(account.id, { priority: reordered.length - index });
      }
      void messageApi.success("优先级顺序已更新");
    } catch (error) {
      setAccounts(previous);
      void messageApi.error(error instanceof Error ? error.message : "优先级更新失败，已回滚");
    }
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
      void messageApi.success(`已切换当前激活账户为 ${account.account_name}`);
    } catch (error) {
      setAccounts(previous);
      void messageApi.error(error instanceof Error ? error.message : "切换激活账户失败");
    }
  }

  const columns: ColumnsType<AccountRecord> = [
    {
      title: "",
      dataIndex: "drag",
      width: 44,
      render: () => <HolderOutlined className="drag-handle" />,
    },
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
          <br />
          <Text type="secondary">{record.base_url || "OpenAI 官方"}</Text>
          {record.last_total_tokens > 0 || record.primary_used_percent > 0 || record.secondary_used_percent > 0 ? (
            <>
              <br />
              <Text type="secondary">
                最近 Token {Math.round(record.last_total_tokens)}
                {record.model_context_window > 0 ? ` / 上下文 ${Math.round(record.model_context_window)}` : ""}
              </Text>
              <br />
              <Text type="secondary">
                5 小时剩余 {(100 - record.primary_used_percent).toFixed(0)}% · {formatResetAt(record.primary_resets_at)}
              </Text>
              <br />
              <Text type="secondary">
                1 周剩余 {(100 - record.secondary_used_percent).toFixed(0)}% · {formatResetAt(record.secondary_resets_at)}
              </Text>
            </>
          ) : null}
        </div>
      ),
    },
    {
      title: "平台",
      dataIndex: "provider_type",
    },
    {
      title: "认证方式",
      dataIndex: "auth_mode",
      render: (value: string) => authModeTextMap[value] ?? value,
    },
    {
      title: "能力",
      key: "capabilities",
      render: (_, record) => (
        <Space size={4} wrap>
          {record.supports_responses ? <Tag color="blue">/responses</Tag> : <Tag>/responses 未启用</Tag>}
        </Space>
      ),
    },
    {
      title: "状态",
      dataIndex: "status",
      render: (value: string) => <Tag color={statusColorMap[value] ?? "default"}>{statusTextMap[value] ?? value}</Tag>,
    },
    {
      title: "余额",
      dataIndex: "balance",
      render: (value: number) => value.toFixed(2),
    },
    {
      title: "额度",
      dataIndex: "quota_remaining",
      render: (value: number) => Math.round(value),
    },
    {
      title: "RPM",
      dataIndex: "rpm_remaining",
      render: (value: number) => Math.round(value),
    },
    {
      title: "TPM",
      dataIndex: "tpm_remaining",
      render: (value: number) => Math.round(value),
    },
    {
      title: "健康分",
      dataIndex: "health_score",
      render: (value: number) => value.toFixed(2),
    },
    {
      title: "错误率",
      dataIndex: "recent_error_rate",
      render: (value: number) => `${(value * 100).toFixed(1)}%`,
    },
    {
      title: "操作",
      key: "actions",
      width: 220,
      render: (_, record) => (
        <Space>
          <Button type="link" disabled={record.is_active} onClick={() => void handleSetActive(record)}>
            设为激活
          </Button>
          <Button type="link" onClick={() => openEditModal(record)}>
            编辑
          </Button>
          <Button type="link" onClick={() => openTestModal(record)}>
            测试
          </Button>
          <Button danger type="link" onClick={() => void Modal.confirm({
            title: `确认删除账户「${record.account_name}」吗？`,
            okText: "删除",
            cancelText: "取消",
            okButtonProps: { danger: true },
            onOk: () => handleDelete(record),
          })}>
            删除
          </Button>
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
          <Text type="secondary">拖拽顺序仅用于故障切换优先级；当前激活账户由手动切换决定。</Text>
        </div>
        <Dropdown
          menu={{
            items: [
              { key: "official", label: "官方账户" },
              { key: "third_party", label: "第三方账户" },
            ],
            onClick: ({ key }) => setAddModalMode(key as AddModalMode),
          }}
          trigger={["click"]}
        >
          <Button type="primary" icon={<PlusOutlined />}>
            添加账户
          </Button>
        </Dropdown>
      </div>

      <Card className="accounts-card" variant="borderless">
        <Table
          rowKey="id"
          columns={columns}
          dataSource={accounts}
          pagination={false}
          rowClassName={(record) => (record.is_active ? "active-account-row" : "")}
          components={{
            body: {
              row: (props: HTMLAttributes<HTMLTableRowElement> & { "data-row-key"?: string }) => {
                const rowKey = props["data-row-key"];
                const currentIndex = accounts.findIndex((account) => String(account.id) === String(rowKey));
                return (
                  <tr
                    {...props}
                    draggable
                    onDragStart={() => {
                      dragIndexRef.current = currentIndex;
                    }}
                    onDragOver={(event) => {
                      event.preventDefault();
                    }}
                    onDrop={(event) => {
                      event.preventDefault();
                      void handleReorder(dragIndexRef.current ?? -1, currentIndex);
                      dragIndexRef.current = null;
                    }}
                  />
                );
              },
            },
          }}
        />
      </Card>

      <Modal
        open={addModalMode === "third_party"}
        title="添加第三方账户"
        onCancel={() => setAddModalMode(null)}
        footer={null}
        destroyOnHidden
      >
        <Form
          form={thirdPartyForm}
          layout="vertical"
          initialValues={{ base_url: defaultBaseURL, supports_responses: false, allow_chat_fallback: false }}
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
          <Form.Item
            label="回退配置"
            name="allow_chat_fallback"
            valuePropName="checked"
            extra="默认关闭。关闭时强制使用 /responses；开启后当 /responses 不可用会回退 /chat/completions。"
          >
            <Switch checkedChildren="允许回退" unCheckedChildren="强制 responses" />
          </Form.Item>
          <div className="modal-footer">
            <Button onClick={() => setAddModalMode(null)}>取消</Button>
            <Button type="primary" htmlType="submit">
              保存
            </Button>
          </div>
        </Form>
      </Modal>

      <Modal
        open={addModalMode === "official"}
        title="添加官方账户"
        onCancel={() => setAddModalMode(null)}
        footer={null}
        destroyOnHidden
      >
        <Form form={officialForm} layout="vertical" onFinish={(values) => void handleCreateOfficial(values)}>
          <Form.Item label="账户名称" name="account_name" initialValue="local-codex">
            <Input />
          </Form.Item>
          <Text type="secondary">将直接读取当前用户目录下的 <code>~/.codex/auth.json</code>。</Text>
          <div className="modal-footer">
            <Button onClick={() => setAddModalMode(null)}>取消</Button>
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
          <Form.Item label="回退状态">
            <Button onClick={() => editForm.setFieldValue("allow_chat_fallback", !editAllowFallback)}>
              {editAllowFallback ? "已开启回退（点击关闭）" : "已关闭回退（点击开启）"}
            </Button>
          </Form.Item>
          <Form.Item
            label="回退配置"
            name="allow_chat_fallback"
            valuePropName="checked"
            extra="默认关闭。关闭时强制使用 /responses；开启后当 /responses 不可用会回退 /chat/completions。"
          >
            <Switch checkedChildren="允许回退" unCheckedChildren="强制 responses" />
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
