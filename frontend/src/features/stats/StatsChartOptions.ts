import type * as echarts from "echarts";

import type { AppLanguage } from "../../lib/i18n";
import type { UsageTrendPoint } from "../../lib/api";

export type ChartTheme = {
  textPrimary: string;
  textSecondary: string;
  border: string;
  panelSurface: string;
};

type TokenTrendChartOption = echarts.EChartsCoreOption & {
  legend: { data: string[] };
  yAxis: unknown[];
  series: unknown[];
};

export function sortTrendPointsByBucket(data: UsageTrendPoint[]): UsageTrendPoint[] {
  return [...data].sort((left, right) => {
    const leftTime = Date.parse(left.bucket);
    const rightTime = Date.parse(right.bucket);
    if (Number.isNaN(leftTime) || Number.isNaN(rightTime)) {
      return left.bucket.localeCompare(right.bucket);
    }
    return leftTime - rightTime;
  });
}

export function trendBucketsNeedHour(data: UsageTrendPoint[]): boolean {
  return data.some((point) => {
    const date = new Date(point.bucket);
    if (Number.isNaN(date.getTime())) {
      return false;
    }
    return date.getUTCHours() !== 0 || date.getUTCMinutes() !== 0 || date.getUTCSeconds() !== 0;
  });
}

export function formatCompactNumber(language: AppLanguage, value: number): string {
  const absolute = Math.abs(value);
  if (absolute >= 1_000_000) {
    return `${new Intl.NumberFormat(language, { maximumFractionDigits: absolute >= 10_000_000 ? 0 : 1 }).format(value / 1_000_000)}M`;
  }
  if (absolute >= 1_000) {
    return `${new Intl.NumberFormat(language, { maximumFractionDigits: absolute >= 10_000 ? 0 : 1 }).format(value / 1_000)}K`;
  }
  return new Intl.NumberFormat(language, { maximumFractionDigits: 0 }).format(value);
}

function formatBucket(language: AppLanguage, bucket: string, withHour: boolean): string {
  return new Date(bucket).toLocaleString(
    language,
    withHour
      ? {
          month: "numeric",
          day: "numeric",
          hour: "2-digit",
          minute: "2-digit",
          hour12: false,
        }
      : {
          month: "numeric",
          day: "numeric",
        },
  );
}

export function buildTokenTrendChartOption(data: UsageTrendPoint[], language: AppLanguage, theme: ChartTheme): TokenTrendChartOption {
  const sortedData = sortTrendPointsByBucket(data);
  const inputLabel = language === "zh-CN" ? "输入" : "Input";
  const outputLabel = language === "zh-CN" ? "输出" : "Output";
  const requestLabel = language === "zh-CN" ? "请求数" : "Requests";
  const withHour = trendBucketsNeedHour(sortedData);
  return {
    animationDuration: 420,
    animationDurationUpdate: 420,
    animationEasingUpdate: "cubicOut",
    color: ["#3b82f6", "#14b8a6", "#f59e0b"],
    grid: {
      top: 34,
      right: 46,
      bottom: 24,
      left: 12,
      containLabel: true,
    },
    legend: {
      top: 0,
      right: 0,
      icon: "circle",
      itemWidth: 8,
      itemHeight: 8,
      textStyle: {
        color: theme.textSecondary,
        fontSize: 12,
      },
      data: [inputLabel, outputLabel, requestLabel],
    },
    tooltip: {
      trigger: "axis",
      backgroundColor: theme.panelSurface,
      borderColor: theme.border,
      borderWidth: 1,
      textStyle: {
        color: theme.textPrimary,
      },
      formatter: (params: unknown) => {
        const series = Array.isArray(params) ? (params as Array<{ axisValue: string; seriesName: string; value: number }>) : [];
        if (series.length === 0) {
          return "";
        }
        const rows = series
          .map((item) => `${item.seriesName}: ${formatCompactNumber(language, Number(item.value ?? 0))}`)
          .join("<br/>");
        return `${series[0].axisValue}<br/>${rows}`;
      },
    },
    xAxis: {
      type: "category",
      boundaryGap: true,
      data: sortedData.map((point) => formatBucket(language, point.bucket, withHour)),
      axisLabel: {
        color: theme.textSecondary,
        fontSize: 11,
        margin: 14,
      },
      axisLine: {
        lineStyle: {
          color: theme.border,
        },
      },
      axisTick: {
        show: false,
      },
    },
    yAxis: [
      {
        type: "value",
        name: "Token",
        nameGap: 8,
        nameTextStyle: {
          color: theme.textSecondary,
          fontSize: 11,
          align: "left",
        },
        axisLabel: {
          color: theme.textSecondary,
          fontSize: 11,
          formatter: (value: number) => formatCompactNumber(language, value),
        },
        splitLine: {
          lineStyle: {
            color: theme.border,
            opacity: 0.7,
          },
        },
      },
      {
        type: "value",
        name: requestLabel,
        nameGap: 8,
        nameTextStyle: {
          color: theme.textSecondary,
          fontSize: 11,
          align: "right",
        },
        axisLabel: {
          color: theme.textSecondary,
          fontSize: 11,
          formatter: (value: number) => formatCompactNumber(language, value),
        },
        splitLine: {
          show: false,
        },
      },
    ],
    series: [
      {
        name: inputLabel,
        type: "line",
        smooth: true,
        symbol: "none",
        lineStyle: { width: 3 },
        areaStyle: {
          color: "rgba(59, 130, 246, 0.10)",
        },
        data: sortedData.map((point) => point.input_tokens),
      },
      {
        name: outputLabel,
        type: "line",
        smooth: true,
        symbol: "none",
        lineStyle: { width: 3 },
        areaStyle: {
          color: "rgba(20, 184, 166, 0.10)",
        },
        data: sortedData.map((point) => point.output_tokens),
      },
      {
        name: requestLabel,
        type: "bar",
        yAxisIndex: 1,
        barMaxWidth: 16,
        itemStyle: {
          borderRadius: [4, 4, 0, 0],
          opacity: 0.28,
        },
        emphasis: {
          itemStyle: {
            opacity: 0.55,
          },
        },
        data: sortedData.map((point) => point.request_count),
      },
    ],
  };
}
