# Remove Demo Design

## Goal

Remove the static `demo` experience from the frontend so the product only exposes the existing accounts, stats, and settings flows.

## Scope

- Remove the `demo` tab from the top navigation.
- Remove the `demo` view branch from the application shell.
- Delete the `ProductDashboardDemo` feature and its dedicated test.
- Remove `demo`-only i18n text and stylesheet rules.
- Delete the `demo`-only planning documents created for that feature.

## Out of Scope

- No backend changes.
- No changes to accounts, stats, settings, proxy, or tray behavior.
- No refactor outside the files that currently reference `demo`.

## Approach

Use a deletion-only change set driven by UI tests:

1. Update `App.test.tsx` to assert the `演示` tab is absent.
2. Run the focused test and confirm it fails against the current code.
3. Remove the runtime references that still render the demo.
4. Delete the demo component, demo stylesheet block, demo test, and demo-only docs.
5. Re-run focused tests, the full frontend test suite, and the frontend build.

## Risks

- `styles.css` contains a large contiguous `demo` block. Removing the wrong range could affect unrelated layout rules.
- `App.tsx` and `App.test.tsx` already have unrelated local edits. The deletion must be surgical and preserve those in-flight changes.

## Validation

- `npm --prefix frontend run test -- App.test.tsx`
- `npm --prefix frontend run test`
- `npm --prefix frontend run build`
