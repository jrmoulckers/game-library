# Canonical Tree Layout

This is the full layout of the private, Syncthing-synced `GamingProfiles` folder
(ADR-0001). It is **not** part of this Git repository — nothing under this layout
is checked into `jrmoulckers/game-library`. The schemas in [`../../schemas/v1/`](../../schemas/v1/)
are the contract every file below must satisfy.

Paths below use illustrative placeholders (`<appid>`, `<hash>`, `<uuid>`, ...) —
none of them are real filenames, accounts, or paths from any actual device.

```
GamingProfiles/                              # Syncthing folder root (not a Git repo)
├── catalog.json                             # root manifest — catalog.schema.json
│
├── library/                                 # canonical source of truth
│   ├── games/
│   │   ├── pc/
│   │   │   ├── steam/<appid>/game.json           # game.schema.json (platform_class: pc, store: steam)
│   │   │   └── <store>/<storefront_id>/game.json # game.schema.json (platform_class: pc, store: <store>)
│   │   └── retro/<system>/<slug>/game.json       # game.schema.json (platform_class: retro)
│   │
│   ├── releases/
│   │   └── retro/<system>/<sha256>/release.json  # retro-release.schema.json — exact-byte identity
│   │
│   ├── mappings/
│   │   └── playnite/<uuid>.json                  # playnite-mapping.schema.json — adapter-only, never identity
│   │
│   ├── assets/
│   │   └── sha256/<aa>/<hash>/
│   │       ├── asset.json                        # asset.schema.json
│   │       └── content.<ext>                     # exact bytes; sha256(content.<ext>) == <hash>
│   │
│   ├── sources/
│   │   └── <source_id>.json                      # source.schema.json
│   │
│   ├── policies/
│   │   └── <policy_id>.json                      # policy.schema.json — one scope level each
│   │
│   └── canonical/
│       ├── profiles/<id>.json                    # canonical-profile.schema.json (source of truth)
│       ├── themes/<id>.json                      # canonical theme definitions
│       └── mods/<id>.json                        # canonical mod/mod-set definitions
│
├── bundles/                                  # GENERATED, immutable, never source (ADR-0005)
│   └── <bundle_id>/
│       ├── <revision>/
│       │   ├── manifest.lock.json                # bundle-lock.schema.json
│       │   └── ...materialized files...
│       └── current.json                          # bundle-current.schema.json — live pointer + rollback target
│
├── state/                                    # operational state — inventory, staging, safety net
│   ├── inventory/
│   │   ├── reports/<report_id>.json              # inventory-report.schema.json
│   │   └── observations/<observation_id>.json     # inventory-observation.schema.json
│   ├── inbox/                                # unreconciled incoming files, staged for matching
│   ├── quarantine/                           # items policy has flagged, retained but not managed
│   ├── archive/                              # superseded-but-retained data (no auto-purge)
│   └── migration/
│       └── <migration_id>/manifest.json          # migration-manifest.schema.json
│
├── profiles/<id>.json                        # GENERATED — Decky v1 legacy ABI (decky-profile-v1.schema.json)
├── artwork/                                  # GENERATED — legacy Decky artwork root
└── mods/                                     # GENERATED — legacy Decky mods root
```

## Notes on each top-level section

- **`catalog.json`** is a lightweight, generated index (counts, root pointers,
  optional aggregate integrity digest). It never embeds full game/asset/profile
  bodies — those are always read from their own directory-based files. This keeps
  it cheap to sync and diff even as the catalog grows.
- **`library/**`** is the only source of truth. Everything else in the tree
  (`bundles/`, the legacy `profiles/`/`artwork/`/`mods/` roots) is derived from it
  and can always be regenerated.
- **`library/games/pc/steam/<appid>`** vs **`library/games/pc/<store>/<storefront_id>`**:
  the directory path itself encodes `store`; Steam gets its own first-class segment
  because AppID is the most common and most stable PC identity (ADR-0004).
- **`library/releases/retro/<system>/<sha256>`** is keyed by the *release's* full
  raw SHA-256, distinct from `library/games/retro/<system>/<slug>`, which is the
  *logical* game. One logical game can have many releases (regions, revisions).
- **`library/mappings/playnite/`** exists so canonical `game.json` files never need
  to know Playnite exists (ADR-0004).
- **`library/assets/sha256/<aa>/<hash>/`** — `<aa>` is the first two hex characters
  of `<hash>`, used purely as a fan-out directory to keep any single directory
  listing small; it carries no semantic meaning of its own.
- **`bundles/`** and the legacy **`profiles/`/`artwork/`/`mods/`** roots are the
  only generated, mutable-at-the-pointer-level parts of the tree; see ADR-0003 and
  ADR-0005 for their generation and rollback rules.
- **`state/`** is where anything uncertain, unreconciled, or in-flight lives until
  it is either promoted into `library/**` (via policy, see
  [`identity-and-policy.md`](identity-and-policy.md)) or explicitly archived —
  never silently deleted (see [`migration-and-recovery.md`](migration-and-recovery.md)).

## Device-local, out of scope for this tree

Live frontend directories (whatever a given Steam Deck, PC, or emulation frontend
actually reads from at runtime) are **device-local**, not part of this synced
tree. See [`adapters.md`](adapters.md) for how generated bundles get from this tree
onto a specific device.
