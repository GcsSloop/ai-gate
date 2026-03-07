import { render, screen } from "@testing-library/react";

import { MonitoringPage } from "./MonitoringPage";

describe("MonitoringPage", () => {
  it("renders aggregated monitoring data", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            status_counts: { active: 1, cooldown: 2 },
            totals: { balance: 15, quota: 1300 },
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      ),
    );

    render(<MonitoringPage />);

    expect(await screen.findByText("15")).toBeInTheDocument();
    expect(screen.getByText("2")).toBeInTheDocument();
  });
});
