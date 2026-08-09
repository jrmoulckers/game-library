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
  account IDs, no IPs, no secrets. `inventory-report.schema.json` requires
  `sanitized: true` as a standing assertion, checked by tooling, not just claimed.
- **Hash-locked manifests.** Any generated/staged set of files is described by an
  exact-byte SHA-256 manifest (`bundle-lock.schema.json`, or the
  `checksum_manifest_ref` on a migration). Verification compares actual files
  against the manifest; a mismatch is a **failure**, never something to silently
  paper over or auto-repair.
- **Copy-first, no hardlinks/symlinks.** Materializing a bundle or staging a
  migration always copies bytes into the destination. Hardlinks and symlinks are
  never used for canonical or generated content, because they make it possible for
  an edit or deletion in one location to silently corrupt another — every
  materialized copy must be independently verifiable and independently disposable.
- **Drift is failure, not a repair trigger.** If files on disk don't match their
  recorded manifest/lock, the correct response is to stop and surface the
  mismatch (as an inventory observation or a failed migration step), never to
  regenerate over the top of it or assume the manifest was wrong.
- **Staging, then atomic publish.** All changes are written to a staging location
  first (`migration-manifest.schema.json`'s `staging_path`) and are verified there.
  Publishing is a single atomic operation (e.g. a pointer/rename swap, per
  ADR-0005's `current.json`), never an in-place multi-file write to a live path.
- **Local snapshots and rollback, always available.** Every published change keeps
  its predecessor addressable (`bundle-current.schema.json`'s `previous_revision`,
  `migration-manifest.schema.json`'s `rollback_ref`). Rolling back is always a
  metadata/pointer operation, never a reconstruction-from-scratch operation.
- **No auto-purge.** Nothing is automatically deleted. `state/archive/` and old
  bundle revisions accumulate until an operator explicitly prunes them; retention
  policy (`policy.schema.json`) governs *classification* (`managed` /
  `tracked-external` / `promote-on-approval` / `quarantined`), not automatic
  deletion.

## Migration/export manifest shape

`migration-manifest.schema.json` describes one `import` | `export` | `upgrade` |
`rollback` operation as an ordered list of `steps`, each with a `status` of
`pending` → `staged` → `applied` (or `rolled-back` / `failed`). A manifest always
records:

- a sanitized `source_description` / `target_description` (never a literal
  personal path or account identifier),
- `staging_path` (symbolic, never absolute),
- `atomic_publish` (whether the final step is a single atomic swap),
- an optional `checksum_manifest_ref` used to verify staged output before publish,
- an optional `rollback_ref` to a prior migration or bundle revision.

## Recovery walkthrough

1. A migration's steps are all `staged` and its `checksum_manifest_ref` has been
   verified against the staging directory.
2. Publish executes the atomic step; steps move to `applied`.
3. If a problem is discovered afterward, a new migration of `kind: rollback` is
   created referencing the prior migration/bundle revision via `rollback_ref`; its
   own steps go through the same staged → verified → atomic-publish flow. Rollback
   is a forward operation (a new migration), never an edit of history.
4. Nothing from the prior state is deleted as part of this — it remains in
   `state/archive/` or as a retained prior bundle revision until an operator
   explicitly prunes it.
