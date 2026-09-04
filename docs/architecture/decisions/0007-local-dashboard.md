# ADR-0007: Local dashboard is a plan-only Go web surface

- Status: Accepted
- Date: 2026-08-09
- Owner: jrmoulckers

The dashboard-shape decisions below (the Advanced disclosure, Gate
A/B/C, and the review/policy/planning/adapter/recovery surfaces) are superseded
by [ADR-0008](0008-organizer-only-dashboard.md). Everything else — loopback-only
binding, request hardening, thin handlers over Go contracts, source detection,
title resolution, and the Playnite-to-Steam join — remains in force.

## Context

The existing `gamelib` CLI exposes deterministic inventory, identity, policy,
profile, manifest, adapter, and validation operations, but using them requires
remembering commands and reading or editing JSON. A browser dashboard can make
the same contracts easier to understand, provided it does not create a second
business-rule implementation or weaken the repository's read-only and recovery
defaults.

A loopback listener is still reachable by browsers and local processes. It
therefore needs explicit protection against accidental network exposure, DNS
rebinding, cross-origin requests, path traversal, unsafe media rendering, and
concurrent writes.

## Decision

- Add `gamelib serve` using Go `net/http`, `html/template`, embedded static
  assets, and small progressive-enhancement JavaScript modules. Distribution
  remains one binary with no frontend runtime or cloud dependency.
- Accept only explicit loopback IP listeners. Wildcard, hostname, LAN, and
  public listeners are rejected.
- Validate the request `Host`; require same-origin `Origin`, local
  `Sec-Fetch-Site`, and an in-memory per-process CSRF nonce for unsafe methods;
  apply bounded bodies, server timeouts, security headers, sanitized errors,
  and path/media containment checks.
- Do not add account authentication, telemetry, or persistent bearer secrets.
  The process trusts the local OS user and warns that other processes running as
  that user share the same local trust boundary.
- Keep browser handlers thin. Inventory, identity, policy, profile, manifest,
  media, and Decky decisions remain in their existing Go packages or in new
  Go-only review helpers.
- Allow atomic writes to the active host-local config. Save policy and profile
  changes only as validated local drafts with base digests. Create plans and
  gate reviews with immutable create-if-absent semantics.
- Define Gate A as inventory/identity review, Gate B as policy/profile/adapter
  review, and Gate C as exact manifest/hash/space/backup/rollback review. Gate C
  ends in a non-executable record. No HTTP or CLI apply/publish/delete endpoint
  is added.
- Permit profile drafts to select a safe theme ID. Standalone theme authoring
  waits for a separately accepted theme contract.
- Present the primary dashboard as an organizer: Library, Platform, Game,
  Profiles, and Sources. Move the existing review, policy, planning, adapter,
  and recovery surfaces into a single Advanced disclosure without removing
  their endpoints or behavior.
- Build organizer views in a Go-only read-model package over the existing
  snapshot, identity, media, and profile contracts. Browser modules only render
  those views and submit existing validated draft/config shapes.
- Autodetect conventional Windows and Linux source locations using local
  filesystem probes. Detection is read-only, makes no network request, and
  requires explicit confirmation before host-local configuration changes.
- Allow an organizer scan to process roots incrementally and publish partial
  in-memory snapshots. The legacy review refresh remains synchronous and
  compatible; neither path persists private inventory.
- Resolve organizer titles through a process-memory, read-only metadata cache.
  Steam appinfo/manifests/shortcuts, Playnite LiteDB v4 data, and ES-DE
  gamelists are optional providers; malformed, busy, unsupported, or changing
  metadata degrades to placeholders without failing inventory.
- Permit a Playnite-to-Steam entity join only when a reviewed Playnite Steam
  plugin ID and numeric storefront GameId identify the exact Steam AppID.
  Titles never create identity edges.
- Model supported source absence as local-device state. The dashboard does not
  ping, SSH, mount, or otherwise inspect another device or remote service.

## Consequences

- The dashboard is easy to distribute and cannot drift from a separately
  deployed frontend service.
- Complex behavior is tested once in Go and shared by CLI and dashboard.
- Loopback operation is meaningfully hardened without adding a login or secret
  lifecycle.
- Policy/profile edits require an explicit future promotion workflow outside
  this release; this is intentionally less convenient than silently writing
  canonical data.
- Live frontend publication remains a homelab ownership seam, and Playnite
  database write-back remains prohibited.
- The common path now begins with recognizable platforms, games, and artwork;
  implementation-specific review language remains available to expert users
  without defining the product's first impression.

## Evidence

The hardening claims are the testable part, and each has a test:
[`listen_test.go`](../../../internal/dashboard/listen_test.go) rejects wildcard, hostname, LAN
and public listeners; [`hardening_test.go`](../../../internal/dashboard/hardening_test.go) and
[`security_test.go`](../../../internal/dashboard/security_test.go) cover `Host` validation,
same-origin `Origin`, `Sec-Fetch-Site`, the CSRF nonce, bounded bodies, security headers and
path/media containment; [`server_test.go`](../../../internal/dashboard/server_test.go) covers
handler wiring.

"Thin handlers" is verified structurally: business rules live in `internal/{inventory,identity,
policy,profile,manifest,media,review}` and are tested there, not through HTTP
([`ENG-ARCH-001`](https://github.com/jrmoulckers/engineering/blob/v0.116.0/principles/architecture/boundaries-and-contracts.md#minimal-directed-boundaries)).

Falsifiable by: binding a non-loopback address successfully, or any apply/publish/delete
endpoint appearing in the HTTP surface.
