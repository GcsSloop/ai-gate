# README Quick Start Audience Split Design

**Goal:** Split the README quick start into clear end-user and developer paths, while adding a stable latest-release download entry for ordinary users.

## Chosen Approach

Use two explicit quick-start audiences in both Chinese and English README files:

- End users: download the desktop client from the latest GitHub release and follow account/proxy steps
- Developers: use local environment setup and run backend/frontend/desktop from source

## Why This Approach

The existing quick start mixes packaged-client users and contributors. That increases friction for both groups.

A split structure keeps the top-level guidance simple:
- ordinary users immediately see where to download the app
- developers immediately see source-based setup steps
- the release link stays maintenance-free by pointing to `/releases/latest`

## Scope

- Update `/README.md`
- Update `/docs/README.en.md`
- Preserve all existing architecture and product-boundary content
- Do not change the Chinese compatibility bridge page

## Non-Goals

- No code changes
- No packaging changes
- No README top-banner redesign
