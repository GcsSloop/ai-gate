import {
  CheckCircleOutlined,
  CloudDownloadOutlined,
  DeleteOutlined,
  DragOutlined,
  LinkOutlined,
  PlusOutlined,
  ReloadOutlined,
  SearchOutlined,
} from "@ant-design/icons";
import { Button, Card, Checkbox, Empty, Input, Modal, Select, Space, Spin, Typography, message } from "antd";
import { useEffect, useMemo, useRef, useState, type DragEvent } from "react";

import {
  addToolingRepo,
  applyToolingMcpServer,
  deleteToolingMcpServer,
  deleteToolingSkill,
  getToolingDiscoveredSkills,
  getToolingState,
  importToolingMcpServers,
  importToolingSkills,
  installToolingDiscoveredSkill,
  removeToolingRepo,
  resolveToolingRepo,
  refreshToolingDiscoveredSkills,
  reorderToolingRepos,
  updateToolingSkill,
  updateToolingRepo,
  type ToolingDiscoveredSkill,
  type ToolingMcpServer,
  type ToolingResolvedRepo,
  type ToolingSkillRecord,
  type ToolingSkillRepo,
  type ToolingSkillDiscoveryResponse,
  type ToolingState,
} from "../../lib/api";
import sourceOpenAIIcon from "../../assets/providers/openai.png";
import { openExternalUrl } from "../../lib/desktop-shell";

type ToolingMode = "skills" | "mcp";

type ToolingPageProps = {
  mode: ToolingMode;
  t: (text: string) => string;
};

type ManagedClientDescriptor = {
  id: string;
  label: string;
  icon: string;
};

type ManagedClientStatus = ManagedClientDescriptor & {
  enabled: boolean;
};

type RepoEditorMode = "create" | "edit";

type RepoEditorForm = {
  input: string;
  platform: "github" | "gitlab";
  owner: string;
  name: string;
  branch: string;
  branchOptions: string[];
};

const managedClients: ManagedClientDescriptor[] = [
  {
    id: "codex",
    label: "Codex",
    icon: sourceOpenAIIcon,
  },
];

