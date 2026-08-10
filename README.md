# game-library

Canonical game artwork, metadata, profiles, and recovery tooling.

This repository owns the **contract and tooling**: JSON Schemas, architecture
docs, and the `gamelib` Go CLI that reads/validates/plans against that contract.
The actual catalog — games, releases, artwork, profiles — lives in a separate,
private, Syncthing-synced tree (`GamingProfiles`) and is never checked into this
repository. See
[ADR-0001](docs/architecture/decisions/0001-two-tree-topology.md) for why.

## Quick start

- **Understand the design:** start at
  [`docs/architecture/README.md`](docs/architecture/README.md), then
  [`docs/architecture/tree.md`](docs/architecture/tree.md) for the full canonical
  layout.
- **Look up a contract:** schemas live under
  [`schemas/v1/`](schemas/v1/README.md), one file per record type — the
  implemented contracts (canonical profile, policy file, inventory
  report/observation, migration/export manifest, and the frozen Decky v1 legacy
  profile ABI) plus a few forward-looking ones not yet backed by Go code (game,
  retro release, Playnite mapping, source, catalog, asset content metadata,
  bundle lock/current pointer) — see the schema table's Notes column for which
  is which.
- **Identity and retention rules:**
  [`docs/architecture/identity-and-policy.md`](docs/architecture/identity-and-policy.md).
- **Integrations (Steam, Playnite, ES-DE, RomM, Epic/EA/Ubisoft):**
  [`docs/architecture/adapters.md`](docs/architecture/adapters.md) and
  [`docs/architecture/sources.md`](docs/architecture/sources.md).
- **Safety, staging, and rollback:**
  [`docs/architecture/migration-and-recovery.md`](docs/architecture/migration-and-recovery.md).
- **Why things are shaped this way:** [`docs/architecture/decisions/`](docs/architecture/decisions/)
  (ADRs).

### Using the CLI

`gamelib` (module `github.com/jrmoulckers/game-library`, `cmd/gamelib`) is a
**plan/dry-run-only** tool today — every subcommand reads and reports, or writes
a plan document for a human/future-tool to review; **none of them applies
changes to the synced tree**:

```
go build -o bin/gamelib ./cmd/gamelib
gamelib inventory scan                     # scan configured roots
gamelib report summary                     # sanitize a private inventory report
gamelib duplicates report                  # exact duplicate groups from a private report
gamelib identity propose                   # propose steam:/playnite:/retro: identity hints
gamelib import plan                        # plan (never apply) policy outcomes/copies
gamelib profile resolve                    # verify a profile's canonical asset closure
gamelib bundle plan                        # plan (never apply) a retained bundle revision
gamelib export plan --adapter <adapter>    # steam|decky|playnite|esde|romm staging plan
gamelib validate profile|decky-v1|decky-catalog|inventory <path>
gamelib manifest verify                    # verify a plan file's expected SHA-256
gamelib serve                              # loopback-only local dashboard (plan/draft review; no apply)
gamelib version
```

Run the leaf command with `-h` (for example, `gamelib inventory scan -h`) for
flags. See `configs/examples/config.json` and
`configs/examples/policy.json` for example configuration/policy documents, and
`testdata/profiles/example.json` / `testdata/decky/*.json` for example profile
documents matched to `schemas/v1/canonical-profile.schema.json` and
`schemas/v1/decky-profile-v1.schema.json` respectively.

### The local dashboard (`gamelib serve`)

`gamelib serve` starts a loopback-only browser dashboard around the same
read/plan/draft Go contracts the CLI uses — see
[ADR-0007](docs/architecture/decisions/0007-local-dashboard.md). It binds only
to an explicit loopback literal (`127.0.0.1` or `::1`; wildcard, hostname, LAN,
and public addresses are rejected), and its host-local writes are limited to
the active configuration and validated policy/profile drafts, plus immutable
create-if-absent plan and Gate A/B/C review artifacts — there is no apply,
publish, delete, prune, or rollback endpoint, and Gate C reviews are always
recorded as non-executable.

The dashboard itself is server-rendered Go `html/template` plus small
progressive-enhancement ES modules (no build step, no framework, no CDN
dependency, no inline script/style); every JavaScript module only calls this
process's own JSON API — it never re-implements policy precedence, identity
mapping, duplicate classification, profile resolution, or plan/manifest
analysis, all of which stay in the Go packages under `internal/`. With
JavaScript disabled or blocked, each section explains the equivalent `gamelib`
CLI command instead of rendering a blank page.

Opening `http://<listen-address>/` presents the artwork organizer:

- **Library** — platform cards with artwork mosaics, game counts, coverage, and
  missing-art counts. This is the landing view once a source is configured.
- **Platform detail** — a searchable, sortable cover grid with straightforward
  artwork-coverage filters.
- **Game detail** — identities, every available artwork role, missing-role and
  fallback explanations, profile use, exact-copy sharing, media facts, large
  previews, and direct selection of an asset for a saved profile.
- **Profiles** — visual profile cards and plain-language Decky fallback
  semantics. Detailed draft editing remains available in Advanced.
- **Sources** — conventional Steam, Playnite/ExtraMetadata, GamingProfiles, and
  RetroDECK/ES-DE locations are detected locally on Windows and Linux. The
  owner confirms found folders once; manual configuration remains available.
  Organizer rescans report per-source progress and publish partial in-memory
  results as each source completes.
- **Advanced** — the existing setup, overview, artwork table, identity queue,
  duplicates, policy, profile draft editor, plans and Gate A/B/C reviews,
  adapter readiness, and recovery/integrity history remain intact in one
  collapsed area.

Advanced includes:

