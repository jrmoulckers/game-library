# Architecture Decision Records

This directory holds ADRs for the canonical game-artwork/catalog plan. Recording
consequential tradeoffs this way, and superseding rather than editing an accepted
record, follows
[`ENG-ARCH-003`](https://github.com/jrmoulckers/engineering/blob/v0.2.3/principles/architecture/boundaries-and-contracts.md#durable-decisions).

Repository-specific: each ADR uses the standard template (Status / Context /
Decision / Consequences) and a sequential `NNNN-slug.md` name under
`docs/architecture/decisions/`. Note that sibling repositories currently use a
different location and numbering scheme; a unified convention is being decided
centrally, so nothing here is renumbered yet.

| # | Title | Status |
|---|-------|--------|
| [0001](0001-two-tree-topology.md) | Two-tree topology: repo vs. synced private tree | Accepted |
| [0002](0002-content-addressed-assets-and-retention.md) | Content-addressed assets and layered retention policy | Accepted |
| [0003](0003-decky-v1-abi-preservation.md) | Preserve the Decky v1 profile ABI as a generated legacy surface | Accepted |
| [0004](0004-identity-and-mapping-strategy.md) | Identity model and Playnite as adapter-only mapping | Accepted |
| [0005](0005-bundle-generation-and-rollback.md) | Immutable generated bundles with lock manifests and rollback | Accepted |
| [0006](0006-adapter-boundaries-and-ownership.md) | Adapter boundaries, staging-first integration, homelab ownership | Accepted |
| [0007](0007-local-dashboard.md) | Local dashboard is a plan-only Go web surface | Partly superseded by 0008 |
| [0008](0008-organizer-only-dashboard.md) | The dashboard is an artwork organizer, not a review console | Accepted |

See also the narrative docs one level up:

- [`../tree.md`](../tree.md) — full canonical tree layout
- [`../identity-and-policy.md`](../identity-and-policy.md) — identity rules and retention policy precedence
- [`../adapters.md`](../adapters.md) — per-integration adapter contracts
- [`../migration-and-recovery.md`](../migration-and-recovery.md) — safety, staging, rollback
- [`../sources.md`](../sources.md) — external source reference contracts
