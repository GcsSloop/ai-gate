import { ReloadOutlined } from "@ant-design/icons";
import { Button, InputNumber, Space, Switch, Table, Tag, Typography } from "antd";
import { useEffect, useState } from "react";

import { getServerMe, listMyServerAccounts, updateMyServerAccountState, type ServerAssignedAccount, type ServerMe } from "../../lib/api";

type UserPoolPageProps = {
  t: (value: string) => string;
};

export function UserPoolPage({ t }: UserPoolPageProps) {
  const [me, setMe] = useState<ServerMe | null>(null);
  const [accounts, setAccounts] = useState<ServerAssignedAccount[]>([]);
  const [loading, setLoading] = useState(false);

  async function refresh() {
    setLoading(true);
    try {
      const [nextMe, nextAccounts] = await Promise.all([getServerMe(), listMyServerAccounts()]);
      setMe(nextMe);
      setAccounts(nextAccounts);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  async function patchAccount(account: ServerAssignedAccount, patch: Partial<Pick<ServerAssignedAccount, "position" | "is_active" | "is_locked">>) {
    const next = {
      position: patch.position ?? account.position,
      is_active: patch.is_active ?? account.is_active,
      is_locked: patch.is_locked ?? account.is_locked,
    };
    setAccounts((items) => items.map((item) => (item.account_id === account.account_id ? { ...item, ...next } : item)));
    await updateMyServerAccountState(account.account_id, next);
  }

  return (
    <div className="page-surface">
      <div className="page-header-row">
        <div>
          <Typography.Title level={3}>{t("我的网关")}</Typography.Title>
          <Typography.Text type="secondary">
            {me ? `${me.user.name} · ${t("请求数")} ${me.request_count} · ${t("总 Tokens")} ${me.total_tokens}` : t("正在载入")}
          </Typography.Text>
        </div>
        <Button icon={<ReloadOutlined />} loading={loading} onClick={() => void refresh()} />
      </div>
      <Table
        rowKey="account_id"
        loading={loading}
        dataSource={accounts}
        pagination={false}
        columns={[
          { title: t("账户"), dataIndex: "account_name" },
          { title: t("类型"), dataIndex: "provider_type" },
          {
            title: t("状态"),
            dataIndex: "status",
            render: (status: string) => <Tag color={status === "active" ? "green" : "default"}>{status}</Tag>,
          },
          {
            title: t("顺序"),
            dataIndex: "position",
            width: 120,
            render: (_: unknown, account: ServerAssignedAccount) => (
              <InputNumber min={0} value={account.position} onChange={(value) => void patchAccount(account, { position: Number(value ?? 0) })} />
            ),
          },
          {
            title: t("激活"),
            dataIndex: "is_active",
            render: (_: unknown, account: ServerAssignedAccount) => (
              <Switch aria-label={`${t("激活")} ${account.account_name}`} checked={account.is_active} onChange={(checked) => void patchAccount(account, { is_active: checked })} />
            ),
          },
          {
            title: t("锁定"),
            dataIndex: "is_locked",
            render: (_: unknown, account: ServerAssignedAccount) => (
              <Switch aria-label={`${t("锁定")} ${account.account_name}`} checked={account.is_locked} onChange={(checked) => void patchAccount(account, { is_locked: checked })} />
            ),
          },
        ]}
        locale={{ emptyText: t("未分配账户池") }}
      />
      <Space />
    </div>
  );
}
