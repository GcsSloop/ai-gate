import { Card, Empty, Input, Select, Segmented, Spin, Tag } from "antd";
import { useEffect, useMemo, useState } from "react";

import {
  type AccountRecord,
  type UsageDashboardSummary,
  type UsageEventRecord,
  type UsageTrendPoint,
  getDashboardRecentEvents,
  getDashboardSummary,
  getDashboardTrends,
  listAccounts,
} from "../../lib/api";
import type { AppLanguage, Translator } from "../../lib/i18n";

type StatsPageProps = {
  language: AppLanguage;
  t: Translator;
};

type RangeOption = 24 | 168 | 720;

function formatCompactNumber(language: AppLanguage, value: number): string {
  return new Intl.NumberFormat(language, { notation: "compact", maximumFractionDigits: 1 }).format(value);
}

function formatCurrency(language: AppLanguage, value: number): string {
  return new Intl.NumberFormat(language, {
    style: "currency",
    currency: "USD",
    maximumFractionDigits: 4,
  }).format(value);
}

function formatSigned(language: AppLanguage, value: number): string {
  const formatter = new Intl.NumberFormat(language, { maximumFractionDigits: 2 });
  if (value > 0) {
    return `+${formatter.format(value)}`;
  }
  return formatter.format(value);
}

function eventStatusColor(status: string): string {
  if (status === "completed") {
    return "success";
  }
  if (status.includes("rate")) {
    return "warning";
  }
  return "default";
}

