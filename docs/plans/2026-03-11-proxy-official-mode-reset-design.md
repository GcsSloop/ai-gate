# Proxy Official Mode Reset Design

## Goal

When proxy is disabled after starting from the official Codex default mode, remove the top-level `model_provider` key instead of writing `model_provider = "openai"` back into `~/.codex/config.toml`.

## Decision

- Official-mode disable is defined as a proxy session whose previous provider is empty or `openai`.
- In that case, proxy disable should:
  - remove the top-level `model_provider` assignment
  - remove any temporary `aigate` provider definitions
  - leave unrelated provider blocks untouched
- Third-party mode keeps the existing behavior:
  - restore the original provider name
  - restore the original third-party `base_url`
  - remove any temporary `aigate` provider definitions

## Reasoning

Writing `model_provider = "openai"` back is an implementation assumption about Codex default behavior. Removing the key is closer to “return to default” and avoids forcing a hard-coded provider name into user config.

## Files

- Modify [settings_handler.go](/Users/gcssloop/WorkSpace/AIGC/codex-router/backend/internal/api/settings_handler.go)
- Modify [settings_handler_test.go](/Users/gcssloop/WorkSpace/AIGC/codex-router/backend/internal/api/settings_handler_test.go)
- Modify [README.md](/Users/gcssloop/WorkSpace/AIGC/codex-router/README.md)
- Modify [README.zh-CN.md](/Users/gcssloop/WorkSpace/AIGC/codex-router/docs/README.zh-CN.md)
