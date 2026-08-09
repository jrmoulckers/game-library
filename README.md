# game-library

Canonical game artwork, metadata, profiles, and recovery tooling.

This repository owns the **contract and tooling**: JSON Schemas, architecture
docs, and the `gamelib` Go CLI that reads/validates/plans against that contract.
The actual catalog — games, releases, artwork, profiles — lives in a separate,
private, Syncthing-synced tree (`GamingProfiles`) and is never checked into this
repository. See
[ADR-0001](docs/architecture/decisions/0001-two-tree-topology.md) for why.

## Quick start

- **Understand the design:** start at
  [`docs/architecture/README.md`](docs/architecture/README.md), then
  [`docs/architecture/tree.md`](docs/architecture/tree.md) for the full canonical
  layout.
- **Look up a contract:** schemas live under
  [`schemas/v1/`](schemas/v1/README.md), one file per record type — the
  implemented contracts (canonical profile, policy file, inventory
  report/observation, migration/export manifest, and the frozen Decky v1 legacy
  profile ABI) plus a few forward-looking ones not yet backed by Go code (game,
  retro release, Playnite mapping, source, catalog, asset content metadata,
  bundle lock/current pointer) — see the schema table's Notes column for which
  is which.
- **Identity and retention rules:**
  [`docs/architecture/identity-and-policy.md`](docs/architecture/identity-and-policy.md).
- **Integrations (Steam, Playnite, ES-DE, RomM, Epic/EA/Ubisoft):**
  [`docs/architecture/adapters.md`](docs/architecture/adapters.md) and
  [`docs/architecture/sources.md`](docs/architecture/sources.md).
- **Safety, staging, and rollback:**
  [`docs/architecture/migration-and-recovery.md`](docs/architecture/migration-and-recovery.md).
- **Why things are shaped this way:** [`docs/architecture/decisions/`](docs/architecture/decisions/)
  (ADRs).

### Using the CLI

`gamelib` (module `github.com/jrmoulckers/game-library`, `cmd/gamelib`) is a
**plan/dry-run-only** tool today — every subcommand reads and reports, or writes
a plan document for a human/future-tool to review; **none of them applies
changes to the synced tree**:

```
go build -o bin/gamelib ./cmd/gamelib
gamelib inventory scan                     # scan configured roots
gamelib report summary                     # sanitize a private inventory report
gamelib duplicates report                  # exact duplicate groups from a private report
gamelib identity propose                   # propose steam:/playnite:/retro: identity hints
gamelib import plan                        # plan (never apply) policy outcomes/copies
gamelib profile resolve                    # verify a profile's canonical asset closure
gamelib bundle plan                        # plan (never apply) a retained bundle revision
gamelib export plan --adapter <adapter>    # steam|decky|playnite|esde|romm staging plan
gamelib validate profile|decky-v1|decky-catalog|inventory <path>
gamelib manifest verify                    # verify a plan file's expected SHA-256
gamelib version
```

Run the leaf command with `-h` (for example, `gamelib inventory scan -h`) for
flags. See `configs/examples/config.json` and
`configs/examples/policy.json` for example configuration/policy documents, and
`testdata/profiles/example.json` / `testdata/decky/*.json` for example profile
documents matched to `schemas/v1/canonical-profile.schema.json` and
`schemas/v1/decky-profile-v1.schema.json` respectively.

## Repository layout

```
cmd/gamelib/           gamelib CLI entrypoint (Go)
internal/              Go packages implementing the contract: config, model,
                        policy, inventory, identity, media, manifest, profile,
                        decky, report, schema
configs/examples/      Example config.json / policy.json documents
testdata/              Example profile / Decky v1 fixtures used by tests and
                        by this repo's schema validation
reports/baseline/      Sanitized, aggregate-only inventory baselines (see its
                        own README) — never exact paths/hashes/actions
docs/architecture/     Architecture docs and ADRs (this repo's design record)
schemas/v1/            JSON Schema 2020-12 contracts for the synced catalog tree
```

Go source, tests, and CI configuration in this repository are owned by their
respective engineering roles per each area's `AGENTS.md`; this README's
"Quick start"/"Repository layout" sections are maintained for navigation only —
architecture and schema content itself lives under `docs/architecture/` and
`schemas/v1/`.