export function StatsPage({ language, t }: StatsPageProps) {
  const [loading, setLoading] = useState(true);
  const [rangeHours, setRangeHours] = useState<RangeOption>(24);
  const [accountID, setAccountID] = useState<number | undefined>(undefined);
  const [model, setModel] = useState("");
  const [accounts, setAccounts] = useState<AccountRecord[]>([]);
  const [summary, setSummary] = useState<UsageDashboardSummary | null>(null);
  const [trends, setTrends] = useState<UsageTrendPoint[]>([]);
  const [events, setEvents] = useState<UsageEventRecord[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let disposed = false;

    async function load() {
      setLoading(true);
      setError(null);
      try {
        const [accountList, nextSummary, nextTrends, nextEvents] = await Promise.all([
          listAccounts(),
          getDashboardSummary(rangeHours, accountID, model),
          getDashboardTrends(rangeHours, accountID, model),
          getDashboardRecentEvents(rangeHours, accountID, model, 20),
        ]);
        if (disposed) {
          return;
        }
        setAccounts(accountList);
        setSummary(nextSummary);
        setTrends(nextTrends);
        setEvents(nextEvents);
      } catch (loadError) {
        if (!disposed) {
          setError(loadError instanceof Error ? loadError.message : t("加载统计数据失败"));
        }
      } finally {
        if (!disposed) {
          setLoading(false);
        }
      }
    }

    void load();
    return () => {
      disposed = true;
    };
  }, [accountID, model, rangeHours, t]);

  const maxTrendTokens = useMemo(
    () => Math.max(...trends.map((item) => item.total_tokens), 1),
    [trends],
  );

  const statusSummary = useMemo(() => {
    const counts = new Map<string, number>();
    events.forEach((event) => {
      counts.set(event.status, (counts.get(event.status) ?? 0) + 1);
    });
    return Array.from(counts.entries());
  }, [events]);

  const summaryCards = [
    {
      label: t("请求数"),
      value: summary ? formatCompactNumber(language, summary.request_count) : "--",
      hint: summary ? `${summary.success_count} ${t("成功")} / ${summary.failure_count} ${t("失败")}` : "--",
    },
    {
      label: t("总 Token"),
      value: summary ? formatCompactNumber(language, summary.total_tokens) : "--",
      hint: summary ? `${formatCompactNumber(language, summary.input_tokens)} in · ${formatCompactNumber(language, summary.output_tokens)} out` : "--",
    },
    {
      label: t("预估费用"),
      value: summary ? formatCurrency(language, summary.estimated_cost) : "--",
      hint: t("按模型费率估算"),
    },
    {
      label: t("余额变化"),
      value: summary ? formatSigned(language, summary.balance_delta) : "--",
      hint: t("与费用视角分开展示"),
    },
    {
      label: t("额度变化"),
      value: summary ? formatSigned(language, summary.quota_delta) : "--",
      hint: t("适合 quota 型账户"),
    },
  ];

  return (
    <div className="dashboard-page stats-page">
      <div className="stats-header">
        <div>
          <div className="stats-title">{t("Token 与费用统计")}</div>
          <div className="stats-subtitle">{t("聚焦请求量、Token 消耗、预估费用与余额/额度变化。")}</div>
        </div>
        <div className="stats-filters">
          <Segmented
            options={[
              { label: "24h", value: 24 },
              { label: "7d", value: 168 },
              { label: "30d", value: 720 },
            ]}
            value={rangeHours}
            onChange={(value) => setRangeHours(value as RangeOption)}
          />
          <Select
            allowClear
            placeholder={t("全部账户")}
            className="stats-account-filter"
            value={accountID}
            onChange={(value) => setAccountID(value)}
            options={accounts.map((account) => ({ label: account.account_name, value: account.id }))}
          />
          <Input
            allowClear
            placeholder={t("筛选模型")}
            value={model}
            onChange={(event) => setModel(event.target.value)}
            className="stats-model-filter"
          />
        </div>
      </div>

      {loading ? (
        <div className="stats-loading">
          <Spin size="large" />
        </div>
      ) : error ? (
        <Card className="stats-panel">
          <div className="settings-empty">{error}</div>
        </Card>
      ) : (
        <>
          <div className="stats-summary-grid">
            {summaryCards.map((card) => (
              <Card key={card.label} className="stats-summary-card" variant="borderless">
                <div className="stats-card-label">{card.label}</div>
                <div className="stats-card-value">{card.value}</div>
                <div className="stats-card-hint">{card.hint}</div>
              </Card>
            ))}
          </div>

          <div className="stats-content-grid">
            <Card className="stats-panel" variant="borderless" title={t("Token 趋势")}>
              {trends.length === 0 ? (
                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t("暂无趋势数据")} />
              ) : (
                <div className="stats-trend-list">
                  {trends.map((point) => (
                    <div key={point.bucket} className="stats-trend-row">
                      <div className="stats-trend-meta">
                        <span>{new Date(point.bucket).toLocaleString(language, { month: "numeric", day: "numeric", hour: "2-digit", hour12: false })}</span>
                        <span>{formatCompactNumber(language, point.total_tokens)}</span>
                      </div>
                      <div className="stats-trend-bar-shell">
                        <div
                          className="stats-trend-bar"
                          style={{ width: `${Math.max((point.total_tokens / maxTrendTokens) * 100, 8)}%` }}
                        />
                      </div>
                      <div className="stats-trend-foot">
                        <span>{formatCurrency(language, point.estimated_cost)}</span>
                        <span>{point.request_count} {t("次请求")}</span>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </Card>

            <Card className="stats-panel" variant="borderless" title={t("状态分布")}>
              {statusSummary.length === 0 ? (
                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t("暂无状态数据")} />
              ) : (
                <div className="stats-status-list">
                  {statusSummary.map(([status, count]) => (
                    <div key={status} className="stats-status-row">
                      <Tag color={eventStatusColor(status)}>{status}</Tag>
                      <span>{count}</span>
                    </div>
                  ))}
                </div>
              )}
            </Card>
          </div>

          <Card className="stats-panel" variant="borderless" title={t("最近记录")}>
            {events.length === 0 ? (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t("暂无最近记录")} />
            ) : (
              <div className="stats-events-list">
                {events.map((event) => (
                  <div key={event.id} className="stats-event-row">
                    <div className="stats-event-main">
                      <div className="stats-event-title">
                        <span>{event.model}</span>
                        <Tag color={eventStatusColor(event.status)}>{event.status}</Tag>
                      </div>
                      <div className="stats-event-meta">
                        <span>#{event.account_id}</span>
                        <span>{new Date(event.created_at).toLocaleString(language, { hour12: false })}</span>
                        <span>{Math.round(event.latency_ms)} ms</span>
                      </div>
                    </div>
                    <div className="stats-event-metrics">
                      <span>{formatCompactNumber(language, event.total_tokens)} tok</span>
                      <span>{formatCurrency(language, event.estimated_cost)}</span>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </Card>
        </>
      )}
    </div>
  );
}
