# JSON Schemas (v1)

JSON Schema 2020-12 contracts for the canonical `GamingProfiles` tree described in
[`../../docs/architecture/tree.md`](../../docs/architecture/tree.md). These are the
shared contract between every device's copy of the synced tree and the Go tooling
that reads/writes/validates it — see
[ADR-0001](../../docs/architecture/decisions/0001-two-tree-topology.md).

All schemas use `$id`s under the `https://schemas.game-library.dev/v1/` namespace.
This is a stable identifier namespace for `$ref` resolution only — it is not
expected to be a live, dereferenceable URL.

## Files

| File | Validates | Notes |
|---|---|---|
| `common.defs.schema.json` | *(library only, no instances)* | Shared `$defs`: `Sha256Hex`, `SafeId`, `ProfileGameID`, `Slug`, `Timestamp`, `SymbolicPath`, `SafeRelativeSegment`, `Extension`, `AssetRef`, `AssetRole`, `GameRef`, `PlatformClass`, `StoreId`, `SystemId`, `RetentionOutcome`, `ModAssignment`, `Provenance`, `License`. |
| `catalog.schema.json` | `catalog.json` | Root manifest: counts, root pointers, optional integrity digest. **Forward-looking** — not yet backed by a Go type. |
| `asset.schema.json` | `library/assets/sha256/<aa>/<hash>/asset.json` | Content-addressed asset metadata; `retentionState`-conditional `contentFile`/`byteSize`. **Forward-looking** — not yet backed by a Go type. |
| `game.schema.json` | `library/games/{pc,retro}/**/game.json` | Unified PC + retro game, discriminated by `platform_class`. **Forward-looking** — the implemented Go model embeds game identity directly in a profile's `games[]` entries instead (see `canonical-profile.schema.json`); no separate game registry file exists yet. |
| `retro-release.schema.json` | `library/releases/retro/<system>/<sha256>/release.json` | Exact-byte retro release identity. **Forward-looking** — not yet backed by a Go type. |
| `playnite-mapping.schema.json` | `library/mappings/playnite/<uuid>.json` | Adapter-only Playnite UUID → canonical game mapping. **Forward-looking** — today a Playnite UUID is just a value in a `ProfileGame.identities` map (see `canonical-profile.schema.json`); no standalone mapping file exists yet. |
| `source.schema.json` | `library/sources/<source_id>.json` | External source/integration contract description. **Forward-looking** — not yet backed by a Go type. |
| `policy.schema.json` | `library/policies/policy.json` (single file) | `model.PolicyFile`/`model.PolicyRule`: one `default` outcome plus optional scoped `rules[]` (source/system/role/assetSha256 selectors); asset > role > system > source > global precedence. Matches `internal/policy` and `configs/examples/policy.json` exactly. |
| `canonical-profile.schema.json` | `library/profiles/<id>/profile.json` | `model.Profile`: per-game (`games[]`) identities/retro target/per-role `assets{}` selections, optional profile-level `mods[]`. Matches `internal/profile`/`testdata/profiles/example.json` exactly. |
| `decky-profile-v1.schema.json` | `profiles/<id>.json` (generated legacy root) | Frozen Decky plugin ABI v1 — never changed in place. Matches `internal/decky`/`testdata/decky/*.json` exactly. |
| `bundle-lock.schema.json` | `bundles/<profile-id>/<revision-sha256>/bundle.lock.json` | Hash-locked file manifest for one immutable bundle revision; `revision` is the profile's full content SHA-256 (`internal/profile`'s `revision()`), not a counter. **Forward-looking file contents** — `internal/profile.BuildBundlePlan` plans writing this file but no Go type defines its shape yet; path/naming match the implemented plan exactly. |
| `bundle-current.schema.json` | `bundles/<profile-id>/current.json` | Live revision pointer + rollback target; `currentRevision`/`previousRevision` are SHA-256 strings (or `null`). **Forward-looking** — `internal/profile.BuildBundlePlan` explicitly does not update this file (dry-run only). |
| `inventory-report.schema.json` | inventory report JSON (e.g. `reports/*.json`, `gamelib inventory scan` / `report summary` output) | `model.Inventory`: root summaries, optional `observations[]` (private only), `duplicateSummary`, optional `issues[]`; `privacy` is `private`\|`sanitized`. Matches `internal/inventory` exactly. |
| `inventory-observation.schema.json` | inline `observations[]` entries within an inventory report (private reports only) | `model.Observation`/`model.MediaFacts`: root-relative path, size, sha256, media facts, optional system/identityHint. Matches `internal/inventory`/`internal/media` exactly. |
| `migration-manifest.schema.json` | manifest/plan JSON emitted by `gamelib import plan` / `bundle plan` / `export plan` (`manifest verify` reads it back) | `model.Manifest`/`model.Action`: dry-run/plan-only action list (`copy`/`skip`/`quarantine`/`blocked`/`render`, etc.), no applied/rollback state. Matches `internal/manifest`/`internal/profile` exactly. |