export function ToolingPage({ mode, t }: ToolingPageProps) {
  const [messageApi, contextHolder] = message.useMessage();
  const [loading, setLoading] = useState(true);
  const [state, setState] = useState<ToolingState | null>(null);
  const [skillDiscoverOpen, setSkillDiscoverOpen] = useState(false);
  const [repoManagerOpen, setRepoManagerOpen] = useState(false);
  const [discoveryQuery, setDiscoveryQuery] = useState("");
  const [discoveryResponse, setDiscoveryResponse] = useState<ToolingSkillDiscoveryResponse | null>(null);
  const [discoveryLoading, setDiscoveryLoading] = useState(false);
  const [discoveryRefreshing, setDiscoveryRefreshing] = useState(false);
  const [discoveryStatus, setDiscoveryStatus] = useState("");
  const [discoveryBusyId, setDiscoveryBusyId] = useState<string | null>(null);
  const [importBusy, setImportBusy] = useState(false);
  const [repoBusyKey, setRepoBusyKey] = useState<string | null>(null);
  const [repoEditorOpen, setRepoEditorOpen] = useState(false);
  const [repoEditorMode, setRepoEditorMode] = useState<RepoEditorMode>("create");
  const [repoEditing, setRepoEditing] = useState<ToolingSkillRepo | null>(null);
  const [repoDragKey, setRepoDragKey] = useState<string | null>(null);
  const [repoOrdering, setRepoOrdering] = useState(false);
  const [repoForm, setRepoForm] = useState<RepoEditorForm>(() => createEmptyRepoForm());
  const [repoResolving, setRepoResolving] = useState(false);
  const [repoResolveError, setRepoResolveError] = useState("");
  const repoResolveRequestRef = useRef(0);
  const [skillBusyName, setSkillBusyName] = useState<string | null>(null);
  const [mcpBusyId, setMcpBusyId] = useState<string | null>(null);

  async function reload(options?: { background?: boolean }) {
    const background = options?.background ?? false;
    if (!background) {
      setLoading(true);
    }
    try {
      const next = await getToolingState();
      setState(next);
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : t("加载技能与 MCP 管理失败"));
    } finally {
      if (!background) {
        setLoading(false);
      }
    }
  }

  useEffect(() => {
    void reload();
  }, []);

  useEffect(() => {
    if (!skillDiscoverOpen || mode !== "skills") {
      return;
    }
    let active = true;
    setDiscoveryLoading(true);
    void getToolingDiscoveredSkills()
      .then((response) => {
        if (!active) {
          return;
        }
        setDiscoveryResponse(sortDiscoveryResponse(response));
        setDiscoveryStatus(response.cached ? t("使用缓存") : t("最新索引"));
      })
      .catch((error) => {
        if (!active) {
          return;
        }
        void messageApi.error(error instanceof Error ? error.message : t("加载发现技能失败"));
      })
      .finally(() => {
        if (active) {
          setDiscoveryLoading(false);
        }
      });

    setDiscoveryRefreshing(true);
    void refreshToolingDiscoveredSkills()
      .then((response) => {
        if (!active) {
          return;
        }
        setDiscoveryResponse(sortDiscoveryResponse(response));
        setDiscoveryStatus(t("已更新"));
        void reload({ background: true });
      })
      .catch((error) => {
        if (!active) {
          return;
        }
        if (!discoveryResponse) {
          void messageApi.error(error instanceof Error ? error.message : t("刷新发现技能失败"));
        }
      })
      .finally(() => {
        if (active) {
          setDiscoveryRefreshing(false);
        }
      });

    return () => {
      active = false;
    };
  }, [skillDiscoverOpen, mode]);

  const skillCount = useMemo(
    () => (state?.installed_skills ?? []).reduce((count, skill) => count + (skill.installed_apps?.codex ? 1 : 0), 0),
    [state?.installed_skills],
  );
  const mcpCount = state?.mcp_servers.length ?? 0;
  const skills = useMemo(() => state?.installed_skills ?? [], [state?.installed_skills]);
  const skillRepos = useMemo(() => state?.skill_repos ?? [], [state?.skill_repos]);
  const repoRanks = useMemo(() => {
    const index = new Map<string, number>();
    skillRepos.forEach((repo, order) => {
      index.set(repoRecordKey(repo.platform ?? "github", repo.owner, repo.name), order);
    });
    return index;
  }, [skillRepos]);
  const mcpServers = useMemo(() => state?.mcp_servers ?? [], [state?.mcp_servers]);
  const discoveredSkills = useMemo(() => {
    const items = [...(discoveryResponse?.items ?? [])].sort((a, b) => {
      const leftRank = repoRanks.get(repoRecordKey(a.platform, a.repo_owner, a.repo_name)) ?? Number.MAX_SAFE_INTEGER;
      const rightRank = repoRanks.get(repoRecordKey(b.platform, b.repo_owner, b.repo_name)) ?? Number.MAX_SAFE_INTEGER;
      if (leftRank !== rightRank) {
        return leftRank - rightRank;
      }
      return a.name.localeCompare(b.name);
    });
    const query = discoveryQuery.trim().toLowerCase();
    if (!query) {
      return items;
    }
    return items.filter((item) =>
      [item.name, item.description ?? "", item.repo_owner, item.repo_name, item.source_path].join(" ").toLowerCase().includes(query),
    );
  }, [discoveryQuery, discoveryResponse?.items, repoRanks]);

  async function handleImportSkills() {
    setImportBusy(true);
    try {
      await importToolingSkills("codex");
      void messageApi.success(t("导入完成"));
      await reload({ background: true });
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : t("导入失败"));
    } finally {
      setImportBusy(false);
    }
  }

  async function handleSkillToggle(skill: ToolingSkillRecord) {
    const enabled = !skill.installed_apps?.codex;
    const collectionName = skillCollectionName(skill);
    setSkillBusyName(skill.name);
    try {
      await updateToolingSkill(collectionName, { apps: ["codex"], enabled });
      void messageApi.success(enabled ? t("已启用到 Codex") : t("已从 Codex 停用"));
      await reload({ background: true });
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : t("技能同步失败"));
    } finally {
      setSkillBusyName(null);
    }
  }

  function confirmDeleteSkill(skill: ToolingSkillRecord) {
    Modal.confirm({
      title: t("确认删除技能"),
      content: t(`将同时从 AI Gate 与 Codex 删除 ${skill.name}`),
      okText: t("删除"),
      cancelText: t("取消"),
      okButtonProps: { danger: true },
      onOk: async () => {
        const collectionName = skillCollectionName(skill);
        setSkillBusyName(skill.name);
        try {
          await deleteToolingSkill(collectionName);
          void messageApi.success(t("删除成功"));
          await reload({ background: true });
        } catch (error) {
          void messageApi.error(error instanceof Error ? error.message : t("删除失败"));
        } finally {
          setSkillBusyName(null);
        }
      },
    });
  }

  async function handleImportMcp() {
    setImportBusy(true);
    try {
      await importToolingMcpServers("codex");
      void messageApi.success(t("导入完成"));
      await reload({ background: true });
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : t("导入失败"));
    } finally {
      setImportBusy(false);
    }
  }

  async function handleMcpToggle(server: ToolingMcpServer) {
    const enabled = !server.enabled_apps?.codex;
    setMcpBusyId(server.id);
    try {
      await applyToolingMcpServer(server.id, ["codex"], enabled);
      void messageApi.success(enabled ? t("已启用到 Codex") : t("已从 Codex 停用"));
      await reload({ background: true });
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : t("MCP 同步失败"));
    } finally {
      setMcpBusyId(null);
    }
  }

  function confirmDeleteMcp(server: ToolingMcpServer) {
    if (server.delete_allowed === false) {
      Modal.warning({
        title: t("无法删除 MCP"),
        content: server.delete_reason || t("该 MCP 不能在这里删除。"),
        okText: t("知道了"),
        centered: true,
      });
      return;
    }
    let cleanupLocalFiles = true;
    Modal.confirm({
      title: t("确认删除 MCP"),
      content: (
        <DeleteMcpConfirmContent
          server={server}
          t={t}
          onCleanupChange={(next) => {
            cleanupLocalFiles = next;
          }}
        />
      ),
      okText: t("删除"),
      cancelText: t("取消"),
      centered: true,
      okButtonProps: { danger: true },
      onOk: async () => {
        setMcpBusyId(server.id);
        try {
          await deleteToolingMcpServer(server.id, cleanupLocalFiles);
          void messageApi.success(t("删除成功"));
          await reload({ background: true });
        } catch (error) {
          void messageApi.error(error instanceof Error ? error.message : t("删除失败"));
        } finally {
          setMcpBusyId(null);
        }
      },
    });
  }

  async function handleDiscoveryRefresh() {
    setDiscoveryLoading(true);
    try {
      const response = await refreshToolingDiscoveredSkills();
      setDiscoveryResponse(sortDiscoveryResponse(response));
      setDiscoveryStatus(t("已更新"));
      await reload({ background: true });
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : t("刷新发现技能失败"));
    } finally {
      setDiscoveryLoading(false);
    }
  }

  async function handleDiscoveredSkillInstall(skill: ToolingDiscoveredSkill) {
    setDiscoveryBusyId(skill.id);
    try {
      await installToolingDiscoveredSkill({ id: skill.id, apps: ["codex"] });
      const nextAction = skill.update_available ? t("已更新") : t("安装成功");
      setDiscoveryResponse((current) => markDiscoveredSkillInstalled(current, skill.id));
      setDiscoveryStatus(skill.update_available ? t("已更新") : t("已安装"));
      void messageApi.success(nextAction);
      void reload({ background: true });
      void refreshToolingDiscoveredSkills()
        .then((response) => {
          setDiscoveryResponse(sortDiscoveryResponse(response));
          setDiscoveryStatus(t("已更新"));
        })
        .catch((error) => {
          void messageApi.error(error instanceof Error ? error.message : t("刷新发现技能失败"));
        });
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : t("安装失败"));
    } finally {
      setDiscoveryBusyId(null);
    }
  }

  function handleViewDiscoveredSkill(skill: ToolingDiscoveredSkill) {
    void openExternalUrl(skill.source_url);
  }

  async function resolveRepoInput(input: string, options?: { initialBranch?: string }) {
    const trimmed = input.trim();
    setRepoResolveError("");
    if (!trimmed) {
      repoResolveRequestRef.current += 1;
      setRepoResolving(false);
      setRepoForm((current) => ({
        ...createEmptyRepoForm(),
        input: current.input,
      }));
      return;
    }
    const requestId = repoResolveRequestRef.current + 1;
    repoResolveRequestRef.current = requestId;
    setRepoResolving(true);
    try {
      const resolved = await resolveToolingRepo(trimmed);
      if (repoResolveRequestRef.current !== requestId) {
        return;
      }
      setRepoForm((current) => applyResolvedRepo(current, resolved, options?.initialBranch));
    } catch (error) {
      if (repoResolveRequestRef.current !== requestId) {
        return;
      }
      setRepoResolveError(error instanceof Error ? error.message : t("仓库解析失败"));
    } finally {
      if (repoResolveRequestRef.current === requestId) {
        setRepoResolving(false);
      }
    }
  }

  async function handleRepoAdd() {
    const key = repoRecordKey(repoForm.platform, repoForm.owner, repoForm.name);
    setRepoBusyKey(key);
    try {
      await addToolingRepo(repoForm.platform, repoForm.owner, repoForm.name, repoForm.branch);
      void messageApi.success(t("仓库已添加"));
      resetRepoForm();
      await reload({ background: true });
      const response = await refreshToolingDiscoveredSkills();
      setDiscoveryResponse(sortDiscoveryResponse(response));
      setDiscoveryStatus(t("已更新"));
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : t("添加失败"));
    } finally {
      setRepoBusyKey(null);
    }
  }

  async function handleRepoSave() {
    if (repoEditorMode === "create" || !repoEditing) {
      await handleRepoAdd();
      return;
    }
    const key = repoRecordKey(repoEditing.platform ?? "github", repoEditing.owner, repoEditing.name);
    setRepoBusyKey(key);
    try {
      await updateToolingRepo(repoEditing.platform ?? "github", repoEditing.owner, repoEditing.name, {
        platform: repoForm.platform,
        owner: repoForm.owner,
        name: repoForm.name,
        branch: repoForm.branch,
      });
      void messageApi.success(t("仓库已更新"));
      resetRepoForm();
      await reload({ background: true });
      const response = await refreshToolingDiscoveredSkills();
      setDiscoveryResponse(sortDiscoveryResponse(response));
      setDiscoveryStatus(t("已更新"));
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : t("更新失败"));
    } finally {
      setRepoBusyKey(null);
    }
  }

  function openRepoCreate() {
    setRepoEditorMode("create");
    setRepoEditing(null);
    setRepoResolveError("");
    setRepoResolving(false);
    setRepoForm(createEmptyRepoForm());
    setRepoEditorOpen(true);
  }

  function openRepoEdit(repo: ToolingSkillRepo) {
    const repoUrl = buildRepoURL(repo.platform ?? "github", repo.owner, repo.name);
    const nextForm: RepoEditorForm = {
      input: repoUrl,
      platform: repo.platform ?? "github",
      owner: repo.owner,
      name: repo.name,
      branch: repo.branch,
      branchOptions: [repo.branch],
    };
    repoResolveRequestRef.current += 1;
    setRepoEditorMode("edit");
    setRepoEditing(repo);
    setRepoResolveError("");
    setRepoResolving(false);
    setRepoForm(nextForm);
    setRepoEditorOpen(true);
    void resolveRepoInput(repoUrl, { initialBranch: repo.branch });
  }

  function resetRepoForm() {
    repoResolveRequestRef.current += 1;
    setRepoEditing(null);
    setRepoEditorMode("create");
    setRepoEditorOpen(false);
    setRepoResolving(false);
    setRepoResolveError("");
    setRepoForm(createEmptyRepoForm());
  }

  async function handleRepoRemove(repo: ToolingSkillRepo) {
    const key = repoRecordKey(repo.platform ?? "github", repo.owner, repo.name);
    setRepoBusyKey(key);
    try {
      await removeToolingRepo(repo.platform ?? "github", repo.owner, repo.name);
      void messageApi.success(t("仓库已删除"));
      await reload({ background: true });
      const response = await refreshToolingDiscoveredSkills();
      setDiscoveryResponse(sortDiscoveryResponse(response));
      setDiscoveryStatus(t("已更新"));
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : t("删除失败"));
    } finally {
      setRepoBusyKey(null);
    }
  }

  function handleRepoView(repo: ToolingSkillRepo) {
    void openExternalUrl(buildRepoURL(repo.platform ?? "github", repo.owner, repo.name));
  }

  async function handleRepoReorder(targetRepo: ToolingSkillRepo) {
    if (!repoDragKey) {
      return;
    }
    const targetKey = repoRecordKey(targetRepo.platform ?? "github", targetRepo.owner, targetRepo.name);
    const reordered = reorderReposByKey(skillRepos, repoDragKey, targetKey);
    if (reordered === skillRepos) {
      return;
    }
    setState((current) => (current ? { ...current, skill_repos: reordered } : current));
    setRepoOrdering(true);
    try {
      const next = await reorderToolingRepos(
        reordered.map((repo) => ({
          platform: repo.platform ?? "github",
          owner: repo.owner,
          name: repo.name,
        })),
      );
      setState((current) => (current ? { ...current, skill_repos: next } : current));
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : t("仓库排序失败"));
      await reload({ background: true });
    } finally {
      setRepoOrdering(false);
      setRepoDragKey(null);
    }
  }

  if (loading || !state) {
    return (
      <div className="dashboard-page tooling-page tooling-loading">
        {contextHolder}
        <Spin size="large" />
      </div>
    );
  }

  return (
    <div className="dashboard-page tooling-page tooling-minimal-page">
      {contextHolder}
      <Card className="tooling-card tooling-minimal-shell">
        <div className="tooling-minimal-toolbar">
          <div className="tooling-minimal-titlebar">
            <div className="tooling-inline-title">
              {mode === "skills" ? t("Skill 技能") : t("MCP 服务")}
            </div>
            <div className="tooling-inline-count">{`Codex ${mode === "skills" ? skillCount : mcpCount}`}</div>
          </div>
          <Space className="tooling-minimal-actions" wrap size={10}>
            <Button
              type="text"
              className="tooling-action-button"
              icon={<CloudDownloadOutlined />}
              loading={importBusy}
              onClick={() => void (mode === "skills" ? handleImportSkills() : handleImportMcp())}
            >
              {t("导入已有")}
            </Button>
            {mode === "skills" ? (
              <Button
                type="text"
                className="tooling-action-button"
                icon={<SearchOutlined />}
                onClick={() => setSkillDiscoverOpen(true)}
              >
                {t("发现技能")}
              </Button>
            ) : null}
          </Space>
        </div>

        <div className="tooling-managed-list">
          {mode === "skills" ? (
            skills.length > 0 ? (
              skills.map((skill) => (
                <ManagedCard
                  key={skill.managed_path}
                  title={skill.name}
                  description={skill.description || skill.directory}
                  updateAvailable={Boolean(skill.update_available)}
                  statuses={buildManagedStatuses(skill.installed_apps)}
                  busy={skillBusyName === skill.name}
                  toggleLabel={skill.installed_apps?.codex ? t(`停用 ${skill.name} 于 Codex`) : t(`启用 ${skill.name} 到 Codex`)}
                  deleteLabel={t(`删除 ${skill.name}`)}
                  onToggle={() => void handleSkillToggle(skill)}
                  onDelete={() => confirmDeleteSkill(skill)}
                />
              ))
            ) : (
              <Empty description={t("暂无技能")} />
            )
          ) : mcpServers.length > 0 ? (
            mcpServers.map((server) => {
              const display = describeMcpCard(server);
              return (
                <ManagedCard
                  key={server.id}
                  title={display.title}
                  description={display.subtitle}
                  titleVariant="account"
                  descriptionVariant="account"
                  statuses={buildManagedStatuses(server.enabled_apps)}
                  busy={mcpBusyId === server.id}
                  toggleLabel={server.enabled_apps?.codex ? t(`停用 ${display.title} 于 Codex`) : t(`启用 ${display.title} 到 Codex`)}
                  deleteLabel={t(`删除 ${display.title}`)}
                  onToggle={() => void handleMcpToggle(server)}
                  onDelete={() => confirmDeleteMcp(server)}
                />
              );
            })
          ) : (
            <Empty description={t("暂无 MCP")} />
          )}
        </div>
      </Card>

      <Modal
        open={skillDiscoverOpen}
        title={t("发现技能")}
        onCancel={() => setSkillDiscoverOpen(false)}
        footer={null}
        width="100vw"
        style={{ top: 0, paddingBottom: 0 }}
        className="tooling-discovery-modal"
        destroyOnHidden
      >
        <div className="tooling-discovery-shell">
          <div className="tooling-discovery-toolbar">
            <Input
              value={discoveryQuery}
              onChange={(event) => setDiscoveryQuery(event.target.value)}
              placeholder={t("搜索发现的技能")}
              className="tooling-discovery-search"
            />
            <Space wrap size={10}>
              <Typography.Text type="secondary">{discoveryStatus || (discoveryRefreshing ? t("正在更新") : "")}</Typography.Text>
              <Button icon={<ReloadOutlined />} loading={discoveryLoading} aria-label={t("刷新技能索引")} onClick={() => void handleDiscoveryRefresh()}>
                {t("刷新")}
              </Button>
              <Button aria-label={t("仓库管理")} onClick={() => setRepoManagerOpen(true)}>
                {t("仓库管理")}
              </Button>
            </Space>
          </div>

          <div className="tooling-discovery-list tooling-discovery-grid">
            {discoveryLoading && !discoveryResponse ? (
              <div className="tooling-discovery-loading">
                <Spin size="large" />
              </div>
            ) : discoveredSkills.length > 0 ? (
              discoveredSkills.map((skill) => (
                <DiscoveredSkillCard
                  key={skill.id}
                  skill={skill}
                  busy={discoveryBusyId === skill.id}
                  t={t}
                  onView={() => handleViewDiscoveredSkill(skill)}
                  onInstall={() => void handleDiscoveredSkillInstall(skill)}
                />
              ))
            ) : (
              <Empty description={t("暂无发现技能")} />
            )}
          </div>
        </div>
      </Modal>

      <Modal
        open={repoManagerOpen}
        title={t("仓库管理")}
        onCancel={() => {
          setRepoManagerOpen(false);
          resetRepoForm();
        }}
        footer={null}
        destroyOnHidden
        className="tooling-repo-manager-modal"
      >
        <div className="tooling-repo-manager-shell">
          <div className="tooling-repo-manager-toolbar">
            <Typography.Text type="secondary">{t("仅管理公开 GitHub / GitLab 仓库")}</Typography.Text>
            <Button type="primary" icon={<PlusOutlined />} aria-label={t("添加仓库")} onClick={openRepoCreate}>
              {t("添加仓库")}
            </Button>
          </div>

          <div className="tooling-discovery-list tooling-repo-manager-list">
            {skillRepos.length > 0 ? skillRepos.map((repo) => (
              <RepoManageRow
                key={repoRecordKey(repo.platform ?? "github", repo.owner, repo.name)}
                repo={repo}
                busy={repoOrdering || repoBusyKey === repoRecordKey(repo.platform ?? "github", repo.owner, repo.name)}
                dragging={repoDragKey === repoRecordKey(repo.platform ?? "github", repo.owner, repo.name)}
                onDragStart={() => setRepoDragKey(repoRecordKey(repo.platform ?? "github", repo.owner, repo.name))}
                onDragOver={(event) => {
                  event.preventDefault();
                }}
                onDrop={(event) => {
                  event.preventDefault();
                  void handleRepoReorder(repo);
                }}
                onDragEnd={() => setRepoDragKey(null)}
                onView={() => handleRepoView(repo)}
                onEdit={() => openRepoEdit(repo)}
                onDelete={() => void handleRepoRemove(repo)}
              />
            )) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t("暂无已管理仓库")} />}
          </div>
        </div>
      </Modal>

      <Modal
        open={repoEditorOpen}
        title={repoEditorMode === "edit" ? t("编辑仓库") : t("添加仓库")}
        onCancel={() => resetRepoForm()}
        footer={null}
        destroyOnHidden
        className="tooling-repo-editor-modal"
      >
        <div className="tooling-repo-editor-shell">
          <label className="tooling-repo-field">
            <span>{t("仓库链接")}</span>
            <Input
              aria-label={t("仓库链接")}
              className="tooling-repo-editor-input"
              value={repoForm.input}
              disabled={repoEditorMode === "edit"}
              onChange={(event) => {
                const nextInput = event.target.value;
                setRepoForm((current) => ({ ...current, input: nextInput }));
                void resolveRepoInput(nextInput);
              }}
            />
          </label>

          <label className="tooling-repo-field">
            <span>{t("分支")}</span>
            <Select
              aria-label={t("分支")}
              className="tooling-repo-editor-select"
              value={repoForm.branch}
              options={repoForm.branchOptions.map((branch) => ({ label: branch, value: branch }))}
              onChange={(branch) => setRepoForm((current) => ({ ...current, branch }))}
            />
          </label>

          <div className="tooling-repo-editor-summary">
            {repoResolving ? <Typography.Text type="secondary">{t("正在解析仓库和分支…")}</Typography.Text> : null}
            {!repoResolving && repoResolveError ? <Typography.Text type="danger">{repoResolveError}</Typography.Text> : null}
            {!repoResolving && !repoResolveError && repoForm.owner && repoForm.name ? (
              <Typography.Text type="secondary">{`${repoForm.platform.toUpperCase()} · ${repoForm.owner}/${repoForm.name}`}</Typography.Text>
            ) : null}
          </div>

          <Space wrap size={10}>
            <Button
              type="primary"
              loading={repoBusyKey === repoRecordKey(repoForm.platform, repoForm.owner, repoForm.name)}
              disabled={repoResolving || !repoForm.owner || !repoForm.name || !repoForm.branch}
              onClick={() => void handleRepoSave()}
            >
              {repoEditorMode === "edit" ? t("保存仓库") : t("确认添加仓库")}
            </Button>
            <Button onClick={() => resetRepoForm()}>{t("取消")}</Button>
          </Space>
        </div>
      </Modal>
    </div>
  );
}

