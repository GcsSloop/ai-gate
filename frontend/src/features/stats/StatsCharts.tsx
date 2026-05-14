import { useEffect, useMemo, useRef } from "react";
import * as echarts from "echarts";

import type { AppLanguage } from "../../lib/i18n";
import type { UsageModelDistributionPoint, UsageTrendPoint } from "../../lib/api";
import { buildTokenTrendChartOption, formatCompactNumber, type ChartTheme } from "./StatsChartOptions";

type TokenTrendChartProps = {
  data: UsageTrendPoint[];
  language: AppLanguage;
};

type ModelDistributionChartProps = {
  data: UsageModelDistributionPoint[];
  language: AppLanguage;
};

function readChartTheme(): ChartTheme {
  const styles = window.getComputedStyle(document.documentElement);
  return {
    textPrimary: styles.getPropertyValue("--text-primary").trim() || "#111827",
    textSecondary: styles.getPropertyValue("--text-secondary").trim() || "#6a7c73",
    border: styles.getPropertyValue("--panel-border").trim() || "#e5e7eb",
    panelSurface: styles.getPropertyValue("--panel-strong").trim() || "#f8fbff",
  };
}

function useEChart(option: echarts.EChartsCoreOption | null) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const chartRef = useRef<echarts.EChartsType | null>(null);

  useEffect(() => {
    if (import.meta.env.MODE === "test") {
      return;
    }
    if (!option || !containerRef.current) {
      return;
    }

    const chart = chartRef.current ?? echarts.getInstanceByDom(containerRef.current) ?? echarts.init(containerRef.current, undefined, { renderer: "canvas" });
    chartRef.current = chart;
    chart.setOption(option, { notMerge: false, replaceMerge: ["series"] });

    const resize = () => chart.resize();
    const resizeObserver = new ResizeObserver(resize);
    resizeObserver.observe(containerRef.current);
    window.addEventListener("resize", resize);

    return () => {
      resizeObserver.disconnect();
      window.removeEventListener("resize", resize);
    };
  }, [option]);

  useEffect(() => {
    return () => {
      chartRef.current?.dispose();
      chartRef.current = null;
    };
  }, []);

  return containerRef;
}

export function TokenTrendChart({ data, language }: TokenTrendChartProps) {
  const option = useMemo<echarts.EChartsCoreOption | null>(() => {
    if (data.length === 0) {
      return null;
    }
    const theme = readChartTheme();
    return buildTokenTrendChartOption(data, language, theme);
  }, [data, language]);

  const chartRef = useEChart(option);

  return <div ref={chartRef} className="stats-chart-shell stats-chart-shell-lg" data-testid="stats-token-trend-chart" />;
}

export function ModelDistributionChart({ data, language }: ModelDistributionChartProps) {
  const option = useMemo<echarts.EChartsCoreOption | null>(() => {
    if (data.length === 0) {
      return null;
    }
    const theme = readChartTheme();
    const total = data.reduce((sum, item) => sum + item.request_count, 0);
    const requestLabel = language === "zh-CN" ? "请求" : "Requests";
    return {
      animationDuration: 420,
      animationDurationUpdate: 420,
      animationEasingUpdate: "cubicOut",
      color: ["#3b82f6", "#14b8a6", "#f59e0b", "#8b5cf6", "#ef4444", "#06b6d4"],
      tooltip: {
        trigger: "item",
        backgroundColor: theme.panelSurface,
        borderColor: theme.border,
        borderWidth: 1,
        textStyle: {
          color: theme.textPrimary,
        },
        formatter: ({ name, value, percent }: { name: string; value: number; percent: number }) =>
          `${name}<br/>${formatCompactNumber(language, value)} 次请求 · ${percent}%`,
      },
      legend: {
        bottom: 0,
        left: "center",
        icon: "circle",
        itemWidth: 8,
        itemHeight: 8,
        textStyle: {
          color: theme.textSecondary,
          fontSize: 12,
        },
      },
      graphic: [
        {
          type: "text",
          left: "center",
          top: "40%",
          style: {
            text: formatCompactNumber(language, total),
            fill: theme.textPrimary,
            fontSize: 20,
            fontWeight: 700,
          },
        },
        {
          type: "text",
          left: "center",
          top: "52%",
          style: {
            text: requestLabel,
            fill: theme.textSecondary,
            fontSize: 11,
          },
        },
      ],
      series: [
        {
          type: "pie",
          radius: ["54%", "76%"],
          center: ["50%", "42%"],
          avoidLabelOverlap: true,
          padAngle: 1.5,
          itemStyle: {
            borderColor: theme.panelSurface,
            borderWidth: 2,
            borderRadius: 8,
          },
          label: { show: false },
          labelLine: { show: false },
          data: data.map((item) => ({
            name: item.model,
            value: item.request_count,
          })),
        },
      ],
    };
  }, [data, language]);

  const chartRef = useEChart(option);

  return <div ref={chartRef} className="stats-chart-shell stats-chart-shell-sm" data-testid="stats-model-distribution-chart" />;
}
