# ADR-0002: Content-addressed assets and layered retention policy

## Status

Accepted

## Context

Artwork and other binary assets arrive from many sources (scrapers, manual uploads,
generated derivatives) with varying licensing, varying trustworthiness, and varying
desired lifetimes. A single global retention rule ("keep everything" or "keep 30
days") is either wasteful or destructive depending on the asset. We also need a
storage model that de-duplicates identical bytes regardless of where or how many
times they were imported, and that makes tampering or silent substitution obvious.

## Decision

- **Storage:** every asset is stored at
  `library/assets/sha256/<aa>/<hash>/{asset.json,content.ext}` where `<hash>` is the
  exact-byte SHA-256 of `content.ext` and `<aa>` is its first two hex characters
  (fan-out directory to keep directory listings small). Two imports of identical
  bytes always resolve to the same path; there is exactly one copy. `asset.schema.json`
  requires `sha256`, `provenance`, and `license` on every asset — provenance and
  licensing are never optional or implicit.
- **Retention precedence:** policies live at `library/policies/<policy_id>.json`
  (`policy.schema.json`) and apply at exactly one scope level: **asset > role >
  system > source > global**, most specific wins, evaluated as a strict total order
  (see `../identity-and-policy.md` for the full table and worked examples).
- **Outcomes** are one of `managed`, `tracked-external`, `promote-on-approval`, or
  `quarantined`. `promote-on-approval` requires an explicit, auditable approval step;
  no code path may promote quarantined or tracked-external bytes into `managed` just
  because a profile or presentation layer selected them for display.

## Consequences

- **Easier:** de-duplication is automatic and free (it falls out of the storage
  address); auditing "why is this asset here" is a single policy lookup with a fixed
  precedence order; deleting a source or role's policy cannot accidentally widen
  retention for a more specific asset because more specific always wins.
- **Harder:** consumers must resolve a small precedence chain instead of reading one
  flat config value; content-addressing means a single bit-flip (e.g. re-compressed
  artwork) is a *new* asset, not an updated one, so "the same picture" can legitimately
  have multiple content addresses over time — this is intentional (exact-byte
  identity, not perceptual identity) and is handled via `variants` references in
  `asset.schema.json`, not by mutating a hash's content in place.
- Presentation/profile selection is a pure read of existing `AssetRef`s; it is
  schema-incapable of promoting bytes (see `canonical-profile.schema.json`), which
  keeps "what art is shown" and "what art is retained/managed" fully decoupled.
