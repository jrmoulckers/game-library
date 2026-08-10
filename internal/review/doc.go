// Package review implements the read-only inventory/identity/policy/profile
// review domain behind the local dashboard (ADR-0007). Every function here
// is a pure or read-only view over data already produced by the existing
// inventory, identity, policy, profile, manifest, media, and decky
// packages: this package never introduces a second implementation of a
// business rule, and it never writes to the canonical GamingProfiles tree,
// bundles, generated Decky output, the Playnite database, or any live
// frontend. The only writes this package performs are immutable,
// create-if-absent host-local artifacts via internal/workspace (persisted
// plans and Gate A/B/C review records) — never an apply, publish, delete,
// prune, or rollback of anything outside the local workspace.
package review
