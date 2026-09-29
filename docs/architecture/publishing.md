# Profile exports, local publishing, and observed parity

Implemented by `internal/publishing` and `/api/publishing/*`; see ADR-0009.
Catalog migration CLI commands remain plan-only. Nothing here authorizes
unattended live writes or changes Syncthing, a remote host, or a Playnite database.

## What a device badge means

`config/topology.json` describes intended platform applicability. It is a
host-local gamelib sidecar, **not synced Decky metadata**. A Steam profile is
stored once for use on PC, Deck, and Ally; that does not establish deployment
or parity on any of those devices.

`config/targets.json` describes actual frontend targets independently:

```json
{
  "version": 1,
  "targets": [
    {
      "id": "pc-steam",
      "name": "PC Steam",
      "device": "pc",
      "platform": "steam",
      "adapter": "steam"
    }
  ]
}
```

An optional `root` is an absolute **private host-local** directory. Configure
it only after verifying which frontend/device it represents. Leave remote
devices empty unless an actual mounted folder has been verified; there is no
SSH, remote service, or implicit network probe. A retro platform declaration
does not establish whether ES-DE, RetroDECK, or another frontend is installed.
Unknown frontend adapters are reported as unsupported, not silently mapped.

Each comparison freshly hashes the source and target files:

The report separates **all compared files** from `publishable` subset counts.
Blocked files remain in the full comparison and can still differ, even when
every supported copy matches. Such a result is never labeled fully in sync.
An all-matching supported subset explicitly previews zero frontend writes.

| State | Meaning |
| --- | --- |
| matching | Current destination bytes have the source SHA-256. |
| missing | The target directory is observed and this exact file is absent. |
| different | The current destination SHA-256 differs from the source. |
| unknown | A file cannot be safely read/contained, or its target is not observed. |
| unobserved | No actual frontend folder is configured. |
| offline | A configured frontend folder is unavailable on this host. |
| unsupported | This frontend has no enabled local publishing adapter. |

Receipts are historical evidence of an attempted copy, not current parity.
Only local Steam grid copies are enabled. Existing Playnite/ES-DE/RomM
`export plan` adapters retain their CLI contracts, but no database writer,
remote API execution, or unverified retro deployment has been added.

## Sources and platform separation

Canonical selections reuse `profile.BuildExportPlan` and content-addressed
asset paths. The organizer's current bound Steam artwork sets also support a
read-only compatibility export via that same adapter's identity, filename and
extension rules. This does **not** promote legacy/generated payloads into
canonical assets, write a bundle pointer, or make live grids canonical.
No title matching or asset substitution occurs.

Exact Steam/Playnite metadata aliases may resolve a display title, but their
game identities, artwork membership, and profile coverage stay platform-local.
Same hashes can still indicate exact-copy sharing without implying membership.

The Decky v1 ABI is untouched: topology and targets are separate sidecars.
`steam-default`, `artwork: null`, and `.deck-profile-empty` retain built-in-art
semantics. An empty profile is not a clearing/deletion export. Unsupported
extensions, content/extension mismatches, layout JSON, and ambiguous source
filename variants are visibly blocked from copying; their bytes can still be
compared read-only. Nothing is renamed or transcoded to make it fit an adapter.

## Preview, export, approve, restore

Open a profile, configure an actual frontend folder, and choose **Compare now**.
**Preview export** shows the exact target folder, every file's current and
source SHA-256, which files will be added/replaced/left matching, and every
blocked file and reason.

**Generate local export** writes independently copied, verified bytes under
`exports/<preview-digest>/files/` in the local workspace. The manifest retains
all exclusions. A repeat is idempotent; staging drift fails rather than being
overwritten. No live approval is implied by generating an export.

The unchecked approval checkbox and **Approve these copies** authorize only
that preview digest. The engine recomputes the preview before writing:
profile identity, adapter actions, source roots, target identity/root, all
source hashes and observed destination hashes are bound into the digest.
Changed input invalidates approval. Unsupported, unsafe, offline, and
unobserved targets cannot be published.

Publication:

1. Verify/stage every eligible copy before the first frontend replacement.
2. For each changed file, recheck its destination and independently copy the
   predecessor into `publishing/<digest>/backup/` before replacing it.
