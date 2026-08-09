# Architecture

This directory documents the canonical game-artwork/catalog plan: a private,
Syncthing-synced tree (`GamingProfiles`) whose contract is defined by the JSON
Schemas in [`../../schemas/v1/`](../../schemas/v1/), plus the Go tooling (outside
this change) that reads, writes, validates, and migrates that tree.

## Start here

| Doc | Covers |
|---|---|
| [`tree.md`](tree.md) | The full canonical directory layout, annotated. |
| [`identity-and-policy.md`](identity-and-policy.md) | How games/releases/assets are identified, and the asset > role > system > source > global retention precedence. |
| [`adapters.md`](adapters.md) | Per-integration contracts (Steam, Playnite, ES-DE, RomM, Epic/EA/Ubisoft) and the plan/staging-first + homelab-ownership boundary. |
| [`migration-and-recovery.md`](migration-and-recovery.md) | Safety defaults: read-only default, staging/atomic publish, hash-locked manifests, rollback, no auto-purge. |
| [`sources.md`](sources.md) | Reference table of external source contracts and stability. |
| [`decisions/`](decisions/) | ADRs recording why each of the above is shaped the way it is. |

## Quick mental model

```
This repo (jrmoulckers/game-library)          Private Syncthing folder: GamingProfiles
├── schemas/v1/*.schema.json  ───contract───►  catalog.json, library/**, bundles/**, state/**,
├── docs/architecture/**                        generated legacy roots profiles/ artwork/ mods/
└── (Go tooling, elsewhere in this repo)  ───reads/writes/validates───► the tree above
```

- **Canonical source of truth:** `library/**` (games, releases, mappings, assets,
  sources, policies, canonical profiles/themes/mods).
- **Generated, never source:** `bundles/**` and the legacy `profiles/`/`artwork/`/
  `mods/` roots — always reproducible from `library/**`.
- **Operational/in-flight:** `state/**` (inventory, inbox, quarantine, archive,
  migration) — where anything uncertain lives until promoted or archived.
