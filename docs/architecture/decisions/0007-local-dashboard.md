# ADR-0007: Local dashboard is a plan-only Go web surface

## Status

Accepted

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