## Conventions

These schemas are this repository's published contract, so they are versioned and
evolve additively until a declared breaking boundary
([`ENG-ARCH-002`](https://github.com/jrmoulckers/engineering/blob/v0.12.0/principles/architecture/boundaries-and-contracts.md#explicit-additive-contracts),
[`ENG-API-001`](https://github.com/jrmoulckers/engineering/blob/v0.12.0/principles/platforms/api-backend.md#typed-versioned-apis)):
a breaking change gets a new `schemas/vN/` directory rather than an edit in
place. `internal/schema` validates both accepted and rejected fixtures against
the committed files
([`ENG-TEST-007`](https://github.com/jrmoulckers/engineering/blob/v0.12.0/principles/assurance/testing.md#positive-and-negative-polarity)).

Repo-specific conventions:

- **2020-12** (`$schema: https://json-schema.org/draft/2020-12/schema`) throughout.
- **`additionalProperties: false`** on every object type where the property set is
  known and closed; open-ended metadata is modeled as explicit optional fields
  (e.g. `notes`) or `additionalProperties: { "type": "string" }` maps (e.g.
  `Profile.compatibility`, `ProfileGame.identities`) rather than left implicitly
  extensible.
- **Reusable `$defs`** live in `common.defs.schema.json` and are pulled in via
  relative `$ref` (e.g. `"common.defs.schema.json#/$defs/Sha256Hex"`), resolved
  against each schema's own `$id` per standard JSON Schema URI resolution. Any
  tool that loads this schema set must register all files with a single
  resolver/registry (e.g. `ajv.addSchema` for every file) before validating.
- **Schemas that validate an implemented Go type** (`canonical-profile`,
  `policy`, `inventory-report`, `inventory-observation`, `migration-manifest`,
  `decky-profile-v1`) use a `version`/`toolVersion` field only where
  `internal/model` actually has one, and contain **no** `$schema` or
  `schema_version` instance property — none of the real JSON these types produce
  includes either. Those two keys remain reserved for the still-forward-looking
  schemas (`catalog`, `asset`, `game`, `retro-release`, `playnite-mapping`,
  `source`, `bundle-lock`, `bundle-current`) until a concrete Go/consumer shape
  exists to confirm the convention for them too.
- Conditional shape (e.g. `game.schema.json`'s `pc` vs `retro` block,
  `inventory-report.schema.json`'s sanitized-vs-private `observations`
  requirement, `asset.schema.json`'s managed-vs-other `retentionState`
  requirement) is expressed with `if`/`then`/`else`, each branch redeclaring the
  properties it constrains so the schema stays valid in strict mode.
- A few business rules are **documented, not schema-enforced**, because plain
  JSON Schema cannot express them: e.g. "unique per `game`" on `mods` arrays
  (`decky-profile-v1.schema.json`, strictly enforced by `internal/decky.Validate`;
  `canonical-profile.schema.json`'s `mods[]` follows the same convention but is
  not yet re-checked by `internal/profile.Validate`) is a tooling-level
  validation, not a `uniqueItems` constraint (which would only catch whole-object
  duplicates, not duplicate `game` keys with different `set` values). Likewise
  `policy.schema.json`'s duplicate-selector-tuple rejection
  (`internal/policy.Validate`) and `config.IsSafeID`'s reserved-Windows-device-name
  exclusion (on `SafeId`) are documented in `$comment`/`description` rather than
  encoded as a `pattern`.
- Five schemas (`catalog`, `game`, `retro-release`, `playnite-mapping`, `source`)
  describe a **forward-looking canonical registry** that is not yet implemented
  in Go: today, game/release/mapping identity lives inline in a profile's
  `games[]` entries (`id`, `identities{}`, `retro{system,stem}` — see
  `canonical-profile.schema.json`), not in standalone per-game/per-release/
  per-mapping files. These schemas are kept (not deleted) as the agreed target
  shape for that future registry; they are not exercised by any current Go code
  or test fixture.
