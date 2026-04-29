# AGENTS.md

本文件适用于整个仓库，面向在 AI Gate 项目中工作的编码 agent。

## 项目形态

AI Gate 是面向 Codex 工作流的本地优先路由层和桌面壳。最重要的边界是薄网关语义：保留上游协议语义，账号密钥留在本机，做不到的能力要明确不支持，不要伪造兼容。

主要目录：

- `backend/`：Go 路由服务、账号管理、策略、用量、工具链和本地 API handler。
- `frontend/`：Vite + React + Ant Design 桌面端/网页端 UI。
- `desktop/`：Tauri 桌面壳和桌面打包配置。
- `services/skill-metrics-worker/`：Cloudflare Worker，负责 Skill registry、排行和 tracked repo 元数据。
- `scripts/`：发布、打包、迁移和冒烟测试 harness。
- `docs/`：用户文档、运维说明和实现计划。
- `skills/`：仓库内置 Codex skills。

## Harness-First 工作流

每个改动都必须能被某种 harness 验证。harness 可以是单元测试、集成测试、shell 冒烟脚本、本地 dev server、Worker 命令、打包脚本，或带有具体输出的定向手工检查。

编辑前：

- 先识别能覆盖这个 bug 或回归的最小 harness。
- 先读本地代码路径和附近测试，再决定实现方式。
- 保留已有用户改动，不要回退无关 dirty files。

编辑中：

- 做满足需求的最小连贯改动。
- 优先使用现有 helper、路由模式、状态模型和测试工具。
- 避免臆测式抽象和无关重构。
- 行为变更尽量先新增或更新 harness，再改实现。

完成前：

- 运行相关 harness，并在回复中写明具体命令。
- 如果 harness 无法运行，说明原因和仍未验证的风险。
- 没有看到真实输出前，不要声称修复完成、构建成功、发布成功或测试通过。

## 验证矩阵

先运行最窄的适用命令；如果改动影响共享行为，再扩大验证范围。

- 后端 Go 改动：`cd backend && go test ./...`
- 迭代中的特定 Go 包：`cd backend && go test ./internal/<pkg> -run <TestName> -count=1`
- 前端改动：`npm --prefix frontend test`
- 前端生产构建风险：`npm --prefix frontend run build`
- 桌面壳或打包改动：运行 `scripts/desktop/` 下的相关脚本；触及打包行为时运行 `npm --prefix desktop run tauri build`。
- 发布脚本改动：运行对应的 `scripts/test/*_test.sh` harness。
- Skill metrics worker 改动：`cd services/skill-metrics-worker && npm test`
- tracked repo seed 改动：用 `jq empty services/skill-metrics-worker/config/tracked-repos.seed.json` 校验 JSON。
- 端到端路由行为：启动 `make backend`，再按 `docs/testing.md` 用 curl 或 Codex CLI 检查本地 `/ai-router/api` 路由。

如果命令会写缓存或数据库，优先使用 Makefile 和脚本中已有的仓库内路径。除非任务明确需要且已获得许可，不要写工作区之外的位置。

## 后端规范

- 保留薄网关语义。不要在本地合成 `response_id`、`previous_response_id`、response retrieval 或 `/chat/completions` fallback。
- 上游状态码、SSE 生命周期和 terminal events 以上游为准。
- 除非任务明确修改 LAN sharing 行为，后端保持 loopback-first。
- 账号密钥、auth 文件、数据库快照和配置补丁必须保持本地优先且可恢复。
- 优先在被改包附近补仓库级测试。复用现有 temp home、SQLite、auth fixture 和 request handler 测试工具。
- 触及 routing 或 failover 时，要验证 terminal output 和非挂起行为，不能只看 HTTP status。

## 前端规范

- 遵循 `frontend/src/features/*` 里的 React + Ant Design 现有模式。
- 操作型 UI 要保持信息密度、可扫读和可预测。这个项目是工作工具，不是营销页面。
- 使用 `frontend/src/lib/api.ts` 的 API wrapper、`frontend/src/lib/desktop-shell.ts` 的桌面集成 helper、`frontend/src/lib/i18n.ts` 的本地化 helper。
- 在受影响页面或组件旁边新增或更新测试。
- 工具栏、列表、卡片和表格在不同视口下要保持控件尺寸稳定，避免布局跳动。
- 除非现有体系无法承载需求，不要引入新的视觉系统。

## 桌面与打包规范

- Tauri sidecar 行为要和后端二进制及发布脚本保持一致。
- 打包变更要用 `scripts/desktop/` 或 `scripts/test/` 下的对应脚本验证。
- 不要在桌面配置、发布元数据或 notarization 逻辑里硬编码本地开发者路径。
- 签名、updater manifest、发布文件名和资产收集要视为同一个被 harness 覆盖的流程。

## Skill Metrics Worker 规范

- `tracked_repos` 是远程 Skill 仓库的中心 registry。修改 registry 模型时，要保持 seed JSON 和远端行为一致。
- 公共接口不能泄露 admin-only 指标或密钥。
- 管理操作必须保持 token/session 保护。
- 修改 D1 或 KV schema 假设时，要同步补测试。
- 远端 Worker HTTP 入口不可用时，只有在本机 Cloudflare 登录态已可用且任务确实需要时，才用 Wrangler D1 检查。

## 文档规范

- 行为、命令、路由、环境变量、打包或部署步骤变化时，同步更新 `README.md`、`docs/testing.md`、ops 文档或 plans。
- commit 信息、plan 文档、变更记录、交接说明和其他长期记录统一使用中文；只有命令、代码标识、API 路径、错误原文等需要保真的内容保留原文。
- `AGENTS.md` 只写 agent 操作规则，不重复用户向安装和使用说明。
- 持久文档使用绝对日期。避免“今天”“最近”“当前”等相对时间。
- 优先替换过时说明，不要追加互相矛盾的注释。

## Git 与安全

- 工作区可能已有 dirty files。编辑前先检查，保留无关改动。
- 不要运行 `git reset --hard`、大范围 `rm`、基于 checkout 的回退等破坏性命令，除非用户明确要求。
- 不要在用户未要求时 commit、tag、push、deploy 或修改远端状态。
- 除非生成物、本地数据库、缓存和 release assets 本身就是交付物，否则不要改它们。
