# Tray Stability And Plan Cleanup Design

## Goal

修复桌面端托盘菜单展开后自动关闭和偶发 abort 崩溃，同时清理 `docs/plans/` 中与当前薄网关实现冲突的旧说明。

## Root Cause

当前桌面端存在两个高风险点：

1. 托盘菜单会在用户点击托盘图标时立即执行 `refresh_tray_menu`，菜单刚展开就被替换，导致菜单自动关闭。
2. 桌面端还启动了一个后台线程每 2 秒刷新一次托盘菜单，这会在退出或窗口生命周期边界上继续向 tao/Tauri 事件循环发送 UI 更新，放大成 `tao::platform_impl::platform::app::send_event` 上的 panic/abort。

退出路径里还会对主窗口执行 `window.eval(...)` 注入事件，这同样增加了退出阶段的生命周期风险。

## Chosen Approach

采用最小且稳妥的方案：

- 删除托盘后台轮询线程 `start_tray_sync_task`
- 删除托盘点击时的即时 `refresh_tray_menu`
- 仅在这些事件后刷新菜单：应用启动完成、菜单动作执行后
- 退出冲突时不再向 webview 注入脚本，只显示主窗口并记录日志
- 清理 `docs/plans/` 中所有与厚模式、fallback、`THIN_GATEWAY_MODE` 冲突的内容

## Why This Approach

- 能同时修复“菜单自动关闭”和“偶发 abort”两个问题
- 不引入新的线程同步或主线程调度复杂度
- 保持托盘主要功能不变，仅牺牲后台实时刷新
- 与当前“薄网关单一路径”架构一致

## Testing Strategy

- 新增/更新 Rust 单元测试，锁定托盘行为辅助逻辑
- 运行 `cargo test` 验证桌面端测试通过
- 重新构建 Tauri 应用，确认能够成功打包
- 手工验证托盘菜单可稳定展开、不再自动关闭

