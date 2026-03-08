import { render, screen } from "@testing-library/react";

import { App } from "./App";

describe("App", () => {
  it("renders single-page dashboard shell", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify([]), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );

    render(<App />);

    expect(await screen.findByRole("heading", { name: "账户列表" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /添加账户/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "设置" })).toBeInTheDocument();
    expect(screen.getByText("开启代理")).toBeInTheDocument();
    expect(screen.getByText("aigate")).toBeInTheDocument();
  });
});
