# Local Package Version From Reachable Tag Design

## Context
本地桌面端打包当前没有统一入口。版本同步依赖 `scripts/release/sync_release_metadata.sh` 显式传参，产物收集脚本 `scripts/desktop/collect_release_assets.sh` 在没有传入版本时会回退到 `local`，这会导致本地产物名、应用元数据和当前代码所对应版本不一致。

当前需求是在本地打包时自动使用当前提交可达的最近 tag，也就是 `git describe --tags --abbrev=0` 的结果。

## Options

### Option 1: Add a local packaging entrypoint
新增统一的本地打包脚本。脚本负责解析最近可达 tag、同步版本元数据、执行桌面端构建并收集产物。

Pros:
- 本地打包流程单一，版本规则集中。
- 应用元数据和产物文件名一致。
- CI 发布流程可以保持现状。

Cons:
- 需要新增一个脚本并补测试。

### Option 2: Only change asset collection fallback
只修改 `collect_release_assets.sh`，让它默认取最近 tag。

Pros:
- 改动最小。

Cons:
- 应用内版本可能仍停留在旧值。
- 产物名和桌面应用版本可能漂移。

### Option 3: Spread tag resolution across scripts
在多个脚本里分别增加最近 tag 的 fallback。

Pros:
- 单个脚本可独立运行。

Cons:
- 规则重复，后续容易分叉。
- 出问题时定位成本更高。

## Decision
采用 Option 1。新增统一的本地打包入口，并让现有脚本继续作为可复用子步骤存在。

## Design

### Version resolution
本地打包入口按以下优先级确定版本：
1. 显式传入的 `RELEASE_VERSION`
2. `git describe --tags --abbrev=0`

如果两者都拿不到，脚本直接失败，并明确提示当前提交没有可达 tag。

### Metadata sync
在执行桌面端构建前，调用 `scripts/release/sync_release_metadata.sh --tag "$TAG"`。这样 `frontend/package.json`、`desktop/package.json`、`desktop/src-tauri/Cargo.toml`、`desktop/src-tauri/tauri.conf.json` 以及相关 lock 文件都同步到统一版本。

### Asset collection
`collect_release_assets.sh` 保留环境变量优先级，但当外部没有传版本时，默认解析最近可达 tag，而不是回退到 `local`。这样单独运行收集脚本时行为仍然正确。

### Packaging entrypoint
新增 `scripts/desktop/package_local_release.sh`：
1. 解析版本 tag
2. 同步元数据
3. 调用 `npm --prefix desktop run tauri build`
4. 调用 `scripts/desktop/collect_release_assets.sh`
5. 输出版本和产物目录

### Error handling
- `git describe` 失败时，中止并提示先打 tag 或显式设置 `RELEASE_VERSION`
- 任一步骤失败都直接退出，保留原始错误

### Testing
新增脚本测试覆盖以下行为：
- 版本解析 helper 在未提供环境变量时返回最近可达 tag
- `collect_release_assets.sh` 在未提供版本时使用最近 tag
- 本地打包入口在缺少 tag 时会失败，并给出明确错误

## Impact
这个变更只影响本地打包流程，不改变 GitHub release workflow 的输入方式。CI 仍然使用触发 tag 作为版本源。
