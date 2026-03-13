# README Architecture Refresh Design

**Goal:** Enrich both public README entry points with clearer architecture information so Chinese-first users can quickly understand what AI Gate is, how requests flow, and what local safety boundaries it enforces.

## Scope

- Update `/README.md`
- Update `/docs/README.zh-CN.md`
- Keep the current product boundary intact: local-first, thin gateway, native `/responses`, no protocol emulation
- Add architecture-focused documentation only; no product behavior changes

## Chosen Approach

Use a dual-diagram documentation layout in both README files:

1. A system architecture diagram for component relationships
2. A request flow diagram for runtime behavior
3. A component responsibility section for quick scanning
4. A data and safety boundary section to explain where config, auth, and audit data live and why the project stays local-first

## Why This Approach

This balances product communication and engineering clarity.

- A single diagram is too abstract and does not explain runtime behavior.
- A large README rewrite would introduce avoidable copy risk and disturb the current bilingual structure.
- Dual diagrams plus short sections give enough detail for users, contributors, and evaluators without turning the README into internal design docs.

## Content Changes

### README.md

- Expand `Architecture` from a single Mermaid graph into:
  - system architecture diagram
  - request flow diagram
  - component responsibilities
  - data and safety boundary
- Keep existing screenshots and quick-start sections
- Preserve current English-first tone while making the runtime boundary more explicit

### docs/README.zh-CN.md

- Expand `架构概览` into a fuller Chinese-first explanation:
  - system architecture diagram
  - request flow diagram
  - component responsibilities
  - data and safety boundary
- Make the explanations more directly useful for Chinese users evaluating safety, low-friction setup, and account switching

## Non-Goals

- No change to screenshots
- No change to installation flow
- No change to product claims beyond clarifying current behavior
- No replacement of README with a full marketing page

## Risks

- README claims could drift from implementation if the language becomes too broad.
- Mermaid diagrams could become noisy if too detailed.

## Risk Controls

- Reuse only concepts already present in the repo and current README
- Keep diagrams at system level and avoid endpoint-by-endpoint detail
- Keep unsupported behavior explicitly listed so the boundary remains crisp

## Validation

- Markdown renders cleanly in plain text and GitHub-style viewers
- Mermaid syntax is valid
- English and Chinese versions stay semantically aligned
