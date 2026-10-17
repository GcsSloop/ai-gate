# 2026-10-17 Codex 代理开启后会话 provider 混用分析

## 结论

AI Gate 的代理开关现在会修改用户级默认 provider，并自动迁移本机持久化的官方
`openai` 会话到 `aigate`。Codex 将 provider 作为会话状态保存：冷恢复默认沿用
持久化 provider，热恢复保留已加载会话的配置。因此，自动迁移能解决持久化会话混用，
但正在运行的会话仍需重新加载。

本次更新代理开关的会话 provider 迁移，并补充回滚日志、测试和界面提示；没有迁移
真实本机历史或替换已安装的客户端。

## 源码更新

- 路径：`references/openai-codex`。
- 原版本：`6b9826e3aa83b1a5947db50f4332cb9c65f1b340`，`rust-v0.154.0`。
- 更新版本：`ce0323f2f2d3b25e82806cd555237da3db4e1897`，获取时的 `origin/main`。
- 上游提交时间：2026-10-17 04:02:21 UTC（北京时间 12:02:21）。
- 本机运行的 CLI 版本：`0.163.0-alpha.10.2`。源码更新不会改变该二进制。
- 上游开发分支 workspace 版本为 `0.0.0`。AI Gate 的请求版本标识继续保留
  已发布的 `0.154.0` 兼容基线，避免把开发占位版本用于上游模型目录筛选。

保留了子模块的稀疏检出方式，额外展开本次分析涉及的源码和测试。

## 源码链路

### 1. AI Gate 写默认配置并迁移持久化会话

`backend/internal/api/settings_handler.go` 中的 `enableProxy`：

1. 读取用户 home 下 `.codex/config.toml`。
2. 设置 `model_provider = "aigate"`，写入本地 `base_url` 的 provider 定义。
3. 保存代理 session 与配置 hash，返回 `enabled: true`。

随后会扫描 `sessions/`、`archived_sessions/` 和最新 `state_*.sqlite`，只切换
provider 为 `openai` 的会话。操作前写入 `~/.aigate/data/codex/provider-migrations/`
下的 manifest，关闭代理时按 manifest 恢复，且仅在当前仍为 `aigate` 时写回，避免覆盖
用户之后主动选择的 provider。UI 会提示正在运行的会话需要重新加载。

### 2. 冷恢复会话优先恢复持久化 provider

上游 `codex-rs/app-server/src/request_processors/thread_processor.rs`：

- `load_and_apply_persisted_resume_metadata` 读取 state DB 中的 `ThreadMetadata`。
- `merge_persisted_resume_metadata` 在没有显式模型相关覆盖参数时，将数据库的
  `model_provider` 写入 `ConfigOverrides.model_provider`。
- `has_model_resume_override` 检查请求中的 model、provider 和推理强度等覆盖参数。

`codex-rs/core/src/config/mod.rs` 在选择 provider 时，优先级包含：管理要求、
显式 `ConfigOverrides.model_provider`、配置文件默认 provider。因此，旧会话保存的
`openai` 可以通过恢复覆盖参数优先于用户级配置中的 `aigate`。

附近的 `thread_processor_tests.rs` 已包含恢复持久化 provider、保留显式覆盖参数
以及显式 provider 覆盖优先的单元测试。本次阅读这些测试，未编译运行上游 Rust 测试。

### 3. 热恢复会话保留已有运行时配置

同一文件的 `thread_resume_inner` 先尝试 `resume_running_thread`。找到已加载会话时，
使用 `existing_thread.config_snapshot()` 返回已有配置。

若请求显式传入不同 provider，源码会检查是否允许卸载并冷恢复：只有无订阅客户端、
空闲、未运行且允许直接输入等条件同时满足，并完成关闭后，才重新创建会话。
其他情况下保留原会话，并记录 `thread/resume overrides ignored for loaded thread`。

`core/src/config/runtime_refresh.rs` 也以克隆已有配置为基础更新允许刷新的字段，
没有把新 provider 自动替换进已有会话。

### 4. 登录账号与模型路由属于不同状态

AI Gate 写入 `requires_openai_auth = true`，同时保留官方登录凭证。因此即使模型
请求已经走本地网关，客户端仍可能显示官方登录账号。

上游 `codex-rs/model-provider-info/src/lib.rs` 的 `to_api_provider` 优先使用 provider
显式配置的 `base_url`。AI Gate 收到请求后，由
`backend/internal/api/responses_handler.go` 选择账号、解析该账号凭证并构造上游请求。
是否显示官方账号不能替代 provider 和网关请求记录的核对。

## 本机只读证据

以下是 2026-10-17 排查时读取的元数据快照，不含凭证和聊天正文，时间为北京时间。

- 用户级配置：默认 `model_provider = "aigate"`，对应
  `http://127.0.0.1:6789/ai-router/api`；没有选中的 profile 或 profile provider 覆盖。
- 会话数据库快照：`aigate` 738 条、`openai` 100 条、`router` 10 条、
  `my_codex` 5 条、`codex` 4 条。数量包括历史记录，不能当作活跃连接数。
- 样本 A：22:52:54 创建的会话保存为 `openai`，23:25:14 仍有更新。
- 样本 B：23:13:49 创建的会话保存为 `aigate`，23:25:28 仍有更新。
- 两个样本的 rollout 首行 provider 与数据库一致，CLI 版本均为
  `0.163.0-alpha.10.2`，originator 均为 `codex_work_desktop`。
- 配置文件修改时间为 23:13:49，与样本 B 的创建时间相近；这只是时间关联，
  不能单凭文件修改时间推断全部切换经过。

这些证据确认同一客户端存在两种会话 provider，与源码解释一致。本次没有抓取
上述会话的实际模型请求，不能把 provider 元数据当作逐次请求的网络取证，也没有
验证所有桌面端内部覆盖参数或证明每个历史会话命中哪个具体上游账号。

## 处理建议

1. 开启代理后，用新建会话确认默认 provider 为 `aigate`，并对照 AI Gate 的请求记录。
2. 对旧会话，显式切换 provider 时需要重建会话配置；对正在运行或仍有客户端订阅的
   会话，不应假设一次 resume 覆盖就能生效。
3. 已有 `scripts/migrate_openai_history_to_aigate.sh` 可以复制历史到 `aigate`，保留原始
   会话。该方式需要使用迁移副本；继续打开原来的 `openai` 会话仍可能绕过本地网关。
4. 后续产品修复可增加开关说明、旧会话 provider 检查与迁移入口，明确区分默认配置
   已写入和所有会话已切换。需要另行实现和验证，不能把本次分析当作修复。

## 验证

子模块状态核验：

```bash
git -C references/openai-codex rev-parse HEAD origin/main
git -C references/openai-codex status --short
git submodule status
```

代理定向 harness：

```bash
cd backend && go test ./internal/api -run 'TestSettingsHandler(Proxy|UpdatingProxyAddress|EnableWithThirdPartyProvider|EnableNormalizesDuplicateAigateProviderSections|EnableRemovesLegacyAigateDefinitions)' -count=1
```

后端回归：

```bash
GOCACHE="$PWD/.tmp-gocache" go -C backend test ./...
```

上述子模块核验、代理定向 harness、后端全量测试和 `git diff --check` 均通过。
官方配置字段说明参考 [OpenAI 配置文档](https://learn.chatgpt.com/docs/config-file/config-reference)。

Go harness 覆盖 AI Gate 的配置读写、provider 定义与 auth 文件保留行为，不验证
已安装桌面客户端的真实热切换行为。上游 Rust 工程采用稀疏检出，本次没有完整构建。
