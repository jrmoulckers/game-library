# Design

<!-- impeccable:design-schema 1 -->

## Direction

The dashboard is an artwork-led library workbench, not a storefront. Neutral
graphite and paper-like surfaces keep evidence, state, and operator decisions
legible; artwork is the primary source of color. Familiar controls and dense
tables support repeat work without a gamer-neon aesthetic or decorative chrome.

## Color

- `#14171a` anchors the header and primary ink.
- `#f4f4f2`, `#ffffff`, and `#ebebe8` separate the workspace, content, and
  secondary tools.
- `#0b5fd6` is reserved for keyboard focus and active browser affordances.
- Green, amber, and red are paired with explicit text for success, warning, and
  blocking states; color never carries state alone.
- Artwork thumbnails retain their source color without tinting the surrounding
  interface.

## Typography

Use the platform system sans stack for all interface copy and controls.
Monospace is limited to paths, hashes, and technical identifiers. The hierarchy
is deliberately compact: one strong page title, clear section headings, and
plain labels suited to desktop and Steam Deck browser density.

## Layout

Desktop uses a sticky stage rail and one broad work column. At narrow widths or
Steam Deck-like height, the rail becomes a wrapped stage switcher and all
content stacks into a single column. Data tables retain native semantics inside
labeled keyboard-scrollable regions. Forms reflow without changing reading or
focus order.

## Components and States

- Sections use thin borders and restrained corner radii rather than nested
  cards or decorative shadows.
- Buttons and controls share a 44px primary target floor, high-contrast borders,
  and a visible three-pixel focus ring.
- Every table has a caption, labeled headers, and an explicit empty row.
- Long or optional detail uses native `details`/`summary`.
- Loading, success, conflict, empty, disabled, and error states remain visible
  in persistent status regions rather than transient toast-only feedback.
- Media always has a textual identity and a labeled broken-preview state.

## Motion

There is no decorative motion. Navigation, table updates, and disclosures use
native browser behavior. Reduced-motion preferences suppress any incidental
transition or animation introduced by browser/platform defaults.

## Accessibility

Target WCAG 2.2 AA with landmarks, skip navigation, native controls, visible
focus, keyboard/gamepad operation, live status regions, meaningful image text,
non-color state labels, 400% zoom reflow, 200% text scaling, and Steam Deck
viewport support.
