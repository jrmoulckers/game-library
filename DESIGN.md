# Design

<!-- impeccable:design-schema 1 -->

## Direction

The dashboard follows the familiar organizer canon established by Playnite,
LaunchBox, and Jellyfin: a compact dark navigation shell opens onto a bright,
artwork-dominant collection canvas. Platform mosaics, cover grids, and
role-specific previews make the library recognizable before any metadata is
read. Advanced evidence tools retain their precision but recede behind one
disclosure.

## Color

- `#15171c` anchors the persistent navigation shell.
- `#f3f4f6` and `#ffffff` separate the collection canvas from artwork objects.
- `#476bd8` identifies primary actions; `#0b5fd6` remains the keyboard focus
  color.
- Green, amber, and red are paired with explicit text for success, warning, and
  blocking states; color never carries state alone.
- Artwork thumbnails retain their source color without tinting the surrounding
  interface.

## Typography

Use the platform system sans stack for all interface copy and controls.
Monospace is limited to paths, hashes, and technical identifiers inside
Advanced. Large, tightly tracked route headings contrast with compact game
labels and readable metadata chips.

## Layout

Desktop uses a sticky sidebar and broad collection canvas. Library cards hold
four-image mosaics, platform routes use responsive cover grids, and game routes
pair large true-ratio artwork with a facts rail. At Steam Deck-like sizes the
sidebar becomes a four-destination strip, grids reduce cleanly, and game assets
stack without changing reading or focus order.

## Components and States

- Platform cards, game covers, profile previews, and source rows are the only
  raised objects; artwork is never buried in nested cards.
- Buttons and controls share a 44px primary target floor, high-contrast borders,
  and a visible three-pixel focus ring.
- Cover and artwork previews retain meaningful alternative text, labeled
  broken-preview states, and a keyboard-operable native dialog.
- Metadata facts are compact text chips for dimensions, aspect, type, size,
  source, copy count, and profile use.
- Missing roles are explicit text, never empty placeholders or color alone.
- Title metadata resolves progressively from local caches. While it loads, the
  library says so plainly; unavailable metadata leaves an honest labeled
  identity placeholder rather than an error or invented name.
- Supported sources absent from the current device use neutral gray status and
  explanatory copy. They never create empty platform cards or red warnings.
- Every Advanced table keeps its caption, labeled headers, and empty row.
- Long or optional detail uses native `details`/`summary`.
- Loading, success, conflict, empty, disabled, and error states remain visible
  in persistent status regions rather than transient toast-only feedback.
- Media always has a textual identity and a labeled broken-preview state.

## Motion

Cover and platform objects use one short lift response to reinforce
clickability. Navigation, dialogs, scan updates, and disclosures otherwise use
native browser behavior. Reduced-motion preferences remove the lift.

## Accessibility

Target WCAG 2.2 AA with landmarks, skip navigation, native controls, visible
focus, keyboard/gamepad operation, live status regions, meaningful image text,
non-color state labels, 400% zoom reflow, 200% text scaling, and Steam Deck
viewport support.
