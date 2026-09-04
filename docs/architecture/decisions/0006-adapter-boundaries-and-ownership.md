# ADR-0006: Adapter boundaries, staging-first integration, homelab ownership

- Status: Accepted
- Date: 2026-08-09
- Owner: jrmoulckers

## Context

The catalog touches several external systems with wildly different levels of API
stability and trustworthiness: Steam and RomM offer real (if partial) programmatic
contracts; Playnite offers a file-based export but its live database must never be
treated as a target; ES-DE is config/file-based; Epic, EA, and Ubisoft currently
offer no stable, durable contract at all. Meanwhile, applying anything to a live
device (writing frontend directories, restarting services) is an operational
concern distinct from deciding *what* should be applied — conflating the two makes
every integration both a data-mapping problem and a live-system-safety problem at
once.

## Decision

- Every external integration is described by a `source.schema.json` record with an
  explicit `contract` (`openapi` | `rest` | `export-file` | `scrape` | `manual`) and
  `stability` (`stable` | `best-effort` | `planned` | `unsupported`). RomM
  integration prefers its OpenAPI contract (`contract: openapi`) over scraping.
  Epic, EA, and Ubisoft are `capabilities: [inventory]`-only with `stability` at
  most `best-effort` until each offers something more durable — they inform the
  catalog of what exists, never drive artwork/metadata/mapping decisions.
- **All adapters are plan/staging first.** An adapter's job is to produce
  observations (`inventory-observation.schema.json`) and staged migration output
  (`migration-manifest.schema.json`) under `state/`, never to write directly into
  `library/**` or a live frontend root. Promotion from staged/observed data into
  canonical data follows the same retention/approval rules as everything else
  (ADR-0002).
- **Playnite's own database is never written by this system**, in either
  direction beyond reading its export — see ADR-0004. Mapping is one-way:
  Playnite → canonical, recorded as an adapter-only mapping file.
- **Live frontend directories are device-local**, not part of the synced tree.
  Applying a generated bundle (ADR-0005) to a specific device's live frontend
  paths, restarting/reloading services, and any other runtime-apply concern is
  owned by the homelab environment — specifically the CT601 host and its
  Syncthing configuration — not by this repository's tooling. This repo's tooling
  stops at producing a verified, staged, lock-manifested bundle; homelab-side
  automation decides when/whether to apply it to a given device.

## Consequences

- **Easier:** a source with no stable contract (Epic/EA/Ubisoft today) can still be
  represented and tracked for inventory purposes without that instability leaking
  into artwork/metadata correctness; adding a new source is a new `source.json`
  plus an adapter that only ever writes to `state/`, never to canonical data
  directly; live-apply safety (which device, when, how to roll back) is fully
  decoupled from catalog correctness and can evolve independently on the homelab
  side.
- **Harder:** every adapter needs an explicit promotion step reviewed against
  policy before its output becomes canonical — there is no "fast path" direct
  write, by design; homelab-side apply logic must independently honor the bundle
  current-pointer/rollback contract (ADR-0005) since this repo does not enforce it
  at runtime.
- If a currently `unsupported`/`best-effort` source (e.g. Epic) later gains a
  stable contract, upgrading it is a `source.json` edit plus a new/updated adapter
  — not a schema change, since `source.schema.json` already has room for the full
  stability range.

## Evidence

Adapters are read-only by construction and by test.
[`internal/source/detect_test.go`](../../../internal/source/detect_test.go) covers detection
across conventional Windows and Linux locations and asserts it performs no network request and
makes no host-local change without explicit confirmation.

The boundary is documented in [`docs/architecture/adapters.md`](../adapters.md) and the source
shape is fixed by [`schemas/v1/source.schema.json`](../../../schemas/v1/source.schema.json)
([`ENG-INT-001`](https://github.com/jrmoulckers/engineering/blob/v0.116.0/principles/platforms/integration-boundaries.md#thin-typed-adapters)).

Falsifiable by: any write path into a third-party database, or a detection probe that reaches
the network.
