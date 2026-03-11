---
name: release-rebase-pr-tag-loop
description: Use when the user wants a full GitHub release loop for this repository, including rebasing a work branch onto remote main, pushing, monitoring CI, creating and merging a PR, tagging main, pushing the tag, and monitoring release workflows with automatic retry after CI fixes.
---

# Release Rebase PR Tag Loop

## Overview

Run the repository's full delivery loop in two phases:

1. integration loop on the working branch
2. release loop on `main`

If CI fails in either phase, fix the smallest concrete issue, then restart from phase 1 against the latest remote `main`.

## When To Use

- The user asks to finish a branch and carry it through PR, merge, tag, and release validation.
- The user wants CI monitored and requires automatic recovery when a workflow fails.
- The user explicitly wants `rebase`-based integration instead of merge commits.

Do not use this skill for local-only commits, draft-only work, or releases that intentionally skip CI.

## Preconditions

- Remote `origin` is configured and reachable.
- The operator has permission to push branches, open PRs, merge PRs, create tags, and push tags.
- GitHub Actions results can be queried from the current environment.
- The repository policy allows `rebase` and fast-forward style history management.

## Workflow

### Phase 1: Integration Loop

1. Ensure the correct working branch is checked out.
2. Fetch remote `main` and tags.
3. Rebase the working branch onto `origin/main`.
4. Resolve conflicts carefully without dropping user changes.
5. Run the required local verification for the touched scope before pushing.
6. Commit remaining intended changes if needed.
7. Push the branch.
8. Monitor branch CI until it succeeds or fails.
9. If CI fails:
   - identify the failing workflow, job, and concrete error
   - implement the smallest justified fix
   - commit the fix
   - restart from step 2
10. If CI succeeds:
   - create the PR if it does not already exist
   - merge it using the repository's required method, preferring rebase or fast-forward compatible history

### Phase 2: Release Loop

1. Switch to `main`.
2. Pull the latest remote `main` with `--ff-only`.
3. Create the next version tag requested by the user.
4. Push the tag.
5. Monitor the release workflows triggered by the tag.
6. If release CI fails:
   - diagnose the failing job and root cause
   - create a repair branch from the latest `main`
   - implement the smallest justified CI or packaging fix
   - commit and push the repair branch
   - restart from Phase 1
7. If release CI succeeds, stop.

## Operating Rules

- Always start retries from the latest remote `main`; do not continue from a stale local base.
- Prefer `git pull --ff-only` on `main`.
- Prefer `git rebase origin/main` for feature branches.
- Do not create merge commits unless the repository policy explicitly requires them.
- When CI fails, quote the exact failing step and fix only that failure first.
- After every fix, rerun the relevant local verification before pushing.
- Keep commits scoped: feature/fix commits separate from CI-only repair commits when practical.

## Monitoring Guidance

- Check the specific run triggered by the most recent push or tag, not an older run on the same branch.
- Monitor until terminal state: `success`, `failure`, or `cancelled`.
- On failure, inspect the failing job log before changing code.
- If multiple workflows run, prioritize required checks first, then release/package jobs.

## Command Pattern

Typical command sequence:

```bash
git fetch origin main --tags
git rebase origin/main
git push -u origin <branch>
```

After branch CI is green:

```bash
git checkout main
git pull --ff-only origin main
git tag <version>
git push origin <version>
```

Use GitHub CLI, MCP, or repository-approved API calls to:

- inspect workflow runs
- create and merge PRs
- monitor release workflows

## Response Format

When using this skill, report progress in this order:

1. current phase
2. exact git/CI action being taken
3. current blocker, if any
4. restart point when a failure forces a retry
5. final merge result
6. final tag and release result