- **First-run/manual setup** — edit root id/kind/path/system values, validate
  them without writing, set the global policy default, and save with
  base-digest conflict recovery.
- **Overview and artwork browser** — root/file/media totals, validation
  warnings, duplicate and adapter summaries, plus the complete paginated,
  filterable observation table.
- **Identity queue** — deterministic (high-confidence) identity proposals
  shown separately from lower-confidence proposals and unmapped items, with
  their evidence; nothing is ever auto-merged, and creating a Gate A review
  requires explicitly marking every listed item reviewed first.
- **Duplicates** — exact SHA-256 duplicate groups classified as a canonical
  opportunity, an expected generated/export copy, or needing review, each
  with a plain-language explanation and no delete control.
- **Policy** — the active policy's digest alongside a locally saved policy
  draft rule editor (source/system/role/assetSha256/mode), the exact
  precedence weights (assetSha256 8 > role 4 > system 2 > source 1 > global
  default), an impact preview showing winning-rule counts per resolved mode,
  and draft save with base-digest conflict recovery. Saving never promotes a
  draft to the active/canonical policy.
- **Profiles** — list, create, and edit local profile drafts (id/name/
  description/theme, plus JSON-friendly structured editors for games and
  mods matching the canonical profile schema), a closure/resolve preview, and
  a plan-only adapter export preview that, for Decky, states its `artwork`/
  `.deck-profile-empty` semantics explicitly.
- **Plans & gates** — generate import/bundle/export plans, inspect their
  actions (reason, hash, bytes) in a real table, run the exact manifest
  analysis Gate C needs (effects, conflicts, backup/new bytes, per-root free
  space sufficiency), and record Gate A/B/C reviews. Apply is visibly
  unavailable everywhere on this page, because it does not exist as an
  endpoint on this server.
- **Adapters** — plan-only readiness for Steam, Playnite (read-only sidecar
  preview), Decky, ES-DE, and RomM, each with an explanation of its
  boundary (no Steam/Playnite write-back, no homelab live publication).
- **Recovery** — the immutable local plan/Gate A/B/C history with digest and
  integrity status. There is no applied/rolled-back history and no
  prune/delete/apply control anywhere on this page.

```
gamelib serve --listen 127.0.0.1:8787
```

Flags: `--listen` (default `127.0.0.1:8787`), `--workspace` (override the
platform-local workspace directory), `--config` (override the active
configuration file path), `--inventory-report` (review an existing private
inventory report instead of scanning the active configuration's roots for the
in-memory snapshot), `--catalog` (canonical catalog root used for profile
resolve/bundle previews and manifest analysis).

The dashboard trusts the local OS user rather than adding an account/login
system: any other process running as you shares this same loopback trust
boundary, matching ADR-0007.

## Repository layout

```
cmd/gamelib/           gamelib CLI entrypoint (Go); `serve` embeds the local
                        dashboard's server-rendered shell via internal/dashboard
internal/              Go packages implementing the contract: config, model,
                        policy, inventory, identity, media, manifest, profile,
                        decky, report, schema, workspace (host-local
                        config/draft/artifact writes), review (read-only
                        dashboard review domain: overview, observations,
                        media, identity, duplicates, policy impact, profile
                        previews, plan persistence, manifest analysis, Gate
                        A/B/C reviews), dashboard (loopback HTTP server,
                        html/template shell, embedded CSS, and vanilla ES
                        module static assets under internal/dashboard/static)
configs/examples/      Example config.json / policy.json documents
testdata/              Example profile / Decky v1 fixtures used by tests and
                        by this repo's schema validation
reports/baseline/      Sanitized, aggregate-only inventory baselines (see its
                        own README) — never exact paths/hashes/actions
docs/architecture/     Architecture docs and ADRs (this repo's design record)
schemas/v1/            JSON Schema 2020-12 contracts for the synced catalog tree
```

## Authority

This repository is contract and tooling, not a product application. It holds no
roadmap, metric, experiment, or compliance evidence of its own. Where an
organization-wide obligation applies, cite it by stable ID rather than restating
it here; pin to a commit SHA when exact wording matters.

- Product obligations and outcomes:
  [jrmoulckers/product](https://github.com/jrmoulckers/product)
- Engineering mechanisms and evidence:
  [jrmoulckers/engineering](https://github.com/jrmoulckers/engineering)
- Design and interface: [jrmoulckers/studio](https://github.com/jrmoulckers/studio)
- Governance, automation, and shared agent assets:
  [jrmoulckers/.github](https://github.com/jrmoulckers/.github)

The obligations that bear most directly on this repository's contract are the
Product compliance ones. `PROD-COMP-006` (permit only reviewed software
distribution) requires a known license classification and publishing boundary
before software is used or distributed — the analogous question for third-party
artwork and metadata is what motivates the required `provenance` and `license`
fields on [`schemas/v1/asset.schema.json`](schemas/v1/asset.schema.json) and the
`quarantined` retention outcome for licensing concerns. `PROD-COMP-005` (bound
retention and terminal disposition) is the obligation behind the layered
retention policy in
[ADR-0002](docs/architecture/decisions/0002-content-addressed-assets-and-retention.md),
and `PROD-COMP-002` (bound processing by purpose and necessity) is why sanitized
baselines under `reports/baseline/` carry aggregates only.

Compliance obligations establish governance and qualified-review triggers; they
are not legal advice, and nothing in this repository determines licensing
validity for a given asset.

Go source, tests, and CI configuration in this repository are owned by their
respective engineering roles per each area's `AGENTS.md`; this README's
"Quick start"/"Repository layout" sections are maintained for navigation only —
architecture and schema content itself lives under `docs/architecture/` and
`schemas/v1/`.