function sortDiscoveryResponse(response: ToolingSkillDiscoveryResponse): ToolingSkillDiscoveryResponse {
  return {
    ...response,
    items: [...response.items].sort((left, right) => left.name.localeCompare(right.name)),
  };
}

function markDiscoveredSkillInstalled(
  response: ToolingSkillDiscoveryResponse | null,
  id: string,
): ToolingSkillDiscoveryResponse | null {
  if (!response) {
    return response;
  }
  return {
    ...response,
    items: response.items.map((item) => {
      if (item.id !== id) {
        return item;
      }
      return {
        ...item,
        installed_apps: {
          ...item.installed_apps,
          codex: true,
        },
        installed_hash: item.content_hash,
        update_available: false,
      };
    }),
  };
}

function repoRecordKey(platform: string, owner: string, name: string): string {
  return `${(platform || "github").toLowerCase()}:${owner.toLowerCase()}/${name.toLowerCase()}`;
}

function createEmptyRepoForm(): RepoEditorForm {
  return {
    input: "",
    platform: "github",
    owner: "",
    name: "",
    branch: "main",
    branchOptions: ["main"],
  };
}

function applyResolvedRepo(current: RepoEditorForm, resolved: ToolingResolvedRepo, preferredBranch?: string): RepoEditorForm {
  const branchOptions = resolved.branch_options.length > 0 ? resolved.branch_options : [resolved.selected_branch || preferredBranch || "main"];
  const selectedBranch =
    preferredBranch && branchOptions.includes(preferredBranch)
      ? preferredBranch
      : branchOptions.includes(resolved.selected_branch)
        ? resolved.selected_branch
        : branchOptions[0];
  return {
    ...current,
    input: resolved.repo_url,
    platform: resolved.platform,
    owner: resolved.owner,
    name: resolved.name,
    branch: selectedBranch,
    branchOptions,
  };
}

