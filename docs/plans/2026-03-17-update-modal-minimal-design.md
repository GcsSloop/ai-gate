# Update Modal Minimal Design

**Context**
- The home-page update modal currently renders the shared `UpdateCard` component.
- That shared card is also used inside the Settings `关于` tab, so direct edits would change both surfaces.
- The modal currently duplicates the outer modal title with an inner card title, shows explanatory copy that is unnecessary in-context, and uses an extra bordered card that feels heavier than the surrounding modal shell.

**Decision**
- Create a separate update-summary component for the home-page modal instead of reusing the existing about-page `UpdateCard`.
- Keep the Settings `关于` tab unchanged and continue to render the existing `UpdateCard`.
- Make the modal body visually quieter: no inner card border, no duplicated inner title, no descriptive GitHub Release sentence, no manual `检查更新` button.
- Preserve the remaining update content: current version, status, target version or latest version, publish time, release notes, download progress, and action buttons that are still relevant for the current state.

**Visual Direction**
- Match the existing settings and modal language: restrained spacing, clear hierarchy, no extra ornament.
- Keep a comfortable content inset inside the modal so the update data does not press against the modal edges.
- Avoid introducing a new visual style system; stay within the current token palette and typography.

**Non-Goals**
- Do not change the about-page update card.
- Do not change update-service behavior or the underlying desktop update flow.
- Do not remove action buttons other than the manual `检查更新` entry point requested for the modal.

**Testing**
- Add an app test proving the home update modal hides the duplicated title, description, and check button while still showing the update details.
- Keep the existing Settings test proving the `关于` tab still shows the original `UpdateCard` with its title and `检查更新` button.
