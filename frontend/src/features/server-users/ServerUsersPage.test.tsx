import { fireEvent, render, screen, waitFor } from "@testing-library/react";

import { ServerUsersPage } from "./ServerUsersPage";
import { createServerUser, listServerUserAccounts, listServerUsers, setServerUserAccounts } from "../../lib/api";

vi.mock("../../lib/api", async () => {
  const actual = await vi.importActual<typeof import("../../lib/api")>("../../lib/api");
  return {
    ...actual,
    listServerUsers: vi.fn(),
    listServerUserAccounts: vi.fn(),
    setServerUserAccounts: vi.fn(),
    createServerUser: vi.fn(),
    rotateServerUserToken: vi.fn(),
    disableServerUser: vi.fn(),
  };
});

describe("ServerUsersPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(listServerUsers).mockResolvedValue([]);
    vi.mocked(listServerUserAccounts).mockResolvedValue([]);
    vi.mocked(setServerUserAccounts).mockResolvedValue(undefined);
    vi.mocked(createServerUser).mockResolvedValue({
      user: { id: 1, name: "alice", status: "active", created_at: "2026-06-12T00:00:00Z" },
      token: "agt_once",
    });
  });

  it("creates a server user and shows the issued token once", async () => {
    render(<ServerUsersPage t={(value) => value} />);
    await waitFor(() => expect(listServerUsers).toHaveBeenCalled());

    fireEvent.change(screen.getByPlaceholderText("用户名称"), { target: { value: "alice" } });
    fireEvent.click(screen.getByRole("button", { name: /新建用户/ }));

    await waitFor(() => expect(createServerUser).toHaveBeenCalledWith("alice"));
    expect(await screen.findByDisplayValue("agt_once")).toBeInTheDocument();
  });

  it("assigns upstream account pool to a server user", async () => {
    vi.mocked(listServerUsers).mockResolvedValue([
      { id: 1, name: "alice", username: "alice", role: "user", status: "active", created_at: "2026-06-12T00:00:00Z", assigned_accounts: 1 },
    ]);
    vi.mocked(listServerUserAccounts).mockResolvedValue([
      { account_id: 10, account_name: "pool-a", provider_type: "openai-compatible", source_icon: "openai", status: "active", assigned: true, position: 0, is_active: true, is_locked: false, supports_responses: true },
      { account_id: 11, account_name: "pool-b", provider_type: "openai-compatible", source_icon: "openai", status: "active", assigned: false, position: 0, is_active: false, is_locked: false, supports_responses: true },
    ]);

    render(<ServerUsersPage t={(value) => value} />);
    expect(await screen.findByText("alice")).toBeInTheDocument();
    expect(screen.getByText("1")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "分配账户" }));
    expect(await screen.findByText("pool-a")).toBeInTheDocument();
    fireEvent.click(screen.getByLabelText("pool-b"));
    fireEvent.click(screen.getByRole("button", { name: "保存" }));

    await waitFor(() => expect(setServerUserAccounts).toHaveBeenCalledWith(1, [10, 11]));
  });
});
