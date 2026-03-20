# Usage Refresh On Network And Resume

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Trigger an immediate account usage refresh when the app comes back online or appears to resume after being hidden or sleeping.

**Architecture:** Keep the refresh orchestration in `frontend/src/App.tsx` so it works even when the accounts page is not currently visible. Reuse the existing backend `POST /accounts/usage/refresh` endpoint, then bump the existing `accountsSyncToken` so mounted pages silently reload the latest usage snapshots.

**Tech Stack:** React, Vitest, existing REST API endpoints in the frontend shell.

---

## Design Summary

- Add a debounced immediate usage refresh queue in `App`.
- Coalesce concurrent refresh triggers so online/resume bursts do not fan out into repeated backend calls.
- Trigger the queue from:
  - `window.online`
  - `document.visibilitychange` when the page returns from a sufficiently long hidden period
  - a lightweight timer-gap detector as a fallback for sleep/wake cases that do not emit visibility transitions cleanly
- Keep failures silent and rely on the next trigger or normal polling to retry.
- Reuse `accountsSyncToken` after each immediate refresh so existing pages update without a separate state channel.

## Verification

- `npm --prefix frontend test -- --run src/App.test.tsx`
- `npm --prefix frontend test -- --run src/features/accounts/AccountsPage.test.tsx`
- `npm --prefix frontend test`
- `npm --prefix frontend run build`
