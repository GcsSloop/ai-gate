import { CheckCircleOutlined, LockOutlined, ReloadOutlined, SwapOutlined, UnlockOutlined } from "@ant-design/icons";
import { Button, Collapse, Space, Statistic, Tag, Tooltip, Typography } from "antd";
import { useEffect, useState } from "react";

import { getServerMe, getServerUpstreams, updateServerRoute, updateServerUpstreamLock, type ServerMe, type ServerUpstreamAccount, type ServerUpstreams } from "../../lib/api";

type UserPoolPageProps = {
  t: (value: string) => string;
};

export function UserPoolPage({ t }: UserPoolPageProps) {
  const [me, setMe] = useState<ServerMe | null>(null);
  const [upstreams, setUpstreams] = useState<ServerUpstreams | null>(null);
  const [loading, setLoading] = useState(false);
  const [routeUpdatingID, setRouteUpdatingID] = useState<number | null>(null);

  async function refresh() {
    setLoading(true);
    try {
      const [nextMe, nextUpstreams] = await Promise.all([getServerMe(), getServerUpstreams()]);
      setMe(nextMe);
      setUpstreams(nextUpstreams);
    } finally {
      setLoading(false);
    }
  }

  async function refreshUpstreams() {
    setUpstreams(await getServerUpstreams());
  }

  async function handleSwitch(account: ServerUpstreamAccount) {
    setRouteUpdatingID(account.id);
    try {
      await updateServerRoute({ account_id: account.id, locked: false });
      await refreshUpstreams();
    } finally {
      setRouteUpdatingID(null);
    }
  }

  async function handleToggleLock(account: ServerUpstreamAccount) {
    const locked = !account.account_locked;
    setRouteUpdatingID(account.id);
    try {
      await updateServerUpstreamLock(account.id, locked);
      await refreshUpstreams();
    } finally {
      setRouteUpdatingID(null);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

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
      <div className="stats-summary-grid">
        <Statistic title={t("请求数")} value={me?.request_count ?? 0} loading={loading} />
        <Statistic title={t("总 Tokens")} value={me?.total_tokens ?? 0} loading={loading} />
      </div>
      <div className="user-upstreams-receipt">
        <Collapse
          className="user-upstreams-collapse"
          defaultActiveKey={["upstreams"]}
          expandIconPlacement="end"
          items={[
            {
              key: "upstreams",
              label: (
                <div className="user-upstreams-header">
                  <span>{t("上游账号")}</span>
                  <span className="user-upstreams-count">
                    {t("可用")} {upstreams?.available_accounts ?? 0} / {upstreams?.total_accounts ?? 0}
                  </span>
                </div>
              ),
              children: (
                <div className="user-upstream-list">
                  {(upstreams?.accounts ?? []).map((account) => (
                    <UpstreamRow
                      key={account.id}
                      account={account}
                      updating={routeUpdatingID === account.id}
                      t={t}
                      onSwitch={handleSwitch}
                      onToggleLock={handleToggleLock}
                    />
                  ))}
                  {upstreams && upstreams.accounts.length === 0 ? (
                    <Typography.Text type="secondary">{t("暂无上游账号")}</Typography.Text>
                  ) : null}
                </div>
              ),
            },
          ]}
        />
      </div>
    </div>
  );
}

type UpstreamRowProps = {
  account: ServerUpstreamAccount;
  updating: boolean;
  t: (value: string) => string;
  onSwitch: (account: ServerUpstreamAccount) => Promise<void>;
  onToggleLock: (account: ServerUpstreamAccount) => Promise<void>;
};

function UpstreamRow({ account, updating, t, onSwitch, onToggleLock }: UpstreamRowProps) {
  const selectable = canManuallySelectServerUpstream(account);
  return (
    <div className={account.current ? "user-upstream-row is-current" : "user-upstream-row"}>
      <div className="user-upstream-main">
        <div className="user-upstream-title-row">
          <Typography.Text strong>{account.account_name}</Typography.Text>
          {account.current ? (
            <Tag color="green" icon={<CheckCircleOutlined />}>
              {t("当前使用中")}
            </Tag>
          ) : null}
          {account.account_locked ? <Tag color="blue">{t("已锁定")}</Tag> : null}
          {!account.available && !account.account_locked ? <Tag>{t("不可用")}</Tag> : null}
        </div>
        <Typography.Text type="secondary" className="user-upstream-base-url">
          {account.base_url || t("OpenAI 官方")}
        </Typography.Text>
      </div>
      <div className="user-upstream-usage">
        <span>{usageSummary(account)}</span>
        <span>{t("Tokens")} {formatCompactNumber(account.last_total_tokens)}</span>
      </div>
      <Space size={6} className="user-upstream-actions">
        <Tooltip title={t("切换到此上游")}>
          <Button
            size="small"
            icon={<SwapOutlined />}
            loading={updating}
            disabled={!selectable}
            aria-label={`${t("切换")}-${account.account_name}`}
            onClick={() => void onSwitch(account)}
          />
        </Tooltip>
        <Tooltip title={account.account_locked ? t("解除锁定") : t("锁定上游账号")}>
          <Button
            size="small"
            icon={account.account_locked ? <UnlockOutlined /> : <LockOutlined />}
            loading={updating}
            disabled={!selectable}
            aria-label={`${account.account_locked ? t("解锁") : t("锁定")}-${account.account_name}`}
            onClick={() => void onToggleLock(account)}
          />
        </Tooltip>
      </Space>
    </div>
  );
}

function canManuallySelectServerUpstream(account: ServerUpstreamAccount): boolean {
  return account.status !== "disabled" && account.status !== "invalid";
}

function usageSummary(account: ServerUpstreamAccount): string {
  const display = account.usage_display?.summary;
  if (display?.label || display?.value) {
    return `${display.label ?? ""} ${display.value ?? ""}`.trim();
  }
  if (account.balance > 0) {
    return `余额 ${formatCompactNumber(account.balance)}`;
  }
  if (account.quota_remaining > 0) {
    return `额度 ${formatCompactNumber(account.quota_remaining)}`;
  }
  return "用量未知";
}

function formatCompactNumber(value: number): string {
  if (!Number.isFinite(value)) {
    return "0";
  }
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: value >= 100 ? 0 : 2 }).format(value);
}
