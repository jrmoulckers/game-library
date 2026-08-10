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
| `playnite` | `export-file` | `none` | `inventory`, `mapping` | `stable` | Export-compatible contract; the organizer may resolve names from a stable local database opened strictly read-only. |
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
  (Playnite library data, ES-DE gamelists); the adapter never talks to the
  external tool's live process and never writes its database.
- **`scrape`** — HTML/UI scraping; last resort, always `best-effort` at most, and
  always plan/staging-first like everything else.
- **`manual`** — operator-entered data with no external system behind it at all
  (e.g. hand-confirmed mappings, manually sourced licensing notes).

## Local organizer metadata

The dashboard resolves display names from optional local files without changing
the source contract or writing back:

- Steam names come from `appinfo.vdf`, manifests in locally declared Steam
  library folders, and per-account shortcuts, in that order.
- Playnite names and exact storefront identity come from a stable,
  unencrypted LiteDB v4 games collection opened strictly read-only. The reader
  never creates a log, upgrades, compacts, rebuilds, or replays recovery state.
  Playnite names the collection `Game` (singular); `games` is also accepted for
  older databases.
- ES-DE names come from local `gamelists/<system>/gamelist.xml` files.
  The standard and RetroDECK path conventions are covered synthetically and
  remain pending live verification on a RetroDECK device.

These readers are deliberately resilient in one specific way that synthetic
fixtures easily miss: `appinfo.vdf` is a live cache that may contain records
this reader does not model. Such a record is skipped rather than guessed at,
and never discards the records around it. Resilience never extends to
inventing a value: parsing within a record stays strict, and a corrupt or
out-of-range reference yields no title at all.

Synthetic fixtures cannot prove these readers work, because a fixture can be
shaped to match a broken parser. `internal/metadata` therefore carries a live
acceptance test that runs the real readers against real local files. It is
skipped unless pointed at them, and reports only counts:

```
GAMELIB_LIVE_STEAM_GRID=<steam>/userdata/<account>/config/grid
GAMELIB_LIVE_PLAYNITE=<playnite>/library/files
go test ./internal/metadata -run TestLiveSourcesResolveRealTitles -v
```

Malformed, busy, unsupported, or changing metadata leaves a labeled identity
placeholder instead of failing inventory. Titles never create identity edges;
Playnite may join Steam only through the reviewed Steam plugin ID and exact
numeric GameId.

These caches live only in process memory. Source availability is scoped to the
machine running `gamelib`: an absent Deck-local source is neutral information,
and the dashboard never pings, mounts, or connects to another device.

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
