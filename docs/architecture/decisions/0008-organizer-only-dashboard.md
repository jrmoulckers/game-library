# ADR-0008: The dashboard is an artwork organizer, not a review console

## Status

Accepted. Supersedes the dashboard-shape portions of
[ADR-0007](0007-local-dashboard.md).

## Context

ADR-0007 kept the review, policy, planning, adapter, and recovery surfaces and
moved them behind an Advanced disclosure. Running that dashboard against a real
library showed the compromise did not hold.

Ten section modules each issued their own API calls during page load, so the
audit surface cost startup time whether or not the disclosure was ever opened,
and it accounted for roughly two thirds of the rendered shell. Meanwhile the
organizer views — the part the owner actually used — were slow enough to be
unpleasant: the grid rendered artwork at full resolution over a library holding
1218 assets, media responses were sent `Cache-Control: no-store` so every
re-render re-downloaded them, and each keystroke in the search box destroyed and
rebuilt every tile.

The owner's stated purpose for the tool is narrow: keep one central artwork
library, see which assets each profile uses, and carry those profiles to other
devices. Gate reviews, policy drafts, plan tables, and duplicate triage do not
serve that purpose, and the plan/gate vocabulary described a workflow that was
never going to be exercised by a single-owner local tool.

The safety properties ADR-0007 established are still wanted. What is not wanted
is a user interface built around demonstrating them.

## Decision

- Present the dashboard as an organizer only: Library, Platform, Game, Profiles,
  and Sources. There is no Advanced console.
- Delete the overview, artwork table, identity queue, duplicates, policy,
  profile-draft-editor, plans/gates, adapters, and recovery sections along with
  the endpoints that existed solely to serve them. Gate A/B/C are removed as
  product concepts.
- Keep source setup, because configuring roots is how the organizer learns where
  artwork lives. It stays under a disclosure in the Sources view.
- Keep profile creation in the Profiles view. Choosing an asset for a profile is
  a primary organizer action and must not depend on a deleted surface.
- Serve grid artwork as bounded thumbnails from a content-hash-keyed cache, and
  reserve full-resolution bytes for explicit preview. Media responses are
  cacheable; they carry content-hash ETags and cannot go stale.
- Filter and sort the game grid without detaching nodes, so images are requested
  once per platform rather than once per keystroke.
- Retain every safety property from ADR-0007 that is about behavior rather than
  presentation: loopback-only binding, the request hardening, no account or
  telemetry, thin browser handlers over Go contracts, read-only source
  detection, metadata-driven titles, the exact Playnite-to-Steam join, and
  device-local source absence.
- Retain the CLI surface unchanged. Deterministic inventory, duplicate,
  identity, import, bundle, export, manifest, and validation operations remain
  available from `gamelib` for the cases that genuinely need them.

## Consequences

- The organizer is materially faster. Thumbnails measured 14.6x smaller than the
  originals on real covers, and search filtering no longer rebuilds the grid.
- The dashboard's surface area drops sharply: the rendered shell fell from 39KB
  to 11KB and nine browser modules and their handlers are gone. Less code is
  reachable from the browser, which narrows the loopback attack surface.
- Evidence work moves to the CLI. Anyone who wants duplicate classification or
  identity proposals runs the command, which is where the deterministic
  implementation always lived.
- Plan and gate artifacts are no longer produced by the dashboard. Existing
  artifacts on disk remain readable and are not deleted.
- No apply, publish, delete, or prune endpoint is added by this ADR. Publishing a
  profile to a device remains an owned seam and needs its own decision, because
  it writes to live user files.
