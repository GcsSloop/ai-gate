# All-Branch Dev CI Trigger Design

## Context
当前 `.github/workflows/dev-ci.yml` 只在 `dev` 分支的 `push` 和指向 `dev` 的 `pull_request` 上运行。这会导致其他功能分支即使已经 push，也不会自动执行现有的后端测试/构建和前端测试/构建。

## Decision
保持现有 `dev-ci` 的 job 内容不变，只把 `push` 触发条件从单一 `dev` 分支扩展为所有分支。`pull_request` 仍然只保留针对 `dev` 的校验，`release.yml` 继续只处理 tag 发布。

## Why This Shape
- 这是最小改动，不会改变现有 job 结构。
- 所有分支 push 都能拿到同一套快速反馈。
- 不会把桌面发布或 tag release 逻辑错误地下沉到普通分支。
- PR 校验范围保持稳定，不影响现有分支策略。

## Expected Behavior
- 任意分支 `push`：执行 `backend-test-build` 和 `frontend-test-build`。
- 目标为 `dev` 的 PR：继续执行同一套 CI。
- `v*` tag：继续走 `release.yml`，不受影响。
