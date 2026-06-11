import { fireEvent, render, screen, waitFor } from "@testing-library/react";

import { ServerUsersPage } from "./ServerUsersPage";
import { createServerUser, listServerUsers } from "../../lib/api";

vi.mock("../../lib/api", async () => {
  const actual = await vi.importActual<typeof import("../../lib/api")>("../../lib/api");
  return {
    ...actual,
    listServerUsers: vi.fn(),
    createServerUser: vi.fn(),
    rotateServerUserToken: vi.fn(),
    disableServerUser: vi.fn(),
  };
});

describe("ServerUsersPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(listServerUsers).mockResolvedValue([]);
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
});
