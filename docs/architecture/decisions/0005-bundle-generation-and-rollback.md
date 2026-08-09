# ADR-0005: Immutable generated bundles with lock manifests and rollback

## Status

Accepted

## Context

Frontends (Decky's legacy roots, and potentially future ES-DE/RomM-facing
materializations) need a real directory of files on disk, not a database query —
but those directories are entirely derived from canonical data
(`library/canonical/**`, `library/assets/**`). If we let generation write in place,
a half-finished or failed generation run can leave a device with a broken,
inconsistent directory and no way back to the last good state.

## Decision

- Every generated directory bundle lives at `bundles/<bundle_id>/<revision>/`,
  where `<revision>` is a monotonically increasing integer. Once written, a
  revision's files are **immutable** — never edited in place. A new generation run
  always produces a new revision.
- Each revision carries `manifest.lock.json` (`bundle-lock.schema.json`): every
  materialized file with its path and exact-byte SHA-256, the canonical profile id
  and content-digest it was generated from, and the asset digests it consumed.
  This is the hash-locked manifest that later drift-detection compares against —
  any mismatch between the manifest and the files on disk is treated as failure,
  not silently repaired.
- `bundles/<bundle_id>/current.json` (`bundle-current.schema.json`) is a small
  pointer file naming the live `current_revision` and the `previous_revision`.
  Publishing is: generate into a new revision directory (staging), verify its lock
  manifest, then atomically rewrite `current.json` to point at it. **Rollback is
  re-pointing `current.json` back to `previous_revision`** — it never deletes or
  edits a revision directory, so rollback is always available as long as the prior
  revision hasn't been explicitly pruned by an operator (no auto-purge).

## Consequences

- **Easier:** a bad generation run never corrupts what's live, because publish is a
  single atomic pointer swap after the new revision is fully written and verified;
  rollback is trivial and instant (rewrite one small JSON file); every revision is
  independently auditable via its lock manifest.
- **Harder:** disk usage grows with retained revisions unless an operator prunes
  old ones explicitly (by design — no automatic deletion, see
  `../migration-and-recovery.md`); consumers must always resolve "current" through
  `current.json` rather than assuming the latest numeric revision is live.
- Bundles are always reproducible from `library/canonical/**` + `library/assets/**`,
  which is precisely why they are safe to treat as generated/disposable rather than
  as a second source of truth (consistent with ADR-0003's legacy Decky roots).
