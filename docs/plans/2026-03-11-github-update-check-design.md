# GitHub Update Check And Install Design

## Context
当前桌面端只暴露应用版本号，没有检查更新、下载更新或安装重启能力。发布流程当前只上传 `dmg`、`zip`、`msi` 和校验文件，还没有生成 Tauri updater 所需的签名更新包与 `latest.json`。

Tauri 2 官方 updater 插件支持通过静态 JSON 文件检查更新，并明确支持把 `https://github.com/user/repo/releases/latest/download/latest.json` 作为 endpoint。它要求更新包必须签名，这一要求不能关闭。`createUpdaterArtifacts: true` 会在构建时为各平台生成 updater 工件和 `.sig` 文件。GitHub Releases 可以作为这些静态文件的分发源。 citeturn2view0turn2view1turn2view2turn2view3turn1view1

## Options

### Option 1: Tauri updater plugin + GitHub Release static JSON
直接使用 Tauri 2 官方 updater，客户端内置 GitHub `latest.json` endpoint，发布流程产出并上传 updater artifacts、签名文件和 `latest.json`。

Pros:
- 官方支持路径，平台安装细节和签名校验都由 Tauri 接管。 citeturn2view0turn2view2turn2view3
- 前端可直接用 `check()`、`downloadAndInstall()`，安装后配合 `relaunch()` 完成重启。 citeturn3view0turn3view1
- `tauri-action` 官方支持生成 `latest.json` 并上传 updater 资产到 GitHub Release。 citeturn2view3turn4search1

Cons:
- 需要配置签名密钥和更新公钥。
- Release workflow 需要重构为以 updater 工件为中心。

### Option 2: GitHub Releases API + 自研下载/安装
客户端请求 GitHub Releases API 获取最新版本，自己挑选资产、下载并调用平台安装器。

Pros:
- GitHub 数据结构完全可控。 citeturn1view1

Cons:
- 下载安装逻辑、签名校验、平台兼容都要自己维护。
- 用户体验和安全性都不如 Tauri 官方 updater。

### Option 3: 自建更新服务
后端代理 GitHub Release，向客户端输出动态更新接口。

Pros:
- 支持灰度、渠道和权限控制。

Cons:
- 对当前项目是过度设计。

## Decision
采用 Option 1。使用 Tauri 官方 updater 插件和 GitHub Release 静态 `latest.json`，实现桌面端的检查、下载、安装与重启。

## Design

### Release pipeline
Release workflow 调整为同时生成传统发行包和 updater 工件。`bundle.createUpdaterArtifacts` 设为 `true`，构建环境注入 `TAURI_SIGNING_PRIVATE_KEY` 与可选的 `TAURI_SIGNING_PRIVATE_KEY_PASSWORD`。构建完成后上传 updater 相关文件以及 `latest.json` 到 GitHub Release。GitHub 端仍保留 `dmg`/`msi` 供手动下载安装，updater 则消费 `latest.json` 中声明的 URL 和签名。 citeturn2view1turn2view2turn2view3turn4search1

### Desktop runtime
Rust 侧注册 `tauri-plugin-updater` 和 `tauri-plugin-process`。Updater 配置写入 `tauri.conf.json`：
- `bundle.createUpdaterArtifacts: true`
- `plugins.updater.pubkey: <public key>`
- `plugins.updater.endpoints: ["https://github.com/GcsSloop/ai-gate/releases/latest/download/latest.json"]`
- Windows 额外设置 `installMode: passive`

由于前端要驱动完整更新流程，还需要在 capability 中启用 `updater:default`，它默认包含 `allow-check`、`allow-download`、`allow-install` 和 `allow-download-and-install`。 citeturn2view2turn3view2turn3view3

### Frontend UX
在设置页或关于页加入“检查更新”卡片：
- 默认显示当前版本
- 点击后发起检查
- 若无更新，显示“已是最新版本”
- 若有更新，弹出更新信息：目标版本、发布时间、发布说明
- 用户确认后执行下载并展示进度
- 安装完成后显示“立即重启”按钮，调用 `relaunch()`

同时增加静默检查策略：应用启动后延迟一次后台检查，只更新状态，不自动弹窗；只有用户主动点击或明确有新版本时才打断界面。

### Error handling
- 网络错误：提示“检查更新失败”，保留当前版本
- 配置错误：若 `latest.json` 缺失或不完整，前端提示更新源异常
- 安装失败：展示错误并允许重试下载
- Windows 被系统锁定或权限不足：提示用户关闭占用进程或手动安装

### Testing
1. 前端单元测试：校验更新状态机和按钮文案切换
2. Rust 单元测试：校验应用元数据、更新状态桥接结构
3. Release workflow 测试：新增脚本测试，验证 `latest.json` 与 updater 资产收集逻辑
4. 手工验收：本地构建 updater 工件，检查 GitHub Release 资产命名和客户端更新路径

## Open Prerequisites
需要在仓库或 CI secrets 中补充以下配置：
- `TAURI_SIGNING_PRIVATE_KEY`
- 可选 `TAURI_SIGNING_PRIVATE_KEY_PASSWORD`
- 对应公钥内容，写入客户端配置

没有这组密钥，客户端可以实现 UI 和流程，但无法产出可安装的正式更新。 citeturn2view0turn2view1
