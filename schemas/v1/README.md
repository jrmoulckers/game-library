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
| `common.defs.schema.json` | *(library only, no instances)* | Shared `$defs`: `Sha256Hex`, `SafeId`, `Slug`, `Timestamp`, `SymbolicPath`, `AssetRef`, `AssetRole`, `GameRef`, `PlatformClass`, `StoreId`, `SystemId`, `RetentionOutcome`, `ModAssignment`, `Provenance`, `License`. |
| `catalog.schema.json` | `catalog.json` | Root manifest: counts, root pointers, optional integrity digest. |
| `asset.schema.json` | `library/assets/sha256/<aa>/<hash>/asset.json` | Content-addressed asset metadata. |
| `game.schema.json` | `library/games/{pc,retro}/**/game.json` | Unified PC + retro game, discriminated by `platform_class`. |
| `retro-release.schema.json` | `library/releases/retro/<system>/<sha256>/release.json` | Exact-byte retro release identity. |
| `playnite-mapping.schema.json` | `library/mappings/playnite/<uuid>.json` | Adapter-only Playnite UUID → canonical game mapping. |
| `source.schema.json` | `library/sources/<source_id>.json` | External source/integration contract description. |
| `policy.schema.json` | `library/policies/<policy_id>.json` | One retention/promotion policy at one scope level. |
| `canonical-profile.schema.json` | `library/canonical/profiles/<id>.json` | Canonical (rich) profile; source for the generated legacy profile. |
| `decky-profile-v1.schema.json` | `profiles/<id>.json` (generated legacy root) | Frozen Decky plugin ABI v1 — never changed in place. |
| `bundle-lock.schema.json` | `bundles/<bundle_id>/<revision>/manifest.lock.json` | Hash-locked file manifest for one immutable bundle revision. |
| `bundle-current.schema.json` | `bundles/<bundle_id>/current.json` | Live revision pointer + rollback target. |
| `inventory-report.schema.json` | `state/inventory/reports/<report_id>.json` | Summary of one scan pass. |
| `inventory-observation.schema.json` | `state/inventory/observations/<observation_id>.json` | One scanned item + its provisional outcome. |
| `migration-manifest.schema.json` | `state/migration/<migration_id>/manifest.json` | One staged import/export/upgrade/rollback operation. |

## Conventions

- **2020-12** (`$schema: https://json-schema.org/draft/2020-12/schema`) throughout.
- **`additionalProperties: false`** on every object type where the property set is
  known and closed; open-ended metadata is modeled as explicit optional fields
  (e.g. `notes`) rather than left implicitly extensible.
- **Reusable `$defs`** live in `common.defs.schema.json` and are pulled in via
  relative `$ref` (e.g. `"common.defs.schema.json#/$defs/Sha256Hex"`), resolved
  against each schema's own `$id` per standard JSON Schema URI resolution. Any
  tool that loads this schema set must register all files with a single
  resolver/registry (e.g. `ajv.addSchema` for every file) before validating.
- **`schema_version`** is a `const` integer on every record type so a future
  breaking change has an explicit migration point, distinct from the Decky legacy
  ABI's own frozen `version` field (which is a different, intentionally-separate
  version space — see `decky-profile-v1.schema.json`).
- Conditional shape (e.g. `game.schema.json`'s `pc` vs `retro` block,
  `policy.schema.json`'s scope selector requirement) is expressed with
  `if`/`then`/`else`, each branch redeclaring the properties it constrains so the
  schema stays valid in strict mode.
- A few business rules are **documented, not schema-enforced**, because plain
  JSON Schema cannot express them: e.g. "unique per `game`" on `mods` arrays
  (`decky-profile-v1.schema.json`, `canonical-profile.schema.json`) is a tooling-
  level validation, not a `uniqueItems` constraint (which would only catch
  whole-object duplicates, not duplicate `game` keys with different `set` values).
