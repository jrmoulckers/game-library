# ADR-0009: Hash-observed profiles and approved local Steam publishing

- Status: Accepted
- Date: 2026-09-29
- Owner: jrmoulckers

Partly supersedes the plan-only/local-write boundary in ADR-0007 and the
publishing seam deferred by ADR-0008. Remote deployment ownership in ADR-0006
and canonical bundle-pointer design in ADR-0005 remain unchanged.

## Context

An intended hardware badge and a single central profile do not establish that
any frontend has adopted it. Existing export adapters only produce plans.
The organizer needs practical per-target comparisons and copy-first exports,
without turning into a gate console or silently mutating private artwork.

## Decision

- Keep intended topology and actual frontend target configuration separate,
  private, host-local, and outside the frozen Decky v1 ABI.
- Measure parity with current SHA-256 observations. Missing, different,
  unknown, offline, unsupported and unobserved states remain distinct.
- Reuse existing adapter naming/identity/extension rules. Export the owner's
  existing bound Steam sets read-only as compatibility inputs; do not promote
  legacy/generated/live directories into canonical storage.
- Enable local Steam grid staging/publishing only. Require an exact target,
  file and hash preview and a distinct explicit approval; revalidate before
  mutation. No remote execution or Playnite database writes.
- Back up replacements, keep durable prepared receipts, verify copied bytes,
  and atomically replace each file. Never delete artwork. Rollback requires its
  own preview/approval, restores predecessors and retains additions.
- Reject containment and collision hazards. Keep unsupported source bytes
  visible but excluded, rather than renaming/transcoding or selecting variants.
- Keep Steam and Playnite artwork/profile coverage separate even when exact
  metadata aliases resolve the same title.

## Consequences

The profile detail gains useful copy/parity controls without a review console.
Steam's flat grid cannot provide bundle-pointer atomicity: a multi-file
operation can be partial, but prepared receipts and backups make it
recoverable. Rollback deliberately does not remove newly added files.
External frontend writers must be stopped by the operator; they do not
participate in gamelib's workspace lock. Live publication still requires
target-specific consent after reviewing the actual dry-run, not merely this
implementation request.

## Evidence

`internal/publishing/*_test.go` and `internal/dashboard/publishing_test.go`
execute positive and negative approvals, hash drift, collisions, copy failure,
backup/rollback, partial recovery and locking using temporary fixtures.
`internal/publishing/live_acceptance_test.go` provides opt-in counts-only
read-only real-library comparison. `docs/architecture/publishing.md` records
the endpoint and recovery contracts.

Falsifiable by: a target credited with parity without current hashes, an
unapproved live copy, a replacement without retained predecessor bytes,
cross-platform artwork membership from metadata aliases, or deleted artwork.