function buildRepoURL(platform: string, owner: string, name: string): string {
  return `${platform === "gitlab" ? "https://gitlab.com" : "https://github.com"}/${owner}/${name}`;
}

function DeleteMcpConfirmContent({
  server,
  t,
  onCleanupChange,
}: {
  server: ToolingMcpServer;
  t: (text: string) => string;
  onCleanupChange: (next: boolean) => void;
}) {
  const [cleanupLocalFiles, setCleanupLocalFiles] = useState(true);
  const deleteTargets = server.delete_targets ?? [];

  useEffect(() => {
    onCleanupChange(cleanupLocalFiles);
  }, [cleanupLocalFiles, onCleanupChange]);

  return (
    <Space orientation="vertical" size={10} className="tooling-delete-confirm-content">
      <div>{t(`将同时从 AI Gate 与 Codex 删除 ${server.name}`)}</div>
      <Checkbox
        defaultChecked
        checked={cleanupLocalFiles}
        onChange={(event) => {
          setCleanupLocalFiles(event.target.checked);
        }}
      >
        {t("同步清理本地文件")}
      </Checkbox>
      <Typography.Text type="secondary">
        {t("仅清理 Codex 直接托管的当前 MCP 目录，默认开启。")}
      </Typography.Text>
      {cleanupLocalFiles && deleteTargets.length > 0 ? (
        <div className="tooling-delete-targets" role="list" aria-label={t("待删除路径列表")}>
          <div className="tooling-delete-targets-title">{t("以下本地路径将被删除")}</div>
          {deleteTargets.map((target) => (
            <div key={target} role="listitem" className="tooling-delete-target-path">
              {target}
            </div>
          ))}
        </div>
      ) : null}
    </Space>
  );
}

