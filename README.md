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
gamelib serve                              # loopback-only artwork organizer dashboard (no apply)
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
read-only Go contracts the CLI uses — see
[ADR-0007](docs/architecture/decisions/0007-local-dashboard.md) and
[ADR-0008](docs/architecture/decisions/0008-organizer-only-dashboard.md). It
binds only to an explicit loopback literal (`127.0.0.1` or `::1`; wildcard,
hostname, LAN, and public addresses are rejected), and its host-local writes are
limited to the active configuration and validated profile drafts. There is no
apply, publish, delete, prune, or rollback endpoint.

The dashboard itself is server-rendered Go `html/template` plus small
progressive-enhancement ES modules (no build step, no framework, no CDN
dependency, no inline script/style); every JavaScript module only calls this
process's own JSON API — it never re-implements identity mapping, profile
resolution, or media containment, all of which stay in the Go packages under
`internal/`. With JavaScript disabled or blocked, each section explains the
equivalent `gamelib` CLI command instead of rendering a blank page.

Opening `http://<listen-address>/` presents the artwork organizer:

- **Library** — platform cards with artwork mosaics, game counts, coverage, and
  missing-art counts. Human-readable titles come from local Steam
  `appinfo.vdf`/manifests/shortcuts, Playnite metadata, or ES-DE gamelists; a
  safe labeled placeholder remains when local metadata is unavailable.
- **Platform detail** — a searchable, sortable cover grid with straightforward
  artwork-coverage filters. Covers are served as cached thumbnails, and
  searching filters the existing grid rather than rebuilding it, so typing stays
  responsive over a full library.
- **Game detail** — identities, every available artwork role, missing-role and
  fallback explanations, profile use, exact-copy sharing, media facts,
  full-resolution previews, and direct selection of an asset for a saved
  profile.
- **Profiles** — visual profile cards, plain-language Decky fallback semantics,
  and profile creation. Choosing artwork from a game's detail view assigns it to
  a profile here.
- **Sources** — conventional Steam, Playnite/ExtraMetadata, GamingProfiles, and
  RetroDECK/ES-DE locations are detected locally on Windows and Linux. The
  owner confirms found folders once; manual root editing and validation remain
  available under the **Source folders** disclosure. Organizer rescans report
  per-source progress and publish partial in-memory results as each source
  completes. Source availability is scoped to the current device: a desktop
  without RetroDECK media is complete, not erroneous, and the dashboard never
  probes a Steam Deck or other remote host.

Evidence tooling — duplicate classification, identity proposals, policy impact,
plans, and manifest analysis — lives in the CLI commands above rather than in
the browser.

```
gamelib serve --listen 127.0.0.1:8787
```

Flags: `--listen` (default `127.0.0.1:8787`), `--workspace` (override the
platform-local workspace directory), `--config` (override the active
configuration file path), `--inventory-report` (review an existing private
inventory report instead of scanning the active configuration's roots for the
in-memory snapshot), `--catalog` (canonical catalog root used for profile
previews).

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
                        config/draft writes), review (read-only dashboard
                        domain: organizer read model, media serving, and
                        profile previews), dashboard (loopback HTTP server,
                        html/template shell, thumbnail cache, embedded CSS, and
                        vanilla ES module static assets under
                        internal/dashboard/static)
configs/examples/      Example config.json / policy.json documents
testdata/              Example profile / Decky v1 fixtures used by tests and
                        by this repo's schema validation
reports/baseline/      Sanitized, aggregate-only inventory baselines (see its
                        own README) — never exact paths/hashes/actions
