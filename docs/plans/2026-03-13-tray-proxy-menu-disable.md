# Tray Proxy Menu Disable Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make the desktop tray menu keep both proxy actions visible while disabling the invalid one based on the current proxy state.

**Architecture:** Keep the current tray menu structure and backend action flow intact. Extract a small pure helper for proxy menu enabled-state mapping, test that helper first, then wire the helper into the tray menu builder with Tauri `MenuItemBuilder`.

**Tech Stack:** Rust, Tauri 2 tray/menu APIs, existing unit tests in `desktop/src-tauri/src/main.rs`

---

### Task 1: Add failing tests for proxy menu enabled-state mapping

**Files:**
- Modify: `desktop/src-tauri/src/main.rs`
- Test: `desktop/src-tauri/src/main.rs`

**Step 1: Write the failing test**

Add two tests in the existing test module:

```rust
#[test]
fn proxy_menu_states_disable_enable_when_proxy_is_active() {
    assert_eq!(proxy_menu_enabled_states(true), (false, true));
}

#[test]
fn proxy_menu_states_disable_disable_when_proxy_is_inactive() {
    assert_eq!(proxy_menu_enabled_states(false), (true, false));
}
```

**Step 2: Run test to verify it fails**

Run: `cd desktop/src-tauri && cargo test proxy_menu_states -- --nocapture`
Expected: FAIL because `proxy_menu_enabled_states` does not exist yet.

**Step 3: Write minimal implementation**

Add a pure helper near tray menu code:

```rust
fn proxy_menu_enabled_states(proxy_enabled: bool) -> (bool, bool) {
    if proxy_enabled {
        (false, true)
    } else {
        (true, false)
    }
}
```

**Step 4: Run test to verify it passes**

Run: `cd desktop/src-tauri && cargo test proxy_menu_states -- --nocapture`
Expected: PASS.

### Task 2: Use the helper in the tray menu builder

**Files:**
- Modify: `desktop/src-tauri/src/main.rs`

**Step 1: Write the failing behavior expectation**

The helper tests from Task 1 define the intended state mapping. No extra UI-structure test is needed.

**Step 2: Verify red state is already observed**

Use the failed Task 1 run as the RED proof.

**Step 3: Write minimal implementation**

Update `build_tray_menu(...)` to use `MenuItemBuilder::with_id(...).enabled(...)` for:
- `MENU_PROXY_ENABLE`
- `MENU_PROXY_DISABLE`

Pseudo-implementation:

```rust
let (enable_proxy_enabled, disable_proxy_enabled) =
    proxy_menu_enabled_states(tray_state.proxy.enabled);

let enable_proxy_item = MenuItemBuilder::with_id(MENU_PROXY_ENABLE, "开启代理")
    .enabled(enable_proxy_enabled)
    .build(app)?;
let disable_proxy_item = MenuItemBuilder::with_id(MENU_PROXY_DISABLE, "关闭代理")
    .enabled(disable_proxy_enabled)
    .build(app)?;
```

Then append them using `.item(&enable_proxy_item)` and `.item(&disable_proxy_item)`.

**Step 4: Run focused tests**

Run: `cd desktop/src-tauri && cargo test proxy_menu_states tray_refresh_runs_for_stateful_actions -- --nocapture`
Expected: PASS.

### Task 3: Run broader verification

**Files:**
- Modify: `desktop/src-tauri/src/main.rs`

**Step 1: Run desktop Rust tests**

Run: `cd desktop/src-tauri && cargo test -- --nocapture`
Expected: PASS.

**Step 2: Run diff hygiene check**

Run: `git diff --check`
Expected: no output.

**Step 3: Commit**

```bash
git add docs/plans/2026-03-13-tray-proxy-menu-disable-design.md docs/plans/2026-03-13-tray-proxy-menu-disable.md desktop/src-tauri/src/main.rs
git commit -m "feat: disable invalid tray proxy actions"
```