function DiscoveredSkillCard({
  skill,
  busy,
  t,
  onView,
  onInstall,
}: {
  skill: ToolingDiscoveredSkill;
  busy?: boolean;
  t: (text: string) => string;
  onView: () => void;
  onInstall: () => void;
}) {
  const installed = Boolean(skill.installed_apps?.codex);
  const updateAvailable = installed && Boolean(skill.update_available);
  const actionLabel = updateAvailable ? t("更新") : installed ? t("已安装") : t("安装");
  const statusLabel = updateAvailable ? t("可更新") : installed ? t("已安装") : t("未安装");
  return (
    <div className="tooling-discovered-card">
      <div className="tooling-discovered-card-main">
        <div className="tooling-discovered-card-meta">
          <span className="tooling-discovered-platform">{skill.platform.toUpperCase()}</span>
          <span className="tooling-discovered-repo">{`${skill.repo_owner}/${skill.repo_name}`}</span>
          <span className={`tooling-status-pill ${updateAvailable ? "is-update" : installed ? "is-enabled" : "is-disabled"}`}>{statusLabel}</span>
        </div>
        <button type="button" className="tooling-discovered-title-button" data-testid="tooling-discovered-skill-title" aria-label={`打开 ${skill.name} 的源目录`} onClick={onView}>
          {skill.name}
        </button>
        {skill.description ? <div className="tooling-item-description is-default">{skill.description}</div> : null}
      </div>
      <div className="tooling-discovered-card-actions">
        <Button icon={<LinkOutlined />} aria-label={`查看 ${skill.name} 的仓库页面`} onClick={onView}>
          {t("查看")}
        </Button>
        <Button
          type="primary"
          icon={updateAvailable ? <ReloadOutlined /> : <PlusOutlined />}
          loading={busy}
          disabled={installed && !updateAvailable}
          aria-label={`${actionLabel} ${skill.name}`}
          onClick={onInstall}
        >
          {actionLabel}
        </Button>
      </div>
    </div>
  );
}

