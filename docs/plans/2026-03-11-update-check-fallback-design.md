# Update Check Fallback And Feedback Design

## Context
当前更新卡片在桌面端可以检查和安装更新，但浏览器或不支持 updater 的环境下只会显示“不支持自动更新”，用户看不到 GitHub 上最新版本是多少。同时点击“检查更新”只有按钮 icon 旋转，反馈较弱。

## Decision
保持 Tauri updater 作为桌面端安装入口，同时增加一个通用的只读检查路径：当自动更新不可用时，前端直接请求 GitHub Releases 的 `latest.json`，解析出最新版本、发布时间和说明，仅用于展示，不提供安装按钮。

## UX
- 点击“检查更新”后，按钮继续保留旋转状态。
- 检查状态区域增加显式的脉冲动画，直到检查结束。
- 无论是否支持自动更新，只要能获取 `latest.json`，都显示“最新版本/目标版本”。
- 非桌面环境下文案改为“当前环境不支持自动安装，但已检查到最新版本”。

## Data Flow
1. `UpdateCard` 调用 `updateService.check(currentVersion)`。
2. 桌面端优先走 Tauri updater，返回可安装的更新信息。
3. 非桌面端或桌面端返回 unsupported 时，服务回退到 `latest.json` fetch。
4. `UpdateCard` 根据 `supported` 控制是否显示“下载并安装”，根据返回的 `update` 始终渲染最新版本信息。

## Testing
1. `updateService` 增加 fallback fetch 测试。
2. `UpdateCard` 增加 unsupported 但显示最新版本的测试。
3. `UpdateCard` 增加 checking 动画 class 的测试。
