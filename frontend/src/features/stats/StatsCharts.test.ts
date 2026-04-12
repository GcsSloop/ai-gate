import { sortTrendPointsByBucket } from "./StatsCharts";

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
});
