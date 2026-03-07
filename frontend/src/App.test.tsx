import { render, screen } from "@testing-library/react";

import { App } from "./App";

describe("App", () => {
  it("renders single-page dashboard shell", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn()
        .mockResolvedValueOnce(
          new Response(JSON.stringify([]), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        )
        .mockResolvedValueOnce(
          new Response(
            JSON.stringify({
              total_conversations: 0,
              active_conversations: 0,
              total_runs: 0,
              failover_runs: 0,
            }),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            },
          ),
        ),
    );

    render(<App />);

    expect(await screen.findByRole("heading", { name: "账户列表" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /添加账户/ })).toBeInTheDocument();
    expect(screen.getByText("会话统计")).toBeInTheDocument();
  });
});
