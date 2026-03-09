# Tray Stability And Plan Cleanup Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 修复桌面端托盘菜单自动关闭与偶发 abort，并清理 `docs/plans/` 中与薄网关现状冲突的旧说明。

**Architecture:** 删除托盘后台轮询和点击时菜单重建，把菜单刷新收敛到启动后和菜单动作后两个时机。退出冲突不再在退出路径里向 webview 注入脚本，避免事件循环生命周期竞争。文档层面统一移除厚模式、fallback、`THIN_GATEWAY_MODE` 相关遗留描述。

**Tech Stack:** Rust + Tauri 2，Markdown 文档，Cargo tests

---

### Task 1: Stabilize tray event flow

**Files:**
- Modify: `desktop/src-tauri/src/main.rs`
- Test: `desktop/src-tauri/src/main.rs`

**Step 1: Write the failing test**

添加针对新辅助函数的测试，锁定：
- 托盘点击不应触发菜单刷新
- 菜单动作后应触发菜单刷新
- 退出冲突不再要求前端注入事件

**Step 2: Run test to verify it fails**

Run: `cd desktop/src-tauri && cargo test tray_ -q`
Expected: FAIL because helper functions do not exist yet.

**Step 3: Write minimal implementation**

- 删除 `start_tray_sync_task`
- 删除 `.on_tray_icon_event(...)` 中的 `refresh_tray_menu`
- 启动完成后仅刷新一次托盘菜单
- 菜单动作执行后保留 `refresh_tray_menu`
- 用辅助函数封装“哪些动作后需要刷新菜单”
- 删除 `emit_exit_conflict`，退出冲突改为 `show_main_window + log`

**Step 4: Run test to verify it passes**

Run: `cd desktop/src-tauri && cargo test tray_ -q`
Expected: PASS

**Step 5: Commit**

```bash
git add desktop/src-tauri/src/main.rs
git commit -m "fix: stabilize tray menu lifecycle"
```

### Task 2: Verify desktop build still works

**Files:**
- Modify: none unless build fails

**Step 1: Run focused desktop tests**

Run: `cd desktop/src-tauri && cargo test -q`
Expected: PASS

**Step 2: Run desktop build**

Run: `npm --prefix desktop run tauri build -- --target universal-apple-darwin`
Expected: PASS and bundle artifacts generated.

**Step 3: Commit if needed**

```bash
git add desktop/src-tauri
git commit -m "test: verify tray stability build"
```

### Task 3: Clean conflicting historical plan docs

**Files:**
- Modify: `docs/plans/2026-03-09-thin-gateway-official-mirror-plan.md`
- Modify: `docs/plans/2026-03-09-thin-gateway-third-party-responses.md`
- Modify: any other `docs/plans/*.md` still claiming thick mode, fallback, or `THIN_GATEWAY_MODE`

**Step 1: Write the failing check**

Run a search to identify conflicting phrases:

```bash
rg -n "THIN_GATEWAY_MODE|allow_chat_fallback|chat fallback|responses/compact|input_items|thin gateway mode" docs/plans
```

Expected: Matches found in old plans.

**Step 2: Write minimal doc edits**

- 标注这些文档为“历史方案，已被薄网关单一路径取代”
- 删除或更正仍会误导实现的步骤和回滚说明
- 保留必要的历史背景，但不再把旧行为写成当前方案

**Step 3: Verify the check**

Run:

```bash
rg -n "THIN_GATEWAY_MODE|allow_chat_fallback|chat fallback" docs/plans
```

Expected: no misleading current-tense matches outside clearly marked historical context.

**Step 4: Commit**

```bash
git add docs/plans
git commit -m "docs: mark legacy gateway plans as historical"
```

### Task 4: Final verification

**Files:**
- Modify: none unless verification exposes issues

**Step 1: Run backend verification**

Run: `cd backend && go test ./... -count=1`
Expected: PASS

**Step 2: Run frontend verification**

Run: `npm --prefix frontend test -- AccountsPage`
Expected: PASS

**Step 3: Run desktop verification**

Run: `cd desktop/src-tauri && cargo test -q`
Expected: PASS

**Step 4: Commit final fixes if required**

```bash
git add .
git commit -m "chore: finalize tray stability cleanup"
```