function RepoManageRow({
  repo,
  busy,
  dragging,
  onDragStart,
  onDragOver,
  onDrop,
  onDragEnd,
  onView,
  onEdit,
  onDelete,
}: {
  repo: ToolingSkillRepo;
  busy?: boolean;
  dragging?: boolean;
  onDragStart: () => void;
  onDragOver: (event: DragEvent<HTMLDivElement>) => void;
  onDrop: (event: DragEvent<HTMLDivElement>) => void;
  onDragEnd: () => void;
  onView: () => void;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const repoUrl = buildRepoURL(repo.platform ?? "github", repo.owner, repo.name);
  return (
    <div
      className={`tooling-repo-row ${dragging ? "is-dragging" : ""}`}
      draggable
      onDragStart={onDragStart}
      onDragOver={onDragOver}
      onDrop={onDrop}
      onDragEnd={onDragEnd}
    >
      <div className="tooling-repo-main">
        <div className="tooling-repo-title">
          <span className="tooling-repo-drag-handle" aria-hidden="true">
            <DragOutlined />
          </span>
          {`${repo.owner}/${repo.name}`}
        </div>
        <div className="stats-subtitle">{`${repo.skill_count} skills · ${repoUrl}`}</div>
      </div>
      <div className="tooling-repo-actions">
        <button type="button" className="tooling-repo-action-button" aria-label={`查看 ${repo.owner}/${repo.name}`} onClick={onView}>
          查看
        </button>
        <button type="button" className="tooling-repo-action-button" aria-label={`编辑 ${repo.owner}/${repo.name}`} onClick={onEdit}>
          编辑
        </button>
        <button type="button" className="tooling-repo-action-button is-danger" aria-label={`删除 ${repo.owner}/${repo.name}`} disabled={busy} onClick={onDelete}>
          删除
        </button>
      </div>
    </div>
  );
}

function skillCollectionName(skill: ToolingSkillRecord): string {
  const trimmed = skill.managed_path.trim();
  const parts = trimmed.split(/[\\/]/).filter(Boolean);
  return parts.at(-1) ?? skill.name;
}

function buildManagedStatuses(enabledApps?: Record<string, boolean>): ManagedClientStatus[] {
  return managedClients.map((client) => ({
    ...client,
    enabled: Boolean(enabledApps?.[client.id]),
  }));
}

function describeMcpCard(server: ToolingMcpServer): { title: string; subtitle?: string } {
  const displayPath = getMcpDisplayPath(server);
  return {
    title: server.id,
    subtitle: displayPath,
  };
}

function getMcpDisplayPath(server: ToolingMcpServer): string | undefined {
  if (looksLikeLocalPath(server.name)) {
    return server.name.trim();
  }
  const command = server.spec?.command;
  if (typeof command === "string" && looksLikeLocalPath(command)) {
    return command.trim();
  }
  return undefined;
}

function looksLikeLocalPath(value?: string): boolean {
  if (!value) {
    return false;
  }
  const trimmed = value.trim();
  return (
    trimmed.startsWith("/") ||
    trimmed.startsWith("~/") ||
    trimmed.startsWith("./") ||
    trimmed.startsWith("../") ||
    trimmed.startsWith("\\\\") ||
    /^[A-Za-z]:[\\/]/.test(trimmed)
  );
}

function ManagedCard({
  title,
  description,
  titleVariant = "default",
  descriptionVariant = "default",
  updateAvailable = false,
  statuses,
  busy,
  toggleLabel,
  deleteLabel,
  onToggle,
  onDelete,
}: {
  title: string;
  description?: string;
  titleVariant?: "default" | "account";
  descriptionVariant?: "default" | "account";
  updateAvailable?: boolean;
  statuses: ManagedClientStatus[];
  busy?: boolean;
  toggleLabel: string;
  deleteLabel: string;
  onToggle: () => void;
  onDelete: () => void;
}) {
  const primaryStatus = statuses[0];
  const isPrimaryEnabled = primaryStatus?.enabled ?? false;
  return (
    <div className="tooling-item-card">
      <div className="tooling-item-main">
        <div className="tooling-item-title-row">
          <div className={`tooling-item-title ${titleVariant === "account" ? "is-account" : ""}`}>{title}</div>
          {updateAvailable ? <span className="tooling-status-pill is-update">可更新</span> : null}
        </div>
        {description ? (
          <div className={`tooling-item-description ${descriptionVariant === "account" ? "is-account" : "is-default"}`}>
            {description}
          </div>
        ) : null}
      </div>
      <div className="tooling-item-aside">
        <div className="tooling-item-statuses" aria-label="安装目标状态">
          {statuses.map((status) => (
            <div
              key={status.id}
              className={`tooling-status-pill ${status.enabled ? "is-enabled" : "is-disabled"}`}
              aria-label={`${status.label} ${status.enabled ? "已激活" : "未激活"}`}
              title={`${status.label} ${status.enabled ? "已激活" : "未激活"}`}
            >
              <img src={status.icon} alt="" aria-hidden="true" className="tooling-provider-icon tooling-provider-icon-small" />
              <span>{status.label}</span>
            </div>
          ))}
        </div>
        <div className="tooling-item-hover-actions">
          <button
            type="button"
            className={`tooling-item-primary-action ${isPrimaryEnabled ? "is-danger" : "is-primary"}`}
            aria-label={toggleLabel}
            onClick={onToggle}
            disabled={busy}
          >
            <CheckCircleOutlined className="tooling-item-primary-action-icon" />
            {isPrimaryEnabled ? "停用" : "启用"}
          </button>
          <button
            type="button"
            className="tooling-item-icon-action tooling-item-icon-action-danger"
            aria-label={deleteLabel}
            onClick={onDelete}
            disabled={busy}
          >
            <DeleteOutlined />
          </button>
        </div>
      </div>
    </div>
  );
}

function reorderReposByKey(repos: ToolingSkillRepo[], sourceKey: string, targetKey: string): ToolingSkillRepo[] {
  if (sourceKey === targetKey) {
    return repos;
  }
  const fromIndex = repos.findIndex((repo) => repoRecordKey(repo.platform ?? "github", repo.owner, repo.name) === sourceKey);
  const toIndex = repos.findIndex((repo) => repoRecordKey(repo.platform ?? "github", repo.owner, repo.name) === targetKey);
  if (fromIndex < 0 || toIndex < 0) {
    return repos;
  }
  const next = [...repos];
  const [moved] = next.splice(fromIndex, 1);
  next.splice(toIndex, 0, moved);
  return next;
}

function DiscoveryRow({
  title,
  description,
  actionLabel,
  actionBusy,
  danger = false,
  onAction,
}: {
  title: string;
  description: string;
  actionLabel: string;
  actionBusy?: boolean;
  danger?: boolean;
  onAction: () => void;
}) {
  return (
    <div className="tooling-repo-row">
      <div className="tooling-repo-main">
        <div className="tooling-repo-title">{title}</div>
        <div className="stats-subtitle">{description}</div>
      </div>
      <Button size="small" danger={danger} loading={actionBusy} icon={danger ? <DeleteOutlined /> : <PlusOutlined />} onClick={onAction}>
        {actionLabel}
      </Button>
    </div>
  );
}
