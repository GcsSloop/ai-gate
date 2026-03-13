# README Chinese-First Entry Design

**Goal:** Make the repository default README Chinese-first for the primary audience while preserving a clear English document path and avoiding long-term bilingual duplication drift.

## Chosen Approach

Adopt a split documentation entry strategy:

- Root `README.md` becomes the canonical Chinese-first entry page
- `docs/README.en.md` becomes the canonical English document
- `docs/README.zh-CN.md` becomes a lightweight compatibility entry that points readers to the root README and other key docs

## Why This Approach

This keeps the GitHub landing experience aligned with the product's actual audience without forcing permanent duplication of two full Chinese READMEs.

- Chinese users see the intended default document immediately.
- English readers still have a stable explicit path.
- Future maintenance becomes simpler because the canonical Chinese content lives in one place.

## Scope

- Rewrite root `README.md` using the current Chinese README as the source of truth
- Create `docs/README.en.md` from the current English README content
- Replace `docs/README.zh-CN.md` with a short bridge document
- Preserve current architecture diagrams, screenshots, quick start, and technical boundaries

## Non-Goals

- No product behavior changes
- No rebranding changes
- No large information architecture redesign beyond language entry routing

## Risks

- Existing links that point to `docs/README.zh-CN.md` may now land on a short bridge page instead of the full Chinese content.
- Relative asset and doc links can break when content moves from root to `docs/`.

## Risk Controls

- Keep the bridge page explicit and useful instead of empty.
- Re-check all relative paths after moving English content to `docs/README.en.md`.
- Preserve section parity between the new Chinese root README and the English document.
