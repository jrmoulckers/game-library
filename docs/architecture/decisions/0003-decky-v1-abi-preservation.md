# ADR-0003: Preserve the Decky v1 profile ABI as a generated legacy surface

## Status

Accepted

## Context

An existing Decky plugin reads profiles directly from `profiles/<id>.json` with a
specific, narrow shape (`version`, `id`, `name`, optional `description`, `artwork`
as string-or-null, `mods` as a list of per-game selections). That plugin cannot be
changed as part of this plan, and its on-disk contract cannot silently drift —
any accidental shape change breaks a running device integration outside this
repository's control.

At the same time, the canonical catalog needs richer profile data than the legacy
shape can express: content-addressed artwork references instead of bare strings,
theme links, and richer provenance — none of which the legacy ABI has room for
without breaking it.

## Decision

- Freeze the legacy shape as **`decky-profile-v1.schema.json`**, matching the
  existing plugin exactly: `version` is `const 1`; `id` is the filename stem and
  must match `^[a-z0-9][a-z0-9._-]*$`; `id`/`name` required; `description` optional
  (omitted, never present-as-null); `artwork` is `string | null` and **required**
  (`null` explicitly means "no art change", never "unset"); `mods` is a list of
  `{game, set}` unique per `game`, where each entry is a **complete set** (applying
  it fully replaces prior selections for that game — no merge/partial semantics).
  `deck-default` and `steam-default` are reserved ids that are always retained;
  `steam-default` is retained specifically as the **empty marker** profile
  (`mods: []`, `artwork: null`).
- Author richer data in **`canonical-profile.schema.json`** under
  `library/canonical/profiles/<id>.json` — same `id`/reserved-id conventions, but
  `artwork` is a full `AssetRef` (or `null`) and profiles may carry a `theme_ref`.
- The legacy `profiles/<id>.json` tree is **generated, never hand-authored**: a
  generator reads the canonical profile and deterministically produces the exact
  v1 shape 1:1, sharing the same `id`. `artwork/` and `mods/` at the tree root are
  likewise generated legacy roots, materialized from `library/assets/**` and
  `library/canonical/mods/**` respectively.

## Consequences

- **Easier:** the existing Decky plugin needs zero changes and gets no surprises;
  canonical data can evolve (new fields, richer references) without ever touching
  the frozen ABI; the frozen shape is small enough to validate trivially on every
  generation run.
- **Harder:** every canonical→legacy field mapping (e.g. `AssetRef` → legacy
  artwork string) must be defined and kept stable in the generator, documented in
  `../adapters.md`; a genuinely breaking legacy need would require a new `version: 2`
  ABI and a migration, never an in-place edit of v1.
- Because the legacy tree is generated, it is also disposable and reproducible: it
  can always be regenerated from `library/canonical/**` plus `library/assets/**`,
  which is what makes it safe to treat as a generated bundle (ADR-0005) rather than
  a second source of truth.
