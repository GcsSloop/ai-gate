import { CopyOutlined, PlusOutlined, ReloadOutlined, StopOutlined } from "@ant-design/icons";
import { Alert, Button, Input, Modal, Space, Table, Tag, Typography, message } from "antd";
import { useEffect, useState } from "react";

import { createServerUser, disableServerUser, listServerUsers, rotateServerUserToken, type ServerUser } from "../../lib/api";

type ServerUsersPageProps = {
  t: (value: string) => string;
};

export function ServerUsersPage({ t }: ServerUsersPageProps) {
  const [users, setUsers] = useState<ServerUser[]>([]);
  const [loading, setLoading] = useState(false);
  const [name, setName] = useState("");
  const [issuedToken, setIssuedToken] = useState("");
  const [messageApi, contextHolder] = message.useMessage();

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
    setIssuedToken(created.token);
    await refresh();
  }

  async function handleRotate(user: ServerUser) {
    const rotated = await rotateServerUserToken(user.id);
    setIssuedToken(rotated.token);
    await refresh();
  }

  async function copyToken() {
    await navigator.clipboard.writeText(issuedToken);
    void messageApi.success(t("令牌已复制"));
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
              <Button icon={<CopyOutlined />} onClick={() => void copyToken()} />
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
              </Space>
            ),
          },
        ]}
      />
    </div>
  );
}
