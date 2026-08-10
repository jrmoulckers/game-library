# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

The primary user is the local owner of a private game-artwork library. They use
the dashboard on desktop and Steam Deck browsers to understand source
inventories, resolve identities, author retention and profile drafts, and review
safe migration/export plans without editing JSON by hand.

## Product Purpose

`gamelib` makes artwork and profile state across Steam, Playnite, Deck Gaming
Profiles, RetroDECK/ES-DE, RomM, and the canonical GamingProfiles catalog
understandable and reviewable. Success means the user can trace every proposed
identity, retention outcome, profile closure, duplicate classification, and plan
action before any live system could change.

## Positioning

The product is a recovery-first local workbench over deterministic Go contracts:
it explains and records plans, but never silently fuzzy-merges identities,
deletes by hash, promotes assets, or mutates a live frontend.

## Operating Context

The dashboard runs from the `gamelib` binary on the user's machine, reads
host-local symbolic-root configuration and private inventory data, and works
with the separate private GamingProfiles tree. It produces validated local
drafts and immutable plan/review documents. Homelab deployment, Syncthing,
CT601, Cartridge, and device-local frontend publication remain outside this
repository.

## Capabilities and Constraints

- Bind to loopback only; no LAN/public mode, cloud service, account
  authentication, telemetry, or persistent secret.
- Reuse the existing Go inventory, identity, policy, profile, manifest, media,
  Decky, report, and schema behavior rather than reproducing business rules in
  browser code.
- Atomically write only host-local configuration and explicit local policy or
  profile drafts. Create immutable plan and gate-review records.
- Keep canonical catalog, bundle, generated Decky, Playnite database, live
  frontend, and homelab mutation unavailable.
- Preserve Windows and Linux behavior, symbolic/root-relative paths,
  case-collision handling, Unicode safety, deterministic output, and Decky v1
  compatibility including `deck-default`, `steam-default`, `artwork: null`, and
  `.deck-profile-empty`.

## Evidence on Hand

The repository contains accepted architecture decisions, JSON Schemas, Go
domain packages, synthetic fixtures, cross-platform tests, and sanitized
aggregate reports. It contains no real artwork, private inventory, personal
paths, credentials, account identifiers, or live apply implementation; future
work must not fabricate or commit them.

## Product Principles

1. Explain before acting.
2. Treat ambiguity as a review queue, never an implicit merge.
3. Make drafts and plans visibly different from canonical or live state.
4. Preserve enough evidence for deterministic verification and recovery.
5. Keep private data local and shared artifacts sanitized.

## Accessibility & Inclusion

Target WCAG 2.2 AA. Core workflows must work with keyboard, screen readers,
zoom/reflow, reduced motion, touch, and Steam Deck gamepad-style browser input.
Artwork, confidence, retention, validation, and gate state must never rely on
color or visual recognition alone.
