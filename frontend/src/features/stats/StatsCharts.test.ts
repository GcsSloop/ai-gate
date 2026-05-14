import { buildTokenTrendChartOption, sortTrendPointsByBucket, trendBucketsNeedHour } from "./StatsChartOptions";

describe("StatsCharts", () => {
  it("sorts trend points by bucket time ascending", () => {
    const points = [
      { bucket: "2026-04-08T08:00:00Z", request_count: 1, input_tokens: 80, output_tokens: 8, total_tokens: 88, estimated_cost: 0.08, balance_delta: 0, quota_delta: 0 },
      { bucket: "2026-03-17T08:00:00Z", request_count: 1, input_tokens: 17, output_tokens: 1, total_tokens: 18, estimated_cost: 0.01, balance_delta: 0, quota_delta: 0 },
      { bucket: "2026-04-02T08:00:00Z", request_count: 1, input_tokens: 42, output_tokens: 4, total_tokens: 46, estimated_cost: 0.04, balance_delta: 0, quota_delta: 0 },
      { bucket: "2026-03-25T08:00:00Z", request_count: 1, input_tokens: 25, output_tokens: 2, total_tokens: 27, estimated_cost: 0.02, balance_delta: 0, quota_delta: 0 },
    ];

    const sorted = sortTrendPointsByBucket(points);
    expect(sorted.map((item) => item.bucket)).toEqual([
      "2026-03-17T08:00:00Z",
      "2026-03-25T08:00:00Z",
      "2026-04-02T08:00:00Z",
      "2026-04-08T08:00:00Z",
    ]);
  });

  it("does not mutate original trend array", () => {
    const points = [
      { bucket: "2026-04-02T08:00:00Z", request_count: 1, input_tokens: 42, output_tokens: 4, total_tokens: 46, estimated_cost: 0.04, balance_delta: 0, quota_delta: 0 },
      { bucket: "2026-03-25T08:00:00Z", request_count: 1, input_tokens: 25, output_tokens: 2, total_tokens: 27, estimated_cost: 0.02, balance_delta: 0, quota_delta: 0 },
    ];
    const origin = points.map((item) => item.bucket);

    void sortTrendPointsByBucket(points);

    expect(points.map((item) => item.bucket)).toEqual(origin);
  });

  it("hides hour labels for day-level buckets", () => {
    const points = [
      { bucket: "2026-04-10T00:00:00Z", request_count: 1, input_tokens: 10, output_tokens: 1, total_tokens: 11, estimated_cost: 0.01, balance_delta: 0, quota_delta: 0 },
      { bucket: "2026-04-11T00:00:00Z", request_count: 1, input_tokens: 20, output_tokens: 2, total_tokens: 22, estimated_cost: 0.02, balance_delta: 0, quota_delta: 0 },
    ];
    expect(trendBucketsNeedHour(points)).toBe(false);
  });

  it("shows hour labels for hour-level buckets", () => {
    const points = [
      { bucket: "2026-04-11T08:00:00Z", request_count: 1, input_tokens: 20, output_tokens: 2, total_tokens: 22, estimated_cost: 0.02, balance_delta: 0, quota_delta: 0 },
      { bucket: "2026-04-11T09:00:00Z", request_count: 1, input_tokens: 30, output_tokens: 3, total_tokens: 33, estimated_cost: 0.03, balance_delta: 0, quota_delta: 0 },
    ];
    expect(trendBucketsNeedHour(points)).toBe(true);
  });

  it("adds request count to token trend chart with a separate axis", () => {
    const option = buildTokenTrendChartOption(
      [
        { bucket: "2026-04-11T08:00:00Z", request_count: 7, input_tokens: 20, output_tokens: 2, total_tokens: 22, estimated_cost: 0.03, balance_delta: 0, quota_delta: 0 },
      ],
      "zh-CN",
      {
        textPrimary: "#111827",
        textSecondary: "#6a7c73",
        border: "#e5e7eb",
        panelSurface: "#f8fbff",
      },
    );

    expect(option.legend).toMatchObject({ data: ["输入", "输出", "请求数"] });
    expect(option.yAxis).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ name: "Token" }),
        expect.objectContaining({ name: "请求数" }),
      ]),
    );
    expect(option.series).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ name: "请求数", type: "bar", yAxisIndex: 1, data: [7] }),
      ]),
    );
  });
});
