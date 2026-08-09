# Migration, Recovery, and Safety Rules

See ADR-0005 for the bundle/rollback decision this document expands on. These
rules apply to every tool that touches the synced `GamingProfiles` tree, and are
non-negotiable defaults — a tool must opt *into* anything riskier, never opt out
of these by default.

## Defaults

- **Read-only by default.** Any tool that can write to the tree must be run in an
  explicit write mode; the default invocation only reads and reports.
- **Symbolic roots, relative paths.** Every path recorded in any state/migration
  document uses a symbolic root token (`${LIBRARY}`, `${INBOX}`, `${STAGING}`, ...)
  resolved by local, untracked configuration — never an absolute device path, drive
  letter, home directory, or account-specific segment. See
  `common.defs.schema.json`'s `SymbolicPath`.
- **Sanitized reports.** Anything written under `state/inventory/**` or
  `state/migration/**` is sanitized before being written: no personal paths, no
  account IDs, no IPs, no secrets. `inventory-report.schema.json`'s `privacy`
  field records whether a given inventory document is `private` (root-relative
  paths, safe for local-only use) or `sanitized` (no observations, no relative
  paths at all — safe to attach to a bug report); see
  [`identity-and-policy.md`](identity-and-policy.md) and
  `internal/inventory.Sanitize`.
- **Hash-locked manifests.** Any generated/staged set of files is described by an
  exact-byte SHA-256 manifest (`bundle-lock.schema.json`, or a migration action's
  `sourceSha256`/`expectedDestination` fields, see `migration-manifest.schema.json`).
  Verification (`gamelib manifest verify`) compares actual files against the
  manifest; a mismatch is a **failure**, never something to silently paper over or
  auto-repair.
- **Copy-first, no hardlinks/symlinks.** Materializing a bundle or staging a
  migration always copies bytes into the destination. Hardlinks and symlinks are
  never used for canonical or generated content, because they make it possible for
  an edit or deletion in one location to silently corrupt another — every
  materialized copy must be independently verifiable and independently disposable.
- **Drift is failure, not a repair trigger.** If files on disk don't match their
  recorded manifest/lock, the correct response is to stop and surface the
  mismatch (as an inventory observation or a failed manifest verification), never
  to regenerate over the top of it or assume the manifest was wrong.
- **Staging, then atomic publish.** All changes are planned into a manifest first
  (`migration-manifest.schema.json`) and verified there before anything is applied.
  Publishing is a single atomic operation (e.g. a pointer/rename swap, per
  ADR-0005's `current.json`), never an in-place multi-file write to a live path.
- **Local snapshots and rollback, always available.** Every published change keeps
  its predecessor addressable (`bundle-current.schema.json`'s `previousRevision`).
  Rolling back is always a metadata/pointer operation, never a
  reconstruction-from-scratch operation.
- **No auto-purge.** Nothing is automatically deleted. `state/archive/` and old
  bundle revisions accumulate until an operator explicitly prunes them; retention
  policy (`policy.schema.json`) governs *classification* (`managed` /
  `tracked-external` / `promote-on-approval` / `quarantined`), not automatic
  deletion.

## Migration/export manifest shape

`migration-manifest.schema.json` (`model.Manifest`/`model.Action`, matching what
`internal/manifest` and `internal/profile` actually produce today) describes one
planning operation — `kind` is a dynamic string such as `"import-plan"`,
`"bundle-plan"`, or `"<adapter>-export-plan"` — as an ordered list of `actions`.
Each action names an `action` verb (`copy` / `skip` / `quarantine` / `blocked` /
`render`, among others), a `reason`, and, where applicable, `sourceRoot` /
`sourcePath` / `sourceSha256` / `sourceSize` / `destinationRoot` /
`destinationPath` / `expectedDestination`. **This is a dry-run/plan contract
only** — every `gamelib` subcommand that produces one (`import plan`,
`bundle plan`, `export plan`) is a planner, and `gamelib manifest verify` reads
a manifest back to check `expectedDestination`/`sourceSha256` against reality,
but nothing in the current tooling actually executes (applies) a plan yet.
Applied-state and rollback-record shapes are intentionally left for a future
schema once an apply/rollback command exists in the Go tooling — see
`state/migration/<operation_id>/` in [`tree.md`](tree.md).

## Recovery walkthrough

This describes the **intended** end-to-end flow once an apply/rollback command
exists; today only step 1 (planning) is implemented.

1. A planning run produces a `migration-manifest.schema.json` document
   (`state/migration/<operation_id>/planned.json`) describing the actions that
   *would* copy/skip/quarantine files, and `gamelib manifest verify` confirms its
   `expectedDestination`/`sourceSha256` entries are internally consistent.
2. (Forward-looking) Publish executes the actions atomically; an
   `applied.json` record captures what actually happened.
3. (Forward-looking) If a problem is discovered afterward, a new manifest with
   `kind` such as `"rollback-plan"` is planned referencing the prior operation
   via its `sourceDigest`, restoring the previous `bundle-current.schema.json`
   pointer; its own actions go through the same plan → verify → apply flow.
   Rollback is a forward operation (a new manifest), never an edit of history.
4. Nothing from the prior state is deleted as part of this — it remains in
   `state/archive/` or as a retained prior bundle revision until an operator
   explicitly prunes it.
