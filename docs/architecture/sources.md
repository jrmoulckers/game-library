# Source References and Contracts

This is the reference table backing `source.schema.json` — every external system
the catalog reads from or reconciles against, its contract shape, and its current
stability. See [`adapters.md`](adapters.md) for behavioral rules and
[ADR-0006](decisions/0006-adapter-boundaries-and-ownership.md) for the reasoning.

**Status:** `source.schema.json` (and this reference table) is **forward-looking**
— no Go type in the current `gamelib` implementation reads or writes a
`library/sources/<source_id>.json` file yet. Today, adapter behavior is
implemented directly per-adapter (see `internal/identity`, `internal/profile`'s
`adapterDestination`) rather than declared in a per-source record. This document
still reflects the agreed target contract for each integration.

| `kind` | `contract` | `auth_mode` | `capabilities` | `stability` | Notes |
|---|---|---|---|---|---|
| `steam` | `rest` | `api-key` | `inventory`, `metadata` | `stable` | Drives `pc.appid` identity directly. |
| `steamgriddb` | `rest` | `api-key` | `artwork` | `stable` | Artwork only; never drives identity or mapping. |
| `playnite` | `export-file` | `none` | `inventory`, `mapping` | `stable` | File export only; Playnite's own database is never written. |
| `romm` | `openapi` | `api-key` | `inventory`, `metadata`, `artwork` | `stable` | OpenAPI preferred over scraping wherever available. |
| `es-de` | `export-file` | `none` | `inventory`, `metadata` | `stable` | Config/gamelist-file based. |
| `epic` | `manual` | `unsupported` | `inventory` | `unsupported`/`best-effort` | No durable contract today; inventory-only. |
| `ea` | `manual` | `unsupported` | `inventory` | `unsupported`/`best-effort` | No durable contract today; inventory-only. |
| `ubisoft` | `manual` | `unsupported` | `inventory` | `unsupported`/`best-effort` | No durable contract today; inventory-only. |
| `screenscraper` | `rest` | `api-key` | `artwork`, `metadata` | `best-effort` | Retro-focused metadata/artwork source. |
| `manual` | `manual` | `none` | `inventory`, `artwork`, `metadata`, `mapping` | `planned`/`stable` | Operator-entered data; always highest trust for what it covers. |

## Contract shape guidance

- **`openapi`** — a machine-readable spec exists and is used to generate/validate
  calls. Preferred whenever available (e.g. RomM) over ad hoc scraping.
- **`rest`** — a documented but not formally spec'd HTTP API (e.g. Steam's public
  endpoints, SteamGridDB, ScreenScraper).
- **`export-file`** — the source of truth is a file the external tool produces
  (Playnite library export, ES-DE gamelists); the adapter reads that file, it
  never talks to the external tool's live process or database.
- **`scrape`** — HTML/UI scraping; last resort, always `best-effort` at most, and
  always plan/staging-first like everything else.
- **`manual`** — operator-entered data with no external system behind it at all
  (e.g. hand-confirmed mappings, manually sourced licensing notes).

## Adding a new source

1. Add `library/sources/<source_id>.json` conforming to `source.schema.json`,
   picking the most conservative accurate `contract`/`stability`/`capabilities`.
2. Build (or extend) an adapter that only ever writes to `state/**`
   (`inventory-observation.schema.json`, `migration-manifest.schema.json`) —
   never directly into `library/**`.
3. Anything the adapter surfaces is subject to the same retention policy
   precedence and promotion rules as any other data (see
   [`identity-and-policy.md`](identity-and-policy.md)) before it becomes canonical.
4. Update the table above in this doc (kept in sync manually; it is documentation,
   not generated).
