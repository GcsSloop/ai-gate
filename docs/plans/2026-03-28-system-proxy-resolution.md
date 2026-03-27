# System Proxy Resolution Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make `system` upstream proxy mode read the actual OS system proxy on macOS and Windows instead of only using environment variables.

**Architecture:** Keep the existing `netproxy` transport as the single proxy decision point. Add a platform-aware system proxy resolver behind `ResolveProxy`, with Linux and unsupported environments falling back to `http.ProxyFromEnvironment`.

**Tech Stack:** Go, standard library `net/http`, platform commands on macOS, Windows registry / command helpers, existing backend tests.

---

### Task 1: Add failing tests for system proxy resolution

**Files:**
- Modify: `backend/internal/netproxy/transport_test.go`

**Step 1: Write failing tests**
- Add a test proving `system` mode uses an injected system proxy resolver result instead of environment variables.
- Add a test proving `system` mode falls back to environment variables when system proxy lookup returns nothing.

**Step 2: Run tests to verify failure**
Run: `cd backend && go test ./internal/netproxy -run 'TestResolveProxyUsesSystemProxyResolver|TestResolveProxySystemModeFallsBackToEnvironment'`
Expected: FAIL because no injectable system proxy resolver exists yet.

**Step 3: Commit after green**
```bash
git add backend/internal/netproxy/transport_test.go backend/internal/netproxy/*.go
git commit -m "feat(netproxy): resolve system proxy from os settings"
```

### Task 2: Implement platform-aware system proxy resolution

**Files:**
- Modify: `backend/internal/netproxy/transport.go`
- Create: `backend/internal/netproxy/system_proxy.go`
- Create: `backend/internal/netproxy/system_proxy_darwin.go`
- Create: `backend/internal/netproxy/system_proxy_windows.go`
- Create: `backend/internal/netproxy/system_proxy_other.go`

**Step 1: Add resolver abstraction**
- Introduce a package-level `systemProxyResolver` function variable for test injection.
- Keep `ResolveProxy` as the only public decision point.

**Step 2: Implement system mode behavior**
- In `system` mode, try OS proxy lookup first.
- If system lookup yields a valid proxy URL, return it.
- If lookup is unavailable or empty, fall back to `http.ProxyFromEnvironment`.

**Step 3: Implement macOS resolver**
- Read the active network service proxy via `scutil --proxy`.
- Support HTTP and HTTPS proxies.
- Respect enabled flags and host/port presence.
- Prefer HTTPS proxy for `https` requests.

**Step 4: Implement Windows resolver**
- Read `ProxyEnable` and `ProxyServer` from `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`.
- Support both per-scheme and single proxy formats.
- Prefer HTTPS proxy for `https` requests.

**Step 5: Implement fallback resolver for unsupported platforms**
- Return nil so Linux/others keep environment behavior.

### Task 3: Verify integrated behavior

**Files:**
- Modify only if needed: existing tests

**Step 1: Run targeted backend tests**
Run: `cd backend && go test ./internal/netproxy ./internal/usagedrv/... ./internal/bootstrap`
Expected: PASS

**Step 2: Run broader API/provider smoke tests**
Run: `cd backend && go test ./internal/api ./internal/providers/...`
Expected: PASS

**Step 3: Review diff**
Run: `git diff --stat`
Expected: only netproxy-related implementation and tests.