docs/architecture/     Architecture docs and ADRs (this repo's design record)
schemas/v1/            JSON Schema 2020-12 contracts for the synced catalog tree
scripts/               Repo tooling, incl. fetching the Engineering lint config
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

### Engineering practice

Engineering rules are cited by `ENG-*` ID and are not restated here. Resolve any
ID against
[`principles/index.json`](https://github.com/jrmoulckers/engineering/blob/v0.2.3/principles/index.json).
The Go-specific technique for satisfying them is
[practices/go.md](https://github.com/jrmoulckers/engineering/blob/v0.2.3/practices/go.md);
this repository has no npm surface, so the `@jrmoulckers/*` presets do not apply.

The IDs that bear most directly on this repository:

| ID | Where it shows up here |
| --- | --- |
| [`ENG-ARCH-001`](https://github.com/jrmoulckers/engineering/blob/v0.2.3/principles/architecture/boundaries-and-contracts.md#minimal-directed-boundaries) | `internal/**` is the boundary; `cmd/gamelib` holds only `main` and flag parsing |
| [`ENG-ARCH-002`](https://github.com/jrmoulckers/engineering/blob/v0.2.3/principles/architecture/boundaries-and-contracts.md#explicit-additive-contracts) | [`schemas/v1/`](schemas/v1/README.md) is the versioned published contract |
| [`ENG-ARCH-003`](https://github.com/jrmoulckers/engineering/blob/v0.2.3/principles/architecture/boundaries-and-contracts.md#durable-decisions) | [`docs/architecture/decisions/`](docs/architecture/decisions/) |
| [`ENG-INT-001`](https://github.com/jrmoulckers/engineering/blob/v0.2.3/principles/platforms/integration-boundaries.md#thin-typed-adapters) | [`docs/architecture/adapters.md`](docs/architecture/adapters.md) |
| [`ENG-SEC-001`](https://github.com/jrmoulckers/engineering/blob/v0.2.3/principles/assurance/security-and-privacy.md#secret-lifecycle) | `source.json`'s `endpoint_ref` is a sanitized pointer (doc anchor, env var name), never a literal credential |
| [`ENG-SEC-002`](https://github.com/jrmoulckers/engineering/blob/v0.2.3/principles/assurance/security-and-privacy.md#verified-supply-chain) | CI pins every external action to an immutable commit SHA rather than a mutable tag |
| [`ENG-OBS-005`](https://github.com/jrmoulckers/engineering/blob/v0.2.3/principles/operations/observability.md#redacted-observable-evidence) | `sanitized` inventory reports, safe to attach to a bug report |
| [`ENG-TEST-004`](https://github.com/jrmoulckers/engineering/blob/v0.2.3/principles/assurance/testing.md#distinct-static-signals) | the independent CI signals below |
| [`ENG-TEST-007`](https://github.com/jrmoulckers/engineering/blob/v0.2.3/principles/assurance/testing.md#positive-and-negative-polarity) | paired accept/reject fixtures in `internal/schema` and the validator tests |
| [`ENG-TEST-010`](https://github.com/jrmoulckers/engineering/blob/v0.2.3/principles/assurance/testing.md#executable-procedures) | the documented no-residue rule is executed by CI, not asserted in prose |

### Verifying a change

Each signal reports independently and blocks the merge
([`ENG-TEST-004`](https://github.com/jrmoulckers/engineering/blob/v0.2.3/principles/assurance/testing.md#distinct-static-signals)):

```
test -z "$(gofmt -l .)"   # format
go vet ./...              # vet
golangci-lint run         # lint
go test ./...             # behavior
go build ./...            # build
```

The lint configuration is owned by
[jrmoulckers/engineering](https://github.com/jrmoulckers/engineering/blob/v0.2.3/configs/golangci.yml)
and is **not** copied into this repository. golangci-lint has no config
inheritance, so `scripts/fetch-engineering-config.sh` materializes it at a pinned
tag into a gitignored `.golangci.yml`:

```
scripts/fetch-engineering-config.sh          # defaults to v0.2.3
scripts/fetch-engineering-config.sh main     # or any ref
golangci-lint run
```

The engineering repository is public, so this needs no token. The script fails
loudly on a non-200, an empty body, or a payload that is not a golangci-lint
config — lint passing against a config that failed to download would be worse
than a red build.

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
