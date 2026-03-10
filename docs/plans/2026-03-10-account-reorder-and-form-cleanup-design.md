# Account Reorder And Form Cleanup Design

## Scope

- Replace HTML5 drag/drop account sorting with pointer-driven live reordering on all clients.
- Shrink source icon presentation in the account editor and account cards.
- Remove the `原生 /responses` switch from third-party creation and account editing flows.

## Decisions

### 1. Overlay-based live reorder

Use a drag handle as the only reorder entry point, but switch the interaction model to an overlay-based sortable list backed by `@dnd-kit`. The dragged account card should detach from normal layout and follow the pointer as a floating overlay. The original list position must keep a stable placeholder footprint so surrounding cards are pushed away in real time instead of waiting for pointer release.

The in-memory list should reorder continuously during drag-over, and priorities should still persist only once on drag end. If persistence fails, revert to the pre-drag order and show a warning.

### 2. Form simplification

The product boundary already requires third-party providers to support native `/responses`, so the explicit switch is redundant and misleading. Remove it from both create and edit dialogs. Submit `supports_responses: true` implicitly for compatible accounts.

### 3. Icon sizing

Reduce visual weight by shrinking source icons in both the card list and select options. Keep the icon container square and preserve cover-fit behavior so the three supported providers remain visually recognizable.

## Test strategy

- Update account page tests so create/edit flows no longer expect the switch.
- Add a drag test that verifies the dragged card enters placeholder mode, a floating overlay is rendered, the list reorders during drag, and priorities persist on release.
