# DMG Packaging Design

Goal: make macOS DMG generation deterministic and identical between local packaging and GitHub Actions release builds.

Decision: stop depending on transient `target/.../bundle/dmg/bundle_dmg.sh`. Use a repo-managed CLI (`create-dmg`) invoked from `scripts/desktop/notarize_macos.sh` so the layout engine is available in every clean environment.

Approach:
- add `create-dmg` to desktop dev dependencies
- update `notarize_macos.sh` to call `npx create-dmg` with the existing background, window, and icon coordinates
- keep `hdiutil create` only as an explicit last-resort fallback when `create-dmg` is unavailable
- add tests that fail if the script silently falls back in scenarios where deterministic DMG creation should happen

Success criteria:
- CI logs show `create-dmg` usage instead of only `hdiutil create`
- produced DMG includes background image and Applications drop target
- local and release outputs match
