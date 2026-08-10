# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

The primary user is the local owner of a private game-artwork library. They use
the dashboard on desktop and Steam Deck browsers to browse platforms and games,
see every artwork role, understand presentation profiles, fill coverage gaps,
gather artwork into profiles for their other devices, and connect local sources
without learning repository-specific vocabulary.

## Product Purpose

`gamelib` organizes artwork and profile state across Steam, Playnite, Deck
Gaming Profiles, RetroDECK/ES-DE, RomM, and the canonical GamingProfiles
catalog. Success means the user opens a platform, recognizes games by their
artwork and titles, sees available and missing roles, and understands which
profiles and frontends use each asset.

## Positioning

The product is an artwork-first local organizer over deterministic Go contracts.
The dashboard shows games, artwork, and profiles; it never silently fuzzy-merges
identities, deletes by hash, promotes assets, or mutates a live frontend.
Deterministic verification, duplicate classification, and planning stay
available as `gamelib` CLI commands rather than shaping the interface.

## Operating Context

The dashboard runs from the `gamelib` binary on the user's machine, reads
host-local symbolic-root configuration and private inventory data, and works
with the separate private GamingProfiles tree. It produces validated local
profile drafts. Homelab deployment, Syncthing, CT601, Cartridge, and
device-local frontend publication remain outside this repository.

## Capabilities and Constraints

- Bind to loopback only; no LAN/public mode, cloud service, account
  authentication, telemetry, or persistent secret.
- Reuse the existing Go inventory, identity, policy, profile, manifest, media,
  Decky, report, and schema behavior rather than reproducing business rules in
  browser code.
- Atomically write only host-local configuration and explicit local profile
  drafts.
- Keep canonical catalog, bundle, generated Decky, Playnite database, live
  frontend, and homelab mutation unavailable.
- Preserve Windows and Linux behavior, symbolic/root-relative paths,
  case-collision handling, Unicode safety, deterministic output, and Decky v1
  compatibility including `deck-default`, `steam-default`, `artwork: null`, and
  `.deck-profile-empty`.
- Autodetect well-known local source locations without contacting a network,
  reading a live frontend database, or hardcoding a person's path.
- Aggregate files into games only from deterministic or explicit identity
  evidence. Ambiguous relationships stay separate and visible as needing
  attention.
- Resolve display titles only from local, read-only metadata: Steam caches and
  manifests, Playnite's safely readable game collection, and ES-DE gamelists.
  A metadata failure keeps a labeled identity placeholder and never fails the
  artwork library.
- Correlate Playnite with Steam only from the reviewed Steam plugin identity
  plus exact numeric storefront GameId. Never correlate by title similarity.
- Treat source availability as device-local. A supported source absent from the
  current machine is neutral information, not a broken-library state or a
  reason to probe another device.

## Evidence on Hand

The repository contains accepted architecture decisions, JSON Schemas, Go
domain packages, synthetic fixtures, cross-platform tests, and sanitized
aggregate reports. It contains no real artwork, private inventory, personal
paths, credentials, account identifiers, or live apply implementation; future
work must not fabricate or commit them.

## Product Principles

1. Lead with games and artwork, not implementation details.
2. Make the common path visual, direct, and near-zero configuration.
3. Treat ambiguity as a lightweight affordance, never an implicit merge.
4. Keep drafts visibly different from canonical or live state.
5. Preserve deterministic verification and recovery in the CLI without making
   them the product's front door.
6. Keep private data local and shared artifacts sanitized.
7. Stay fast on a real library. Artwork browsing and search must not degrade as
   the collection grows.

## Accessibility & Inclusion

Target WCAG 2.2 AA. Core workflows must work with keyboard, screen readers,
zoom/reflow, reduced motion, touch, and Steam Deck gamepad-style browser input.
Artwork, confidence, retention, and validation state must never rely on
color or visual recognition alone.
