# README Showcase Redesign

## Goal

Refit the Chinese and English README pages so they read like a strong open-source project landing page instead of a plain technical manual.

## Scope

- Rebuild the top-level Chinese README in `README.md`.
- Rebuild the English README in `docs/README.en.md` with the same information architecture.
- Update the compatibility landing page in `docs/README.zh-CN.md` so it points readers to the expanded Chinese and English entries.
- Add live GitHub badges for stars, forks, license, latest release, platform, and CI status.
- Expand the product-tour section to include the newly added screenshots for MCP and Skill management.

## Design

The new README should lead with product value first, then move into proof and details:

1. Hero block: project title, concise value proposition, live badges, and a short one-paragraph pitch.
2. Why AI Gate: explain the real problem space around multi-account routing, quota exhaustion, and local workflow stability.
3. Highlights: emphasize official + third-party account management, uninterrupted failover, local-first security, Skill/MCP management, statistics, and desktop usability.
4. Product tour: present the main dashboard, proxy, statistics, MCP, and Skill screenshots as product proof.
5. Quick start: give separate paths for end users and developers.
6. Architecture and safety: keep the thin-gateway and local-only boundaries explicit.
7. Codex, Skill, and MCP workflow section: explain how AI Gate fits underneath CLI, Skill workflows, and MCP operations.
8. Development and packaging: keep the operational details, but move them behind the product-facing sections.

## Risks

- Over-marketing would weaken trust, so the copy must stay concrete.
- The English README should match structure and claims without sounding machine-translated.
- Badge URLs must target the current GitHub repository and existing workflow file.
