import { Card, Empty, Input, Select, Segmented, Spin, Tag } from "antd";
import { useEffect, useMemo, useRef, useState } from "react";

import {
  type AccountRecord,
  type DashboardRangeKey,
  type UsageDashboardSummary,
  type UsageModelDistributionPoint,
  type UsageEventRecord,
  type UsageTrendPoint,
  getDashboardModelDistribution,
  getDashboardRecentEvents,
  getDashboardSummary,
  getDashboardTrends,
  listAccounts,
} from "../../lib/api";
import type { AppLanguage, Translator } from "../../lib/i18n";
import { ModelDistributionChart, TokenTrendChart } from "./StatsCharts";

type StatsPageProps = {
  language: AppLanguage;
  t: Translator;
};

type RangeOption = DashboardRangeKey;

function formatCompactNumber(language: AppLanguage, value: number): string {
  const absolute = Math.abs(value);
  if (absolute >= 1_000_000) {
    return `${new Intl.NumberFormat(language, { maximumFractionDigits: absolute >= 10_000_000 ? 0 : 1 }).format(value / 1_000_000)}M`;
  }
  if (absolute >= 1_000) {
    return `${new Intl.NumberFormat(language, { maximumFractionDigits: absolute >= 10_000 ? 0 : 1 }).format(value / 1_000)}K`;
  }
  return new Intl.NumberFormat(language, { maximumFractionDigits: 0 }).format(value);
}

function formatCurrency(language: AppLanguage, value: number): string {
  return new Intl.NumberFormat(language, {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(value);
}

function eventStatusColor(status: string): string {
  if (status === "completed") {
    return "success";
  }
  if (status.includes("rate") || status.includes("usage")) {
    return "warning";
  }
  return "default";
}

export function StatsPage({ language, t }: StatsPageProps) {
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [rangeHours, setRangeHours] = useState<RangeOption>("24h");
  const [accountID, setAccountID] = useState<number | undefined>(undefined);
  const [model, setModel] = useState("");
  const [accounts, setAccounts] = useState<AccountRecord[]>([]);
  const [summary, setSummary] = useState<UsageDashboardSummary | null>(null);
  const [trends, setTrends] = useState<UsageTrendPoint[]>([]);
  const [modelDistribution, setModelDistribution] = useState<UsageModelDistributionPoint[]>([]);
  const [events, setEvents] = useState<UsageEventRecord[]>([]);
  const [error, setError] = useState<string | null>(null);
  const hasLoadedRef = useRef(false);

  useEffect(() => {
    let disposed = false;

    async function load() {
      if (hasLoadedRef.current) {
        setRefreshing(true);
      } else {
        setLoading(true);
      }
      setError(null);
      try {
        const [accountList, nextSummary, nextTrends, nextModelDistribution, nextEvents] = await Promise.all([
          listAccounts(),
          getDashboardSummary(rangeHours, accountID, model),
          getDashboardTrends(rangeHours, accountID, model),
          getDashboardModelDistribution(rangeHours, accountID, model),
          getDashboardRecentEvents(rangeHours, accountID, model, 20),
        ]);
        if (disposed) {
          return;
        }
        setAccounts(Array.isArray(accountList) ? accountList : []);
        setSummary(nextSummary);
        setTrends(Array.isArray(nextTrends) ? nextTrends : []);
        setModelDistribution(Array.isArray(nextModelDistribution) ? nextModelDistribution : []);
        setEvents(Array.isArray(nextEvents) ? nextEvents : []);
        hasLoadedRef.current = true;
      } catch (loadError) {
        if (!disposed) {
          setError(loadError instanceof Error ? t(loadError.message) : t("加载统计数据失败"));
        }
      } finally {
        if (!disposed) {
          setLoading(false);
          setRefreshing(false);
        }
      }
    }

    void load();
    return () => {
      disposed = true;
    };
  }, [accountID, model, rangeHours, t]);

  const accountNameByID = useMemo(() => {
    return new Map(accounts.map((account) => [account.id, account.account_name]));
  }, [accounts]);

  const totalTokens = summary?.total_tokens ?? 0;
  const inputShare = totalTokens > 0 ? (summary?.input_tokens ?? 0) / totalTokens : 0;
  const outputShare = totalTokens > 0 ? (summary?.output_tokens ?? 0) / totalTokens : 0;

  const summaryCards = [
    {
      label: t("请求数"),
      value: summary ? formatCompactNumber(language, summary.request_count) : "--",
      hint: summary ? `${summary.success_count} ${t("成功")} / ${summary.failure_count} ${t("失败")}` : "--",
    },
    {
      label: t("输入 Token"),
      value: summary ? formatCompactNumber(language, summary.input_tokens) : "--",
      hint: summary ? `${Math.round(inputShare * 100)}% ${t("占总 Token")}` : "--",
    },
    {
      label: t("输出 Token"),
      value: summary ? formatCompactNumber(language, summary.output_tokens) : "--",
      hint: summary ? `${Math.round(outputShare * 100)}% ${t("占总 Token")}` : "--",
    },
    {
      label: t("预估费用"),
      value: summary ? formatCurrency(language, summary.estimated_cost) : "--",
      hint: t("按模型费率估算"),
    },
  ];

  return (
    <div className="dashboard-page stats-page">
          <div className="stats-header">
            <div>
              <div className="stats-title">{t("Token 与费用统计")}</div>
              <div className="stats-subtitle">{t("聚焦请求量、输入输出 Token 与预估费用。")}</div>
            </div>
        <div className="stats-filters">
          <Segmented
            options={[
              { label: "24h", value: "24h" },
              { label: "7d", value: "7d" },
              { label: "30d", value: "30d" },
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
            <Card className={`stats-panel ${refreshing ? "stats-panel-refreshing" : ""}`} variant="borderless" title={t("Token 趋势")}>
              {trends.length === 0 ? (
                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t("暂无趋势数据")} />
              ) : (
                <TokenTrendChart data={trends} language={language} />
              )}
            </Card>

            <Card className={`stats-panel ${refreshing ? "stats-panel-refreshing" : ""}`} variant="borderless" title={t("模型分布")}>
              {modelDistribution.length === 0 ? (
                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t("暂无模型数据")} />
              ) : (
                <ModelDistributionChart data={modelDistribution} language={language} />
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
                        <span>{`${accountNameByID.get(event.account_id) ?? t("账户")} · #${event.account_id}`}</span>
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
