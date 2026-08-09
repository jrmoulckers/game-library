# Identity and Retention Policy

See ADR-0002 and ADR-0004 for the decisions behind this document; this is the
practical reference for both models.

## Identity model

| Domain | Canonical identity | Schema | Notes |
|---|---|---|---|
| PC / Steam | Steam AppID (integer) | `game.schema.json` (`pc.appid`) | Stable, vendor-issued, first-class. |
| PC / other storefronts | Storefront's own opaque ID | `game.schema.json` (`pc.storefront_id`) | Scoped by `pc.store`; format is whatever that storefront uses. |
| Retro (logical game) | `system` + `slug` | `game.schema.json` (`retro.system`, `retro.slug`) | The concept "this game on this system", independent of any one file. |
| Retro (exact release) | Full raw SHA-256 of the artifact | `retro-release.schema.json` (`sha256`) | Exact bytes. Never a truncated/rolling/partial hash. |
| Asset | Exact-byte SHA-256 of `content.<ext>` | `asset.schema.json` (`sha256`) | De-duplicates automatically; a re-encode is a new asset, not an edit. |
| Playnite entry | **Not canonical** — UUID kept only in adapter-only mapping | `playnite-mapping.schema.json` | See below. |

### Filenames and aliases are never identity

Retro `filename_aliases` (on both `game.schema.json`'s `retro` block and
`retro-release.schema.json`) exist purely so a human operator recognizes what
they're looking at. Matching, de-duplication, and policy decisions are always made
on `sha256`, never on filename, extension, or path.

### Playnite UUIDs are adapter-only

A Playnite UUID is a property of one Playnite installation's local database. It is
recorded **only** in `library/mappings/playnite/<uuid>.json`
(`playnite-mapping.schema.json`), which:

- points at a canonical `game_ref` (Steam AppID, storefront ID, or retro slug),
- is flagged `adapter_only: true` (schema-fixed `const true`) so nothing can
  mistake it for identity,
- carries a `confidence` of `exact`, `fuzzy`, or `manual`, so low-confidence
  auto-matches are distinguishable from operator-confirmed ones.

If Playnite's database is rebuilt and issues a new UUID for the "same" game, only
the mapping file needs to change — the canonical game, its assets, and its
history are unaffected.

## Retention policy precedence

Policies (`policy.schema.json`) live at `library/policies/<policy_id>.json`. Each
policy applies at **exactly one** scope level:

```
asset  >  role  >  system  >  source  >  global
(most specific)                          (least specific, fallback)
```

- `global` policies have no `selector` and apply when nothing more specific matches.
- `source`, `system`, and `role` policies apply to everything from that source,
  system, or asset role, respectively (`selector` required).
- `asset` policies apply to one exact SHA-256 and always win over every other
  level, no matter what a broader policy says.

Because each level's selector is unique, there is never a tie to break — for any
given piece of data there is exactly one applicable policy per level, and the
first one found walking the list above (asset first) is authoritative.

### Outcomes

| Outcome | Meaning |
|---|---|
| `managed` | Fully retained and governed by this system; safe to reference from canonical data. |
| `tracked-external` | Known and recorded, but the authoritative copy lives outside this tree (e.g. a source we can always re-fetch from); not duplicated into `library/assets/**` unless promoted. |
| `promote-on-approval` | Retained in `state/` pending an explicit, auditable approval before becoming `managed`. |
| `quarantined` | Flagged by policy (e.g. licensing concern, unmatched, suspicious) and retained but never surfaced as canonical. |

### Promotion is never implicit

Selecting an asset for presentation (a profile referencing it, a bundle
materializing it) is a pure read of an existing `AssetRef`. Nothing in the
schemas or the generation path allows presentation selection to change an
asset's `RetentionOutcome` — promotion out of `promote-on-approval` is always a
separate, explicit, auditable step, never a side effect of "someone chose to
display this."

## Worked example

A SteamGridDB-sourced grid image for a well-established Steam game:

1. No `asset`-level policy exists for its specific hash → check `role`.
2. A `role` policy for `grid` says `managed` → check nothing more specific
   contradicts it (there isn't one) → outcome is `managed`.

The same image, if a `source`-level policy for `steamgriddb` said
`promote-on-approval` and no more specific `role`/`asset` policy overrode it, would
instead sit in `state/` until approved — the `role` policy above only wins because
it is more specific than `source`, not because it was written more recently.
