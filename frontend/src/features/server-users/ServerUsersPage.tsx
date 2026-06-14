import { CopyOutlined, PlusOutlined, ReloadOutlined, StopOutlined, TeamOutlined } from "@ant-design/icons";
import { Alert, Button, Checkbox, Input, Modal, Space, Table, Tag, Typography, message } from "antd";
import { useEffect, useState } from "react";

import { createServerUser, disableServerUser, listServerUserAccounts, listServerUsers, rotateServerUserToken, setServerUserAccounts, type ServerUser, type ServerUserAccountAssignment } from "../../lib/api";

type ServerUsersPageProps = {
  t: (value: string) => string;
};

export function ServerUsersPage({ t }: ServerUsersPageProps) {
  const [users, setUsers] = useState<ServerUser[]>([]);
  const [loading, setLoading] = useState(false);
  const [name, setName] = useState("");
  const [issuedToken, setIssuedToken] = useState("");
  const [assignmentUser, setAssignmentUser] = useState<ServerUser | null>(null);
  const [assignments, setAssignments] = useState<ServerUserAccountAssignment[]>([]);
  const [selectedAccountIDs, setSelectedAccountIDs] = useState<number[]>([]);
  const [assignmentLoading, setAssignmentLoading] = useState(false);
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

  async function openAssignments(user: ServerUser) {
    setAssignmentUser(user);
    setAssignmentLoading(true);
    try {
      const items = await listServerUserAccounts(user.id);
      setAssignments(items);
      setSelectedAccountIDs(items.filter((item) => item.assigned).map((item) => item.account_id));
    } finally {
      setAssignmentLoading(false);
    }
  }

  async function saveAssignments() {
    if (!assignmentUser) {
      return;
    }
    setAssignmentLoading(true);
    try {
      await setServerUserAccounts(assignmentUser.id, selectedAccountIDs);
      setAssignmentUser(null);
      setAssignments([]);
      setSelectedAccountIDs([]);
      await refresh();
    } finally {
      setAssignmentLoading(false);
    }
  }

  function toggleSelectedAccount(accountID: number, checked: boolean) {
    setSelectedAccountIDs((current) => {
      if (checked) {
        return current.includes(accountID) ? current : [...current, accountID];
      }
      return current.filter((value) => value !== accountID);
    });
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
          { title: t("账户池"), dataIndex: "assigned_accounts", align: "right", render: (value?: number) => value ?? 0 },
          {
            title: t("操作"),
            key: "actions",
            align: "right",
            render: (_: unknown, user: ServerUser) => (
              <Space>
                <Button icon={<TeamOutlined />} aria-label={t("分配账户")} onClick={() => void openAssignments(user)}>
                  {t("分配账户")}
                </Button>
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
      <Modal
        open={assignmentUser !== null}
        title={assignmentUser ? `${t("分配账户")} · ${assignmentUser.name}` : t("分配账户")}
        okText={t("保存")}
        cancelText={t("取消")}
        okButtonProps={{ "aria-label": t("保存") }}
        confirmLoading={assignmentLoading}
        onOk={() => void saveAssignments()}
        onCancel={() => {
          setAssignmentUser(null);
          setAssignments([]);
          setSelectedAccountIDs([]);
        }}
      >
        <Space orientation="vertical" className="server-users-assignment-list">
          {assignments.map((item) => (
            <Checkbox key={item.account_id} checked={selectedAccountIDs.includes(item.account_id)} aria-label={item.account_name} onChange={(event) => toggleSelectedAccount(item.account_id, event.target.checked)}>
              <Space>
                <span>{item.account_name}</span>
                <Tag>{item.provider_type}</Tag>
                <Tag color={item.status === "active" ? "green" : "default"}>{item.status}</Tag>
              </Space>
            </Checkbox>
          ))}
        </Space>
      </Modal>
    </div>
  );
}
