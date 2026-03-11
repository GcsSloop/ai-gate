# OpenAI Codex Submodule Design

## Goal

Convert `references/openai-codex` from a bare nested gitlink repository into a standard Git submodule.

## Decision

- Keep the existing path: `references/openai-codex`
- Keep the existing upstream remote: `https://github.com/openai/codex.git`
- Preserve the currently checked out nested repository commit as the parent repository's recorded submodule commit
- Add a standard `.gitmodules` entry and absorb the nested `.git` directory into the parent repository's `.git/modules`

## Reasoning

The repository already tracks `references/openai-codex` as a gitlink (`160000`) but lacks `.gitmodules`, so tooling treats it like an irregular nested repository. Standardizing it as a real submodule keeps the reference model intact while making clone, status, and CI behavior predictable.
