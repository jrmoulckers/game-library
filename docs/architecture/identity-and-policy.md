# Identity and Retention Policy

See ADR-0002 and ADR-0004 for the decisions behind this document; this is the
practical reference for both models.

## Identity model

| Domain | Canonical identity | Schema | Notes |
|---|---|---|---|
| PC / Steam | Steam AppID (integer) | `game.schema.json` (`pc.appid`) — **forward-looking**; today expressed as a profile's `games[].identities.steam` value, see below | Stable, vendor-issued, first-class. |
| PC / other storefronts | Storefront's own opaque ID | `game.schema.json` (`pc.storefront_id`) — **forward-looking** | Scoped by `pc.store`; format is whatever that storefront uses. |
| Retro (logical game) | `system` + `slug` | `game.schema.json` (`retro.system`, `retro.slug`) — **forward-looking**; today expressed as a profile's `games[].retro.{system,stem}`, see below | The concept "this game on this system", independent of any one file. |
| Retro (exact release) | Full raw SHA-256 of the artifact | `retro-release.schema.json` (`sha256`) — **forward-looking** | Exact bytes. Never a truncated/rolling/partial hash. |
| Asset | Exact-byte SHA-256 of `content.<ext>` | `asset.schema.json` (`sha256`) | De-duplicates automatically; a re-encode is a new asset, not an edit. |
| Playnite entry | **Not canonical** — UUID kept only in an adapter-only mapping | `playnite-mapping.schema.json` — **forward-looking**; today expressed as a profile's `games[].identities.playnite` value | See below. |

**Implementation note:** the currently implemented Go model (`model.Profile` /
`canonical-profile.schema.json`) does not yet have a standalone game/release/
mapping registry. Instead, each profile's `games[]` entry carries its own
free-form `id` (e.g. `"steam:123"`, `"retro:n64:example-game"`, following the
same `steam:`/`playnite:`/`retro:` convention `internal/identity` proposes from
scanned files), an `identities{}` map of adapter name → native id (Playnite UUID
goes here, under the `"playnite"` key), and an optional `retro{system,stem}`
block for the logical retro game. The `game.schema.json` /
`retro-release.schema.json` / `playnite-mapping.schema.json` tables above
describe the agreed target shape for a future standalone registry, not
anything the CLI reads or writes today.

### Filenames and aliases are never identity

Retro `filename_aliases` (on both `game.schema.json`'s `retro` block and
`retro-release.schema.json`) exist purely so a human operator recognizes what
they're looking at. Matching, de-duplication, and policy decisions are always made
on `sha256`, never on filename, extension, or path.

### Playnite UUIDs are adapter-only

A Playnite UUID is a property of one Playnite installation's local database. It
must never be treated as canonical identity. Today it is recorded only as the
`"playnite"` entry in a profile's `games[].identities` map (`model.ProfileGame`,
consumed by `internal/profile`'s adapter destination resolution, never written
back to any Playnite database). The forward-looking `playnite-mapping.schema.json`
describes a future standalone `library/mappings/playnite/<uuid>.json` file with
the same intent — pointing at a canonical game reference, flagged
`adapter_only: true`, carrying a `confidence` of `exact`/`fuzzy`/`manual` — for
once a standalone game registry exists.

If Playnite's database is rebuilt and issues a new UUID for the "same" game, only
the `identities.playnite` value (or, in the forward-looking model, the mapping
file) needs to change — the canonical game, its assets, and its history are
unaffected.

## Retention policy precedence

Policy is a **single** file (`policy.schema.json`, `model.PolicyFile`, validated
and resolved by `internal/policy`): one global `default` outcome plus an
optional `rules[]` list, each rule an independent combination of zero or more
selectors (`source`, `system`, `role`, `assetSha256`) plus a required `mode`.
There is no one-file-per-scope-level layout — a single rule may combine
selectors (e.g. `system` + `role` together).

Precedence is an additive specificity score computed per rule
(`internal/policy.specificity`), most specific wins:

```
assetSha256 (+8)  >  role (+4)  >  system (+2)  >  source (+1)  >  default (0, global fallback)
(most specific)                                                    (least specific, fallback)
```

- `default` is the global fallback outcome, used when no rule matches at all.
- A rule with only `source` set (weight 1) is the least specific override; a
  rule with only `assetSha256` set (weight 8) always wins over any rule that
  does not also set `assetSha256`, regardless of how many other selectors that
  rule sets.
- `internal/policy.Resolve` picks the single highest-scoring matching rule for
  a given observation; `internal/policy.Validate` rejects two rules that share
  the exact same `(source, system, role, assetSha256)` selector tuple —
  including the all-empty tuple, so at most one selector-less rule may exist
  — as a "duplicate policy selector" configuration error. This whole-tuple
  uniqueness rule is documented rather than schema-enforced (see
  `policy.schema.json`'s `$comment`), since plain JSON Schema `uniqueItems`
  cannot express "reject duplicates on a derived key ignoring `mode`."

### Outcomes

| Outcome | Meaning |
|---|---|
| `managed` | Fully retained and governed by this system; safe to reference from canonical data. |
| `tracked-external` | Known and recorded, but the authoritative copy lives outside this tree (e.g. a source we can always re-fetch from); not duplicated into `library/assets/**` unless promoted. |
| `promote-on-approval` | Retained in `state/` pending an explicit, auditable approval before becoming `managed`. |
| `quarantined` | Flagged by policy (e.g. licensing concern, unmatched, suspicious) and retained but never surfaced as canonical. |

### Promotion is never implicit

Selecting an asset for presentation (a profile referencing it, a bundle
materializing it) is a pure read of an existing asset reference. Nothing in the
schemas or the generation path allows presentation selection to change an
asset's retention outcome — promotion out of `promote-on-approval` is always a
separate, explicit, auditable step, never a side effect of "someone chose to
display this."

## Worked example

A SteamGridDB-sourced grid image for a well-established Steam game, evaluated
against a policy file with `default: "quarantined"` and two rules —
`{role: "grid", mode: "managed"}` (weight 4) and
`{source: "steamgriddb", mode: "promote-on-approval"}` (weight 1):

1. Both rules match this observation (its inferred role is `grid` and its
   source root is `steamgriddb`); their scores are 4 and 1 respectively.
2. The `role` rule has the higher score, so it wins: outcome is `managed`.

The same image would instead sit in `state/` pending approval only if the
`role` rule did not exist (or matched a different role) — the `source` rule
only loses because it is less specific, not because of file order.
