# 统计延迟卡片秒数展示实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将统计页四张请求质量卡片的延迟从毫秒改为秒，并固定保留 1 位小数，同时完成软件发布闭环。

**Architecture:** 保留后端毫秒数据协议，仅在统计页的卡片展示 helper 中进行单位转换和格式化。最近记录列表继续显示整数毫秒。发布前通过既有的无 Apple 签名 CI harness 审计 GitHub Actions 工作流，不改动仅供本地 macOS 使用的签名脚本。

**Tech Stack:** React 19、Ant Design、Vitest、Vite、GitHub Actions、GitHub CLI。

---

### Task 1：为延迟卡片补充失败回归测试

**Files:**
- Modify: `frontend/src/features/stats/StatsPage.test.tsx`
- Test: `frontend/src/features/stats/StatsPage.test.tsx`

**Step 1: Write the failing test**

在现有统计页测试中断言：321.4 毫秒显示为 `0.3 s`，880 毫秒显示为 `0.9 s`，1200 毫秒显示为 `1.2 s`，最大 / 最小显示为 `1.5 s / 0.1 s`；同时断言最近记录仍显示 `321 ms`。

**Step 2: Run test to verify it fails**

Run: `npm --prefix frontend test -- --run src/features/stats/StatsPage.test.tsx`

Expected: FAIL because the cards currently display integer millisecond values.

### Task 2：实现卡片延迟秒数格式化

**Files:**
- Modify: `frontend/src/features/stats/StatsPage.tsx:52-54`

**Step 1: Write minimal implementation**

将 `formatLatency` 的输入毫秒值除以 1000，使用本地化数字格式固定 1 位小数，并将单位从 `ms` 改为 `s`。不修改最近记录的渲染逻辑。

**Step 2: Run test to verify it passes**

Run: `npm --prefix frontend test -- --run src/features/stats/StatsPage.test.tsx`

Expected: PASS。

### Task 3：验证前端和发布脚本

**Files:**
- Check: `frontend/src/features/stats/StatsPage.tsx`
- Check: `frontend/src/features/stats/StatsPage.test.tsx`
- Check: `.github/workflows/release.yml`
- Check: `.github/workflows/dev-ci.yml`

**Step 1: Run focused and full frontend harnesses**

Run: `npm --prefix frontend test -- --run src/features/stats/StatsPage.test.tsx`

Run: `npm --prefix frontend test`

Run: `npm --prefix frontend run build`

**Step 2: Run CI/release harnesses**

Run: `bash scripts/test/release_no_apple_signing_test.sh`

Run: `bash scripts/test/release_updater_signing_key_test.sh`

Run: `bash scripts/test/release_version_helpers_test.sh && bash scripts/test/sync_release_metadata_test.sh && bash scripts/test/collect_release_assets_test.sh && bash scripts/test/release_updater_manifest_test.sh && bash scripts/test/package_local_release_test.sh && bash scripts/test/release_msvc_spub_test.sh`

**Step 3: Check diff hygiene**

Run: `git diff --check`

### Task 4：提交并执行软件发布闭环

**Files:**
- Commit: `docs/plans/2026-07-20-stats-latency-seconds-design.md`
- Commit: `docs/plans/2026-07-20-stats-latency-seconds.md`
- Commit: `frontend/src/features/stats/StatsPage.test.tsx`
- Commit: `frontend/src/features/stats/StatsPage.tsx`

**Step 1: Create scoped commits**

只暂存本次任务文件，使用中文提交信息；不带入工作区已有的后端、账户页和图片改动。

**Step 2: Rebase and push**

拉取 `origin/main` 和 tags，将工作分支 rebase 到最新 `origin/main`，重新运行本地验证后推送 `codex/stats-latency-seconds`。

**Step 3: Monitor CI and merge PR**

只监控本次推送触发的 workflow，成功后创建并以 rebase 方式合并 PR；若 CI 失败，先定位具体 job 和步骤，做最小修复后从最新 `origin/main` 重新开始。

**Step 4: Tag and monitor release**

在合并后的最新 `main` 上使用下一个版本 tag `v1.6.7`，推送 tag，并监控 release workflow 到终态；若失败，按发布闭环从最新 `main` 新建修复分支处理。
