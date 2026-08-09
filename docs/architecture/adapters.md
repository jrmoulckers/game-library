# Adapter Contracts and Boundaries

See ADR-0006 for the decision behind this document. This is the practical
reference for what each integration is allowed to do and where its boundary sits.

## Universal rules

1. **Plan/staging first.** Every adapter's output starts life under `state/`
   (`inventory-observation.schema.json`, `inventory-report.schema.json`,
   `migration-manifest.schema.json`). No adapter writes directly into
   `library/**`, `bundles/**`, or a live frontend root. In the current
   implementation this is enforced structurally: `gamelib`'s `import plan`,
   `bundle plan`, and `export plan` subcommands only ever emit a
   `migration-manifest.schema.json`-shaped plan (`model.Manifest`); there is no
   `apply` subcommand yet, so nothing in this repo's tooling can write to
   `library/**`/`bundles/**` on its own today.
2. **Promotion follows policy.** Moving staged/observed data into `library/**` is
   subject to the same retention/approval rules as any other data (see
   [`identity-and-policy.md`](identity-and-policy.md)).
3. **Every integration is a `source.json`.** Isolating provider behaviour behind a
   thin, single-purpose, declared seam is
   [`ENG-INT-001`](https://github.com/jrmoulckers/engineering/blob/v0.1.0/principles/platforms/integration-boundaries.md#thin-typed-adapters).
   Repo-specific: `source.schema.json` records an integration's `contract`
   (`openapi` | `rest` | `export-file` | `scrape` | `manual`), `auth_mode`,
   `capabilities`, and `stability`, and no adapter code should assume more than
   its `source.json` declares.
4. **No secrets in the tree.** Per
   [`ENG-SEC-001`](https://github.com/jrmoulckers/engineering/blob/v0.1.0/principles/assurance/security-and-privacy.md#secret-lifecycle),
   `source.schema.json`'s `endpoint_ref` is a sanitized pointer (doc anchor, env
   var name), never a literal URL/IP/hostname/credential.

## Per-integration notes

### Steam

- `contract: rest` (Steam's public/partner APIs where available), `stability: stable`
  for AppID-keyed metadata.
- Drives `pc.appid` identity directly; highest-trust PC source.

### Playnite

- `contract: export-file` — Playnite's file-based library export is the supported
  integration path.
- **Playnite's own database is never written by this system, in either direction.**
  Only its export is read; the only artifact produced is an adapter-only mapping
  file (`library/mappings/playnite/<uuid>.json`, **forward-looking** — today
  expressed inline as the `"playnite"` key of a profile's `games[].identities`
  map, see [`identity-and-policy.md`](identity-and-policy.md)), never a write-back
  into Playnite.
- Mapping `confidence` (`exact` | `fuzzy` | `manual`) must be recorded on every
  mapping so low-confidence auto-matches are distinguishable and reviewable.

### ES-DE (EmulationStation Desktop Edition)

- `contract: export-file` (its own config/gamelist file conventions).
- Plan/staging first, same as every other adapter: reconciled against
  `library/games/retro/**` and `library/releases/retro/**` via SHA-256, never by
  filename.

### RomM

- `contract: openapi` preferred — RomM exposes an OpenAPI-described contract, and
  it is used in preference to scraping wherever available. `stability: stable`
  for the parts of that contract actually consumed.
- Provides retro inventory/metadata that gets reconciled the same way as ES-DE.

### Epic, EA (App/Origin), Ubisoft (Connect)

- `capabilities: [inventory]` only, `stability` at most `best-effort` (frequently
  `unsupported`), `contract` typically `manual` today, because none currently
  offers a stable, durable contract this system can build on.
- These sources tell the catalog "this game exists in this store", nothing more —
  they never drive artwork, metadata, or mapping decisions, and are never used to
  auto-promote anything.
- If any of these gains a real stable contract in the future, that's a
  `source.json` update (new `contract`/`stability`) plus a new/updated adapter —
  not a schema change (ADR-0006).

## Live-apply boundary: homelab ownership

Everything above produces or reconciles data inside the synced `GamingProfiles`
tree (ADR-0001). Getting a generated bundle (ADR-0005) onto a specific device's
**live, device-local frontend directories**, and any runtime concern that follows
from that (restarting a frontend, reloading a plugin, choosing which revision a
given device should be running), is owned by the **homelab environment** —
specifically the **CT601** host and its **Syncthing** configuration — not by this
repository's tooling.

This repo's responsibility stops at: produce a verified, staged, hash-locked
bundle revision and a correct `current.json` pointer. Whether/when a given device
adopts that pointer, and how it's actually copied into that device's live paths,
is homelab-side automation's decision, using the safety rules in
[`migration-and-recovery.md`](migration-and-recovery.md) (copy-first, no
hardlinks/symlinks, atomic publish, rollback via `previousRevision`).
