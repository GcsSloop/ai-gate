# Skill Discovery Index Design

**Date:** 2026-04-08

## Goal

Rebuild the skill discovery flow into a full-screen experience that defaults to a sorted skill list, uses an index-only cache, refreshes in the background on open, supports explicit no-cache refresh, provides repository-page viewing and one-click install actions, and moves repository management into a secondary modal.

## Scope

- Add a discoverable skill index separate from managed/installed skills.
- Cache only index metadata, never `SKILL.md` bodies.
- Trigger a background refresh every time the discovery modal opens.
- Refresh the open page automatically after a successful background update.
- Add explicit manual refresh that bypasses cache.
- Add repository management CRUD for public GitHub and GitLab repositories.
- Keep managed skill install/uninstall behavior intact.

## Non-Goals

- Recommendation and rating systems.
- Private repository support.
- Full-text indexing of skill body content.
- Persisting raw remote skill files in cache.

## UX

### Discovery Modal

- The primary "发现技能" action opens a full-screen modal.
- Default view is the discovered skill list, not repository management.
- Top bar includes:
  - search input over cached/current index
  - refresh button
  - repository management entry
  - status text showing cached/live/updating state
- Initial open behavior:
  - load cached index immediately if present
  - show the list sorted by skill name
  - start a background refresh automatically
  - silently replace the list when the refresh completes if the modal is still open
- Manual refresh behavior:
  - ignore cache
  - fetch latest index only
  - show loading state in place
- Skill cards show:
  - name
  - summary/description
  - source platform
  - source repository name
  - source repository page link
  - install action
  - installed state

### Repository Management Modal

- Opened from inside the discovery modal as a secondary modal.
- Supports:
  - listing configured repositories
  - adding a public GitHub or GitLab repository
  - editing owner/name/branch/platform
  - removing a repository
- Successful add/edit/delete triggers a rescan and updates the discovery list when the primary modal is still open.

## Data Model

### Config

Keep repository configuration in `tooling.json`.

- Extend repo config with `platform` (`github` or `gitlab`)
- Keep:
  - owner
  - name
  - branch
  - enabled

### Discovery Cache

Store discovery cache separately from config under AI Gate data root.

- New cache file: skill discovery index JSON
- Cached payload fields:
  - fetched_at
  - repos hash/version
  - items[]
- Cached item fields:
  - stable id
  - skill name
  - description summary
  - source platform
  - source repo owner/name
  - branch
  - repository URL
  - source path within repo
  - installable collection identifier
  - managed/install status snapshot

Do not store:

- raw `SKILL.md`
- full repo tarballs
- arbitrary repository file contents beyond temporary scan-time use

## Backend Design

### New Concepts

- `discoveredSkillRecord`: index item returned to frontend
- `skillDiscoveryCache`: persisted index-only cache document
- `skillRepoPlatform`: `github` or `gitlab`

### API Shape

Add endpoints dedicated to discovery instead of overloading `/tooling/state`.

- `GET /tooling/skills/discover`
  - default behavior: may return cached index immediately
  - response includes:
    - items
    - cached
    - updating
    - fetched_at
- `POST /tooling/skills/discover/refresh`
  - forces a fresh scan
  - bypasses cache
  - updates cache on success
  - returns latest items

Extend existing repo APIs:

- `GET /tooling/skills/repos`
- `POST /tooling/skills/repos`
- `PUT /tooling/skills/repos/:platform/:owner/:name`
- `DELETE /tooling/skills/repos/:platform/:owner/:name`
- `GET /tooling/skills/repos/search?q=...`

Search behavior:

- GitHub query hits GitHub repository search API
- GitLab query hits GitLab public project search API
- frontend can filter/scope search by selected platform

### Scan Algorithm

For each configured repository:

1. Resolve repo archive or tree metadata from public source.
2. Download/read only what is needed to identify skill directories.
3. Treat each directory containing `SKILL.md` as a discovered skill.
4. Extract:
   - skill name from frontmatter/title/metadata fallback
   - summary from skill metadata / top description fallback
   - repo URL
   - relative path
5. Build collection/install identifier.
6. Merge with managed skill records to annotate installed state.
7. Sort case-insensitively by name.

### Cache Rules

- Cache only the final index metadata.
- Opening the modal:
  - read cache synchronously
  - return cached data fast
  - frontend separately triggers refresh
- Successful refresh:
  - overwrite cache atomically
  - frontend uses returned payload immediately
- Failed refresh:
  - keep previous cache
  - return an error for manual refresh
  - keep cached list visible for background refresh failures

## Frontend Design

### State

- discovery modal visibility
- repo management modal visibility
- discovery list
- discovery cache metadata
- discovery loading state
- background updating state
- manual refresh state
- selected platform/search text for repo management

### Modal Flow

- open discovery modal
- load cached/server-provided list
- start background refresh in `useEffect`
- if refresh resolves while still open, replace list and refresh visible state
- if closed before refresh completes, ignore stale result

### Sorting and Filtering

- backend returns sorted items
- frontend keeps a local secondary sort guard by `name.toLowerCase()`
- search filters visible items only

### Actions

- View: open repository page from card
- Install: install discovered skill collection into managed skills, then refresh managed state and visible discovery item state
- Refresh: force latest discovery fetch

## Testing

### Backend

- cache read/write only stores metadata
- default discover returns cached items when cache exists
- refresh bypasses cache and writes updated cache
- github repo CRUD
- gitlab repo CRUD
- scan sorts by skill name
- discover payload annotates installed state

### Frontend

- opening modal shows skill list by default
- list uses sorted cached data first
- background refresh updates the open modal silently
- manual refresh bypasses cache and shows loading animation
- card view link opens repo page
- install action triggers install API and updates state
- repository management modal supports create/read/update/delete

## Risks

- Public repository scanning needs to stay lightweight to avoid rate limits.
- GitLab path handling and branch defaults differ slightly from GitHub and must be normalized carefully.
- Installing a discovered skill needs a deterministic mapping from discovered path to managed collection path.

## Recommendation

Implement a dedicated discovery index API plus a separate cache file. This keeps the existing managed skill state stable while giving the discovery flow explicit lifecycle control for cached open, silent refresh, and no-cache refresh.
