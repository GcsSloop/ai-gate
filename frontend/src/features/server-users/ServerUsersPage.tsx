import { CopyOutlined, DeleteOutlined, PlusOutlined, ReloadOutlined, StopOutlined } from "@ant-design/icons";
import { Alert, Button, Input, Modal, Space, Table, Tag, Typography, message } from "antd";
import { useEffect, useState } from "react";

import { createServerUser, deleteServerUser, disableServerUser, listServerUsers, rotateServerUserToken, type CreatedServerUser, type ServerUser } from "../../lib/api";
import { writeClipboardText } from "../../lib/clipboard";

type ServerUsersPageProps = {
  t: (value: string) => string;
};

export function ServerUsersPage({ t }: ServerUsersPageProps) {
  const [users, setUsers] = useState<ServerUser[]>([]);
  const [loading, setLoading] = useState(false);
  const [name, setName] = useState("");
  const [issuedCredential, setIssuedCredential] = useState<CreatedServerUser | null>(null);
  const [messageApi, contextHolder] = message.useMessage();
  const issuedToken = issuedCredential?.token ?? "";

  async function refresh() {
    setLoading(true);
    try {
      setUsers(await listServerUsers());
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  async function handleCreate() {
    const trimmed = name.trim();
    if (!trimmed) {
      return;
    }
    const created = await createServerUser(trimmed);
    setName("");
    setIssuedCredential(created);
    await refresh();
  }

  async function handleRotate(user: ServerUser) {
    const rotated = await rotateServerUserToken(user.id);
    setIssuedCredential(rotated);
    await refresh();
  }

  async function copyIssuedImportPayload() {
    if (!issuedCredential) {
      return;
    }
    await writeClipboardText(buildServerUserImportPayload(issuedCredential));
    void messageApi.success(t("AI Gate 导入配置已复制"));
  }

  return (
    <div className="page-surface">
      {contextHolder}
      <div className="page-header-row">
        <div>
          <Typography.Title level={3}>{t("服务用户")}</Typography.Title>
          <Typography.Text type="secondary">{t("为网关调用签发令牌，并按用户统计用量。")}</Typography.Text>
        </div>
        <Button icon={<ReloadOutlined />} onClick={() => void refresh()} loading={loading} />
      </div>
      <Space.Compact className="server-users-create-row">
        <Input value={name} onChange={(event) => setName(event.target.value)} placeholder={t("用户名称")} onPressEnter={() => void handleCreate()} />
        <Button type="primary" icon={<PlusOutlined />} onClick={() => void handleCreate()}>
          {t("新建用户")}
        </Button>
      </Space.Compact>
      {issuedToken ? (
        <Alert
          type="success"
          showIcon
          title={t("仅显示一次的新令牌")}
          description={
            <Space.Compact className="server-users-token-row">
              <Input value={issuedToken} readOnly />
              <Button
                aria-label={t("复制 AI Gate 导入配置")}
                icon={<CopyOutlined />}
                onClick={() => void copyIssuedImportPayload()}
              />
            </Space.Compact>
          }
        />
      ) : null}
      <Table
        rowKey="id"
        loading={loading}
        dataSource={users}
        pagination={false}
        columns={[
          { title: t("用户"), dataIndex: "name" },
          {
            title: t("状态"),
            dataIndex: "status",
            render: (status: string) => <Tag color={status === "active" ? "green" : "default"}>{status}</Tag>,
          },
          { title: t("请求数"), dataIndex: "request_count", align: "right" },
          { title: t("总 Tokens"), dataIndex: "total_tokens", align: "right" },
          {
            title: t("操作"),
            key: "actions",
            align: "right",
            render: (_: unknown, user: ServerUser) => (
              <Space>
                <Button icon={<ReloadOutlined />} onClick={() => void handleRotate(user)} />
                <Button
                  danger
                  aria-label={`${t("禁用用户")}-${user.name}`}
                  icon={<StopOutlined />}
                  disabled={user.status !== "active"}
                  onClick={() => {
                    Modal.confirm({
                      title: t("禁用用户"),
                      content: user.name,
                      onOk: async () => {
                        await disableServerUser(user.id);
                        await refresh();
                      },
                    });
                  }}
                />
                <Button
                  danger
                  aria-label={`${t("删除用户")}-${user.name}`}
                  icon={<DeleteOutlined />}
                  onClick={() => {
                    Modal.confirm({
                      title: t("删除用户"),
                      content: user.name,
                      okText: t("确认删除"),
                      cancelText: t("取消"),
                      okButtonProps: { danger: true },
                      onOk: async () => {
                        await deleteServerUser(user.id);
                        if (issuedCredential?.user.id === user.id) {
                          setIssuedCredential(null);
                        }
                        await refresh();
                      },
                    });
                  }}
                />
              </Space>
            ),
          },
        ]}
      />
    </div>
  );
}

function buildServerUserImportPayload(issued: CreatedServerUser): string {
  return JSON.stringify(
    {
      kind: "aigate-account-share",
      schema_version: 1,
      exported_at: new Date().toISOString(),
      account: {
        provider_type: "openai-compatible",
        account_name: issued.user.name || issued.user.username || "AI Gate Server",
        source_icon: "",
        auth_mode: "api_key",
        base_url: serverGatewayBaseURL(),
        credential_ref: issued.token,
        account_driver: "builtin_api_key",
        usage_driver: "",
        usage_config_json: "",
        supports_responses: true,
        skip_tls_verify: false,
      },
    },
    null,
    2,
  );
}

function serverGatewayBaseURL(): string {
  if (typeof window === "undefined") {
    return "/ai-gate/v1";
  }
  const pathname = window.location.pathname || "";
  const webuiIndex = pathname.indexOf("/webui");
  const routePrefix = webuiIndex > 0 ? pathname.slice(0, webuiIndex).replace(/\/+$/, "") : "/ai-gate";
  const prefix = routePrefix || "/ai-gate";
  return `${window.location.origin}${prefix}/v1`;
}
