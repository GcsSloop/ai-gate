# Tray Icon Platform Design

## Goal
修复桌面端菜单栏/托盘图标在 macOS 和 Windows 上的显示策略，使 macOS 能根据浅色/深色菜单栏自动适配黑白，而 Windows 保持彩色托盘图标。

## Selected Approach
- macOS：使用 `assets/aigate-128x128-white.png` 作为模板图标原型，复制到 Tauri 图标目录中的 tray 专用文件，并在 tray builder 上显式启用 template 语义。
- Windows：使用 `assets/aigate-128x128-color.png` 作为专用托盘图标，保持彩色显示。
- 运行时：在 Rust 端按平台选择不同的 tray icon 资源，不改变应用主图标资源。

## Why
macOS 菜单栏图标需要 template image 语义，系统才会在亮/暗背景下自动反相；单纯一张白色 PNG 不带 template 标记时不会稳定跟随主题。Windows 没有这套语义，直接使用彩色托盘图标更符合平台习惯。

## Scope
只修改桌面端托盘图标资源选择与 tray 初始化逻辑，并补充针对平台分支选择的单元测试。
