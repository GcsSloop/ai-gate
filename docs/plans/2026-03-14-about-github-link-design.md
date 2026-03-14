# 关于页 GitHub 链接设计

## 目标

在程序的关于页面增加项目 GitHub 仓库链接，指向：

- https://github.com/GcsSloop/ai-gate

## 现状

关于页位于设置页面的 `about` tab，当前已经展示：

- 应用图标与名称
- 版本号
- 作者
- 更新卡片

但没有项目主页入口。

## 方案对比

### 方案 A：在关于页元信息区域新增 GitHub 行

优点：
- 与作者、版本信息保持同一视觉层级
- 改动最小，不破坏当前布局
- 用户能直接在 About 中找到仓库地址

缺点：
- 入口显眼程度适中，不是强按钮样式

### 方案 B：在 About 顶部加单独按钮

优点：
- 更醒目

缺点：
- 会打破当前极简信息卡片布局
- 容易与更新卡片争夺视觉焦点

### 方案 C：把 GitHub 链接塞进更新卡片附近

优点：
- 和发布、版本来源相关

缺点：
- 语义不如放在 About 元信息区直接
- 位置不够稳定

## 选择

采用方案 A。

## 详细设计

- 在关于页 `about-meta` 中增加一行 `GitHub`
- 右侧展示可点击链接文本，优先显示 `GcsSloop/ai-gate`
- 使用标准外链：
  - `href="https://github.com/GcsSloop/ai-gate"`
  - `target="_blank"`
  - `rel="noreferrer"`
- 桌面端和浏览器端都复用浏览器跳转行为，不新增 Tauri 专属调用
- 补一个前端测试，验证：
  - 关于页存在 `GitHub` 文本
  - 链接 `href` 正确

## 影响范围

- `frontend/src/features/settings/SettingsPage.tsx`
- `frontend/src/features/settings/SettingsPage.test.tsx`
- 如有必要，补 `frontend/src/styles.css` 细节样式
