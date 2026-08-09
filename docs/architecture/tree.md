# Canonical Tree Layout

This is the full layout of the private, Syncthing-synced `GamingProfiles` folder
(ADR-0001). It is **not** part of this Git repository — nothing under this layout
is checked into `jrmoulckers/game-library`. The schemas in [`../../schemas/v1/`](../../schemas/v1/)
are the contract every file below must satisfy.

Paths below use illustrative placeholders (`<appid>`, `<hash>`, `<uuid>`, ...) —
none of them are real filenames, accounts, or paths from any actual device.

```
GamingProfiles/                              # Syncthing folder root (not a Git repo)
├── catalog.json                             # root manifest — catalog.schema.json [FORWARD-LOOKING, not yet implemented]
│
├── library/                                 # canonical source of truth
│   ├── games/                                    # [FORWARD-LOOKING, not yet implemented]
│   │   ├── pc/
│   │   │   ├── steam/<appid>/game.json           # game.schema.json (platform_class: pc, store: steam)
│   │   │   └── <store>/<storefront_id>/game.json # game.schema.json (platform_class: pc, store: <store>)
│   │   └── retro/<system>/<slug>/game.json       # game.schema.json (platform_class: retro)
│   │
│   ├── releases/                                 # [FORWARD-LOOKING, not yet implemented]
│   │   └── retro/<system>/<sha256>/release.json  # retro-release.schema.json — exact-byte identity
│   │
│   ├── mappings/                                 # [FORWARD-LOOKING, not yet implemented]
│   │   └── playnite/<uuid>.json                  # playnite-mapping.schema.json — adapter-only, never identity
│   │
│   ├── assets/                                   # [FORWARD-LOOKING content shape, path convention is implemented]
│   │   └── sha256/<aa>/<hash>/
│   │       ├── asset.json                        # asset.schema.json
│   │       └── content.<ext>                     # exact bytes; sha256(content.<ext>) == <hash>
│   │
│   ├── sources/                                  # [FORWARD-LOOKING, not yet implemented]
│   │   └── <source_id>.json                      # source.schema.json
│   │
│   ├── policy.json                               # policy.schema.json — single file: default + scoped rules[]
│   │
│   ├── profiles/
│   │   └── <id>/profile.json                     # canonical-profile.schema.json (source of truth; model.Profile)
│   ├── themes/
│   │   └── <id>/theme.json                       # canonical theme definition (id-scoped directory)
│   └── mods/
│       └── <game-id>/<mod-set>/
│           ├── mod.json                          # canonical mod-set metadata
│           └── payload/**                        # mod payload files
│
├── bundles/                                  # GENERATED, immutable, never source (ADR-0005)
│   └── <profile-id>/
│       ├── <manifest-sha256>/                    # revision = full SHA-256 of the profile's canonical JSON
│       │   ├── bundle.lock.json                  # bundle-lock.schema.json
│       │   └── assets/**                         # ...materialized asset files...
│       └── current.json                          # bundle-current.schema.json — live pointer + rollback target
│
├── state/                                    # operational state — inventory, staging, safety net
│   ├── inventory/                                # inventory-report.schema.json / inventory-observation.schema.json
│   │   └── <report_id>.json                      # observations[] embedded inline (private reports only)
│   ├── inbox/                                # unreconciled incoming files, staged for matching
│   ├── quarantine/                           # items policy has flagged, retained but not managed
│   ├── archive/                              # superseded-but-retained data (no auto-purge)
│   └── migration/
│       └── <operation_id>/
│           ├── planned.json                      # migration-manifest.schema.json — dry-run plan
│           ├── applied.json                      # [FORWARD-LOOKING] executed-action record, once apply exists
│           └── rolled-back.json                  # [FORWARD-LOOKING] rollback record, once apply exists
│
├── profiles/<id>.json                        # GENERATED — Decky v1 legacy ABI (decky-profile-v1.schema.json)
├── artwork/                                  # GENERATED — legacy Decky artwork root
└── mods/                                     # GENERATED — legacy Decky mods root
```

## Notes on each top-level section

- **`catalog.json`**, **`library/games/**`**, **`library/releases/**`**,
  **`library/mappings/**`**, **`library/sources/**`**, and `asset.json`'s exact
  content shape are **forward-looking**: no Go type in the current `gamelib`
  implementation reads or writes them yet. Today, game/release/mapping identity
  lives inline inside a profile's `games[]` entries — `id` (free-form, e.g.
  `"steam:123"`, `"retro:n64:example-game"`), an `identities{}` map (adapter name
  → native id, e.g. Playnite UUID), and an optional `retro{system,stem}` block —
  see [`identity-and-policy.md`](identity-and-policy.md) and
  `canonical-profile.schema.json`. These schemas remain the agreed target shape
  for a future standalone registry; they are kept, not deleted, but should not be
  read as describing anything the CLI produces today.
- **`library/policy.json`** is a **single** file (`model.PolicyFile`): one global
  `default` outcome plus an optional `rules[]` list of scoped overrides
  (`source`/`system`/`role`/`assetSha256` selectors, each with a `mode`). There is
  no one-file-per-scope-level layout. See
  [`identity-and-policy.md`](identity-and-policy.md) for precedence.
- **`library/profiles/<id>/profile.json`** is the implemented canonical profile
  (`model.Profile`, validated/resolved/exported by `internal/profile`): it
  carries per-game (`games[]`) asset selections keyed by role, not a single
  profile-level artwork override — that only exists in the generated legacy
  Decky ABI.
- **`library/themes/<id>/theme.json`** and **`library/mods/<game-id>/<mod-set>/`**
  are the canonical, hand-authored theme and mod-set definitions a profile can
  reference by id; `payload/**` holds the actual mod files applied to the game.
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
  listing small; it carries no semantic meaning of its own. This path convention
  *is* implemented (`internal/profile.canonicalAssetPath`,
  `internal/manifest.assetPath`).
- **`bundles/<profile-id>/<manifest-sha256>/`** — the revision segment is the full
  SHA-256 hex digest of the profile's canonical JSON encoding (`internal/profile`'s
  `revision()`), **not** an incrementing counter; a different profile edit produces
  an unrelated-looking hash, not the next integer. `bundle.lock.json` naming and
  the `assets/**` destination layout match `internal/profile.BuildBundlePlan`
  exactly, though that plan is dry-run only today (see
  [`migration-and-recovery.md`](migration-and-recovery.md)) — nothing yet writes
  `bundle.lock.json` or `current.json` to disk.
- **`bundles/`** and the legacy **`profiles/`/`artwork/`/`mods/`** roots are the
  only generated, mutable-at-the-pointer-level parts of the tree; see ADR-0003 and
  ADR-0005 for their generation and rollback rules.
- **`state/`** is where anything uncertain, unreconciled, or in-flight lives until
  it is either promoted into `library/**` (via policy, see
  [`identity-and-policy.md`](identity-and-policy.md)) or explicitly archived —
  never silently deleted (see [`migration-and-recovery.md`](migration-and-recovery.md)).
  `state/migration/<operation_id>/planned.json` is the only stage the current CLI
  actually produces (`gamelib import plan` / `bundle plan` / `export plan`, all
  dry-run); `applied.json`/`rolled-back.json` are forward-looking, reserved for
  once an apply/rollback command exists.

## Device-local, out of scope for this tree

Live frontend directories (whatever a given Steam Deck, PC, or emulation frontend
actually reads from at runtime) are **device-local**, not part of this synced
tree. See [`adapters.md`](adapters.md) for how generated bundles get from this tree
onto a specific device.
