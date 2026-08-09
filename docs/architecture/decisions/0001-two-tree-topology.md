# ADR-0001: Two-tree topology: repo vs. synced private tree

## Status

Accepted

## Context

The system has two very different kinds of content:

1. **Schemas, docs, and tooling** — small, text-based, safe to keep in a public/shared
   Git history, valuable to diff and review over time.
2. **Catalog data and binary assets** — potentially large (artwork, ROM/ISO metadata
   references), private, personal-device-shaped, and updated far more often than the
   tooling that reads it.

Putting both in the same Git repository would force binary/catalog churn into Git
history (bloating clones, leaking private inventory data in history, and coupling
tooling releases to catalog edits), or would force us to bend Git around one of the
two use cases (submodules, LFS, sparse checkouts) with meaningfully more operational
complexity than the problem warrants.

## Decision

Split ownership across two trees:

- **This Git repository** (`jrmoulckers/game-library`) owns everything that should be
  versioned, reviewed, and shared: JSON Schemas (`schemas/v1/`), architecture docs
  (`docs/`), and — outside this change — the Go tooling that reads/writes/validates
  the synced tree. It contains **no catalog data and no assets**.
- **A private Syncthing-synced folder, `GamingProfiles`**, owns the actual catalog:
  `catalog.json`, `library/**` (games, releases, mappings, assets, sources, policies,
  canonical profiles/themes/mods), generated `bundles/**`, `state/**`, and the
  generated legacy Decky roots (`profiles/`, `artwork/`, `mods/`). It is not a Git
  repository; Syncthing provides device-to-device replication, this repo's tooling
  provides validation/generation, and `state/` provides recoverability (see ADR-0005
  and `../migration-and-recovery.md`).

Tooling in this repo reads/writes the synced tree via a configurable root path; the
schemas in `schemas/v1/` are the shared contract between the two trees and are the
thing that keeps every device's copy of `GamingProfiles` mutually intelligible.

## Consequences

- **Easier:** Git history stays small and reviewable; the private catalog can be as
  large and as personal as needed without ever touching a public/shared remote;
  tooling versions independently of catalog content.
- **Harder:** There is no single `git log` spanning both schema changes and catalog
  changes — schema/tooling upgrades and catalog migrations must be coordinated
  explicitly (see ADR-0005 and the migration/export manifest schema) rather than
  relying on one commit history for both.
- Any schema change that is not backward compatible must ship a migration path for
  the synced tree, since there is no Git history to `git revert` catalog data through.