3. Flush a write-ahead receipt listing the prepared file, then recheck the
   target, copy to a temporary sibling, verify SHA-256, flush, and atomically
   replace. Matching and blocked files are untouched. There are no deletes.
4. Mark the receipt complete only after all copies finish. A repeat succeeds
   without writes only if the same approved source set still matches.

Steam reads a flat grid rather than an atomic bundle pointer. **Atomicity is
per file, not an all-or-nothing directory transaction.** A failure can leave a
partial operation; its prepared receipt and backups remain available through
**Published copies and recovery**. No success-shaped fallback or automatic
resume is used.

**Preview rollback** shows exact predecessor hashes and the target folder.
Its own approval digest is required. Rollback rechecks current files and
backup hashes, restores replacements atomically, and refuses subsequent edits.
Added files are explicitly counted and retained because rollback never
deletes artwork. An already-restored predecessor is a no-op. A failed rollback
can be previewed again without reconstructing lost bytes.
Each approved restore preview is retained as an immutable
`publishing/<operation>/rollbacks/<restore-digest>.json` record before writes,
and the original receipt records `restoring`/`rolled-back` plus its restore
digest. Interrupted restore attempts remain discoverable.

## Filesystem and request boundary

Source, workspace and target trees must not overlap. Relative paths reject
traversal, drive/UNC syntax, alternate streams, reserved Windows names, and
links in existing ancestors, including the root. Export role/case collisions
and target alternate-image-extension collisions fail closed. Existing Steam
layout JSON is not treated as an alternative image and is never replaced.

A process mutex plus an OS-released workspace file lock excludes concurrent
publishers sharing a workspace on Windows/Linux. This does not lock Steam or
another writer in a different workspace: close competing artwork editors and
frontends before publishing. Hash checks detect drift, but cannot make
uncooperative external writers participate in a filesystem transaction.
The trust boundary remains the local OS user; this is not a defense against
a hostile process running as that user. Backups/receipts/exports are private
local state; portable permission bits are not a Windows ACL guarantee.

Network exposure is unchanged. Writes still require the exact loopback Host/Origin,
JSON content type and `X-Gamelib-Csrf` from `/api/bootstrap`.

| Endpoint | Contract |
| --- | --- |
| `GET/PUT /api/publishing/targets` | Local targets; PUT requires current `baseDigest` plus `targets`. No artwork writes. |
| `GET /api/publishing/parity?profile=steam/standard` | Current per-target hashes/states and exclusions. |
| `POST /api/publishing/preview` | `{profile,target}`; returns `plan` plus private target folder. Read-only. |
| `POST /api/publishing/stage` | `{profile,target,approval}`; writes only workspace export staging. |
| `POST /api/publishing/publish` | Same request; explicit exact-preview approval for eligible local copies. |
| `GET /api/publishing/history` | Durable completed/partial/rolled-back receipts. |
| `POST /api/publishing/rollback-preview` | `{target,operation}`; read-only restore preview and retained-addition count. |
| `POST /api/publishing/rollback` | `{target,operation,approval}`; approved non-deleting restore. |

Plans/receipts are versioned host-local DTOs, not Decky schema extensions.
They embed the existing `model.Manifest` adapter actions, but do not authorize
executing an arbitrary uploaded migration manifest. The server reconstructs
export sources from the configured topology; rollback uses its own retained
receipt. Changing target roots prevents rollback until the original verified
target configuration is restored.

## Executable acceptance

`go test ./internal/publishing ./internal/dashboard ./internal/organizer
./internal/coverage` covers preview approval, drift, collisions, staging,
idempotence, atomic failure, backups, partial receipts after restart,
rollback drift/non-deletion, locks, platform separation, and API hardening.

Opt-in real-library acceptance is read-only, with counts-only output:

```text
GAMELIB_LIVE_WORKSPACE=<existing-dashboard-workspace>
GAMELIB_LIVE_STEAM_GRID=<verified-local-Steam-grid>
go test ./internal/publishing -run Live -v
```

It never calls staging/publish/rollback or saves target configuration.
Neither a successful comparison nor this documentation approves live copies.
Set `GAMELIB_LIVE_PREVIEW_FILE` only when an exact private dry-run report is
needed in a local artifact directory. It contains the verified target path,
full preview/hash lock, and an explicit replacement list; it must never be
committed or attached to a public PR. This optional report write still does not
modify artwork or grant publication approval.
