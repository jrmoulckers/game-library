# game-library

Canonical game artwork, metadata, profiles, and recovery tooling.

This repository owns the **contract and tooling**: JSON Schemas and architecture
docs (and, outside of this change, the Go tooling that reads/validates/generates
against that contract). The actual catalog — games, releases, artwork, profiles —
lives in a separate, private, Syncthing-synced tree (`GamingProfiles`) and is
never checked into this repository. See
[ADR-0001](docs/architecture/decisions/0001-two-tree-topology.md) for why.

## Quick start

- **Understand the design:** start at
  [`docs/architecture/README.md`](docs/architecture/README.md), then
  [`docs/architecture/tree.md`](docs/architecture/tree.md) for the full canonical
  layout.
- **Look up a contract:** schemas live under
  [`schemas/v1/`](schemas/v1/README.md), one file per record type (game, asset,
  retro release, Playnite mapping, source, policy, canonical profile, bundle
  lock/pointer, inventory report/observation, migration manifest, and the frozen
  Decky v1 legacy profile ABI).
- **Identity and retention rules:**
  [`docs/architecture/identity-and-policy.md`](docs/architecture/identity-and-policy.md).
- **Integrations (Steam, Playnite, ES-DE, RomM, Epic/EA/Ubisoft):**
  [`docs/architecture/adapters.md`](docs/architecture/adapters.md) and
  [`docs/architecture/sources.md`](docs/architecture/sources.md).
- **Safety, staging, and rollback:**
  [`docs/architecture/migration-and-recovery.md`](docs/architecture/migration-and-recovery.md).
- **Why things are shaped this way:** [`docs/architecture/decisions/`](docs/architecture/decisions/)
  (ADRs).

## Repository layout

```
docs/architecture/    Architecture docs and ADRs (this repo's design record)
schemas/v1/            JSON Schema 2020-12 contracts for the synced catalog tree
```

Application/service source, if and when it lands in this repository, is
documented and owned separately per each area's `AGENTS.md`; this README only
covers navigation and the architecture/schema contract.
