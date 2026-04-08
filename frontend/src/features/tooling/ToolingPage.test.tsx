import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { Modal } from "antd";
import type { ReactNode } from "react";

import { ToolingPage } from "./ToolingPage";
import * as api from "../../lib/api";

vi.mock("../../lib/api", async () => {
  const actual = await vi.importActual<typeof import("../../lib/api")>("../../lib/api");
  return {
    ...actual,
    getToolingState: vi.fn(),
    importToolingSkills: vi.fn(),
    updateToolingSkill: vi.fn(),
    deleteToolingSkill: vi.fn(),
    searchToolingRepos: vi.fn(),
    addToolingRepo: vi.fn(),
    removeToolingRepo: vi.fn(),
    importToolingMcpServers: vi.fn(),
    applyToolingMcpServer: vi.fn(),
    installToolingMcpServer: vi.fn(),
    deleteToolingMcpServer: vi.fn(),
  };
});

function buildToolingState(): api.ToolingState {
  return {
    skill_sync_method: "symlink",
    clients: [
      {
        app: "codex",
        label: "Codex",
        skills_dir: "/tmp/home/.codex/skills",
        mcp_path: "/tmp/home/.codex/config.toml",
        skills_count: 2,
        mcp_status: "ready",
      },
    ],
    skill_stats: {
      total: 2,
      by_source: {
        codex: 2,
      },
    },
    skill_repos: [
      {
        owner: "openai",
        name: "codex-superpowers",
        branch: "main",
        enabled: true,
        skill_count: 12,
      },
    ],
    installed_skills: [
      {
        name: "superpowers",
        description: "包含 2 个技能",
        directory: "/tmp/aigate/tooling/skills/superpowers",
        source_client: "codex",
        source_kind: "codex",
        managed_path: "/tmp/aigate/tooling/skills/superpowers",
        installed_apps: { codex: true },
      },
      {
        name: "imagegen",
        description: "Generate or edit raster images.",
        directory: "/tmp/aigate/tooling/skills/imagegen",
        source_client: "codex",
        source_kind: "codex",
        managed_path: "/tmp/aigate/tooling/skills/imagegen",
        installed_apps: { codex: false },
      },
    ],
    repo_search_results: [
      {
        owner: "gcssloop",
        name: "codex-router-skills",
        branch: "main",
        url: "https://example.invalid/repo",
        description: "Test repo",
      },
    ],
    discovered_mcp_servers: [
      {
        id: "fetch",
        name: "Fetch Server",
        description: "Fetch MCP",
        source_apps: { codex: true },
        client_status: { codex: "enabled" },
        spec: { type: "stdio", command: "uvx" },
      },
    ],
    mcp_templates: [
      {
        id: "fetch",
        name: "mcp-server-fetch",
        description: "Quick template: mcp-fetch",
        type: "stdio",
        command: "uvx",
        args: ["mcp-server-fetch"],
      },
    ],
    mcp_servers: [
      {
        id: "fetch",
        name: "Fetch Server",
        description: "Fetch MCP",
        enabled_apps: { codex: true },
        client_status: { codex: "enabled" },
        spec: { type: "stdio", command: "node", args: ["/tmp/home/.codex/mcp/fetch/bin/server.js"] },
        delete_allowed: true,
        delete_targets: ["/tmp/home/.codex/mcp/fetch"],
      },
      {
        id: "time",
        name: "Time Server",
        description: "Time MCP",
        template_id: "time",
        enabled_apps: { codex: false },
        client_status: { codex: "disabled" },
        spec: { type: "stdio", command: "npx" },
        delete_allowed: false,
        delete_reason: "该 MCP 由 AI Gate 提供，需在来源侧管理。",
      },
    ],
  };
}

function createDeferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe("ToolingPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.getToolingState).mockResolvedValue(buildToolingState());
    vi.mocked(api.importToolingSkills).mockResolvedValue({ imported: 1 });
    vi.mocked(api.updateToolingSkill).mockResolvedValue({ applied: 1, enabled: true, skill_sync_method: "symlink" });
    vi.mocked(api.deleteToolingSkill).mockResolvedValue();
    vi.mocked(api.searchToolingRepos).mockResolvedValue({ items: [] });
    vi.mocked(api.addToolingRepo).mockResolvedValue({
      owner: "openai",
      name: "codex-superpowers",
      branch: "main",
      enabled: true,
      skill_count: 12,
    });
    vi.mocked(api.removeToolingRepo).mockResolvedValue();
    vi.mocked(api.importToolingMcpServers).mockResolvedValue({ imported: 1 });
    vi.mocked(api.applyToolingMcpServer).mockResolvedValue();
    vi.mocked(api.installToolingMcpServer).mockResolvedValue(buildToolingState().mcp_servers[0]);
    vi.mocked(api.deleteToolingMcpServer).mockResolvedValue();
  });

  it("renders the minimal skills management layout", async () => {
    render(<ToolingPage mode="skills" t={(text) => text} />);

    expect(await screen.findByText("Skill 技能")).toBeInTheDocument();
    expect(screen.getByText("Codex 2")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /导入已有/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /发现技能/ })).toBeInTheDocument();
    expect(screen.getByText("superpowers")).toBeInTheDocument();
    expect(screen.getByText("包含 2 个技能")).toBeInTheDocument();
    expect(screen.getByLabelText("Codex 已激活")).toBeInTheDocument();
    expect(screen.getByLabelText("Codex 未激活")).toBeInTheDocument();
    expect(screen.queryByText("Skills 管理")).not.toBeInTheDocument();
    expect(screen.queryByText("管理 Codex 技能集合。")).not.toBeInTheDocument();
    expect(screen.queryByText("来源统计")).not.toBeInTheDocument();
    expect(screen.queryByText("当前安装技能列表")).not.toBeInTheDocument();
    expect(screen.queryByPlaceholderText("搜索技能")).not.toBeInTheDocument();
    expect(screen.queryByText("仓库管理")).not.toBeInTheDocument();
  });

  it("imports, toggles, and deletes skill collections", async () => {
    const confirmSpy = vi.spyOn(Modal, "confirm").mockImplementation((config) => {
      void config.onOk?.();
      return {
        destroy: vi.fn(),
        update: vi.fn(),
      } as ReturnType<typeof Modal.confirm>;
    });

    render(<ToolingPage mode="skills" t={(text) => text} />);

    await screen.findByText("Skill 技能");
    fireEvent.click(screen.getByRole("button", { name: /导入已有/ }));
    await waitFor(() => expect(api.importToolingSkills).toHaveBeenCalledWith("codex"));

    fireEvent.click(screen.getByRole("button", { name: "启用 imagegen 到 Codex" }));
    await waitFor(() =>
      expect(api.updateToolingSkill).toHaveBeenCalledWith("imagegen", {
        apps: ["codex"],
        enabled: true,
      }),
    );

    fireEvent.click(screen.getByRole("button", { name: "删除 superpowers" }));
    await waitFor(() => expect(confirmSpy).toHaveBeenCalled());
    await waitFor(() => expect(api.deleteToolingSkill).toHaveBeenCalledWith("superpowers"));
  });

  it("uses the managed collection directory when toggling and deleting skills", async () => {
    vi.mocked(api.getToolingState).mockResolvedValue({
      ...buildToolingState(),
      installed_skills: [
        {
          name: "Superpowers Collection",
          description: "包含 2 个技能",
          directory: "/tmp/aigate/tooling/skills/superpowers",
          source_client: "codex",
          source_kind: "codex",
          managed_path: "/tmp/aigate/tooling/skills/superpowers",
          installed_apps: { codex: true },
        },
      ],
    });

    const confirmSpy = vi.spyOn(Modal, "confirm").mockImplementation((config) => {
      void config.onOk?.();
      return {
        destroy: vi.fn(),
        update: vi.fn(),
      } as ReturnType<typeof Modal.confirm>;
    });

    render(<ToolingPage mode="skills" t={(text) => text} />);

    await screen.findByText("Superpowers Collection");
    fireEvent.click(screen.getByRole("button", { name: "停用 Superpowers Collection 于 Codex" }));
    await waitFor(() =>
      expect(api.updateToolingSkill).toHaveBeenCalledWith("superpowers", {
        apps: ["codex"],
        enabled: false,
      }),
    );

    fireEvent.click(screen.getByRole("button", { name: "删除 Superpowers Collection" }));
    await waitFor(() => expect(confirmSpy).toHaveBeenCalled());
    await waitFor(() => expect(api.deleteToolingSkill).toHaveBeenCalledWith("superpowers"));
  });

  it("shows skills discovery in a modal", async () => {
    render(<ToolingPage mode="skills" t={(text) => text} />);

    await screen.findByText("Skill 技能");
    fireEvent.click(screen.getByRole("button", { name: /发现技能/ }));

    const dialog = await screen.findByRole("dialog", { name: "发现技能" });
    expect(within(dialog).getByText("仓库搜索")).toBeInTheDocument();
    expect(within(dialog).getByText("仓库管理")).toBeInTheDocument();
    expect(within(dialog).getByText("openai/codex-superpowers")).toBeInTheDocument();
  });

  it("renders the minimal mcp management layout and toggles codex sync", async () => {
    render(<ToolingPage mode="mcp" t={(text) => text} />);

    expect(await screen.findByText("MCP 服务")).toBeInTheDocument();
    expect(screen.getByText("Codex 2")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /导入已有/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /发现服务/ })).not.toBeInTheDocument();
    expect(screen.getByText("Fetch Server")).toBeInTheDocument();
    expect(screen.getAllByLabelText("Codex 已激活").length).toBeGreaterThan(0);
    expect(screen.getByLabelText("Codex 未激活")).toBeInTheDocument();
    expect(screen.queryByText("MCP 管理")).not.toBeInTheDocument();
    expect(screen.queryByText("管理 Codex MCP 服务。")).not.toBeInTheDocument();
    expect(screen.queryByText("来源统计")).not.toBeInTheDocument();
    expect(screen.queryByText("当前安装服务列表")).not.toBeInTheDocument();
    expect(screen.queryByText("快速模板")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "停用 Fetch Server 于 Codex" }));

    await waitFor(() =>
      expect(api.applyToolingMcpServer).toHaveBeenCalledWith("fetch", ["codex"], false),
    );
  });

  it("confirms mcp deletion with local cleanup checked by default", async () => {
    let cleanupContent: ReactNode;
    const confirmSpy = vi.spyOn(Modal, "confirm").mockImplementation((config) => {
      cleanupContent = config.content;
      void config.onOk?.();
      return {
        destroy: vi.fn(),
        update: vi.fn(),
      } as ReturnType<typeof Modal.confirm>;
    });

    render(<ToolingPage mode="mcp" t={(text) => text} />);

    await screen.findByText("MCP 服务");
    fireEvent.click(screen.getByRole("button", { name: "删除 Fetch Server" }));

    await waitFor(() => expect(confirmSpy).toHaveBeenCalled());
    render(<>{cleanupContent}</>);
    expect(screen.getByText("将同时从 AI Gate 与 Codex 删除 Fetch Server")).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "同步清理本地文件" })).toBeChecked();
    expect(screen.getByText("以下本地路径将被删除")).toBeInTheDocument();
    const deleteTarget = screen.getByText("/tmp/home/.codex/mcp/fetch");
    expect(deleteTarget).toBeInTheDocument();
    expect(deleteTarget).toHaveClass("tooling-delete-target-path");
    await waitFor(() => expect(api.deleteToolingMcpServer).toHaveBeenCalledWith("fetch", true));
  });

  it("blocks deleting non-removable mcp servers and explains why", async () => {
    const warningSpy = vi.spyOn(Modal, "warning").mockImplementation(() => ({
      destroy: vi.fn(),
      update: vi.fn(),
    }) as ReturnType<typeof Modal.warning>);

    render(<ToolingPage mode="mcp" t={(text) => text} />);

    await screen.findByText("MCP 服务");
    fireEvent.click(screen.getByRole("button", { name: "删除 Time Server" }));

    await waitFor(() => expect(warningSpy).toHaveBeenCalled());
    expect(api.deleteToolingMcpServer).not.toHaveBeenCalled();
  });

  it("keeps the current content visible while a toggle refresh is pending", async () => {
    const initialState = buildToolingState();
    const refreshDeferred = createDeferred<api.ToolingState>();
    vi.mocked(api.getToolingState)
      .mockResolvedValueOnce(initialState)
      .mockImplementationOnce(() => refreshDeferred.promise);

    render(<ToolingPage mode="skills" t={(text) => text} />);

    await screen.findByText("imagegen");
    fireEvent.click(screen.getByRole("button", { name: "启用 imagegen 到 Codex" }));

    await waitFor(() =>
      expect(api.updateToolingSkill).toHaveBeenCalledWith("imagegen", {
        apps: ["codex"],
        enabled: true,
      }),
    );

    expect(screen.getByText("Skill 技能")).toBeInTheDocument();
    expect(screen.getByText("imagegen")).toBeInTheDocument();

    refreshDeferred.resolve(initialState);
    await screen.findByText("imagegen");
  });
});
