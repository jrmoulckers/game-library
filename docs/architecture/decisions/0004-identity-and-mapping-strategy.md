# ADR-0004: Identity model and Playnite as adapter-only mapping

- Status: Accepted
- Date: 2026-08-09
- Owner: jrmoulckers

## Context

Games are addressed differently by every system that touches this library: Steam
has a stable numeric AppID; other PC storefronts (GOG, Epic, EA, Ubisoft, itch,
Humble, ...) have their own opaque IDs; retro games have no universal registry at
all — the only thing that is ever exactly identical between two copies of "the same
ROM" is its byte content. Playnite additionally assigns every game a local database
UUID, but that UUID is a property of one Playnite installation's local database, not
a property of the game — it is not portable, not stable across reinstalls in the
general case, and is meaningless outside Playnite.

Treating any single vendor's local identifier as canonical identity would either
tie the whole catalog's stability to that vendor's database internals, or (for
retro) give no identity mechanism at all for byte-identical content arriving under
different filenames from different sources.

## Decision

Canonical identity is:

- **PC / Steam:** numeric Steam AppID (`game.schema.json`'s `pc.appid`).
- **PC / other storefronts:** the storefront's own opaque `storefront_id`, scoped by
  `store`.
- **Retro:** a **logical** identity of `system` + `slug` (the game as a concept,
  e.g. "Chrono Trigger" on "snes"), plus, for exact files, the **full raw SHA-256**
  of the release artifact's bytes (`retro-release.schema.json`) — never a truncated
  hash, never a per-block/rolling hash. Filenames are recorded as
  `filename_aliases` for operator recognition only and are never identity.
- **Playnite UUIDs are adapter-only.** They are never embedded in `game.schema.json`
  and never treated as canonical identity anywhere. They live exclusively in
  `library/mappings/playnite/<uuid>.json` (`playnite-mapping.schema.json`), a
  translation record with `adapter_only: true` (fixed `const true`) pointing at a
  canonical `game_ref`, carrying a `confidence` (`exact` | `fuzzy` | `manual`). If
  Playnite regenerates or loses a UUID, only the mapping file is affected — the
  canonical game and all its assets, releases, and history are untouched.
- Assets are identified purely by exact-byte SHA-256 (ADR-0002); there is no
  separate "asset identity" scheme layered on top.

## Consequences

- **Easier:** the canonical catalog is stable even if Playnite's database is wiped,
  migrated, or replaced entirely — mapping files are cheap to regenerate/re-derive
  and are explicitly disposable; retro identity survives re-dumping/renaming/
  re-organizing files as long as bytes match; multiple storefront copies of the
  same conceptual game can coexist without collision because each carries its own
  `store` + id.
- **Harder:** matching a Playnite entry, a retro file, or a storefront listing to
  the correct canonical game is a reconciliation problem that must be solved
  explicitly (inventory scans, see `../identity-and-policy.md` and
  `inventory-observation.schema.json`), not assumed by construction; a single
  logical retro game may have several valid `releases[]` (regions, revisions,
  translations) that all need distinct policy/retention handling.
- Because Playnite mappings are adapter-only, **Playnite's own database is never
  written by this system** (see ADR-0006) — the relationship is strictly read +
  map, never write-back.

## Evidence

The narrow join condition is tested directly by
[`internal/media/playnite_identity_test.go`](../../../internal/media/playnite_identity_test.go)
and [`internal/review/identity_test.go`](../../../internal/review/identity_test.go), including
the negative cases: a title match alone must not create an identity edge, and a Playnite entry
without a reviewed Steam plugin ID and numeric `GameId` must not join.

The mapping shape is fixed by
[`schemas/v1/playnite-mapping.schema.json`](../../../schemas/v1/playnite-mapping.schema.json).

Falsifiable by: any identity edge created from title similarity.
