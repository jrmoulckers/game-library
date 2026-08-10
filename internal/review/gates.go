package review

import (
	"fmt"
	"strings"
	"time"

	"github.com/jrmoulckers/game-library/internal/manifest"
	"github.com/jrmoulckers/game-library/internal/workspace"
)

// GateAReview is an immutable inventory/identity review record. It never
// authorizes anything by itself; it is evidence that a reviewer looked at
// a specific inventory/identity snapshot at a specific time.
type GateAReview struct {
	ID              string `json:"id"`
	CreatedAt       string `json:"createdAt"`
	InventoryDigest string `json:"inventoryDigest"`
	IdentityDigest  string `json:"identityDigest"`
	Reviewer        string `json:"reviewer,omitempty"`
	Notes           string `json:"notes,omitempty"`
}

// GateBReview is an immutable policy/profile/adapter review record. It
// references the Gate A review it follows, so a reviewer cannot record a
// Gate B review out of sequence.
type GateBReview struct {
	ID               string `json:"id"`
	CreatedAt        string `json:"createdAt"`
	GateAID          string `json:"gateAId"`
	PolicyDigest     string `json:"policyDigest"`
	ProfileDigest    string `json:"profileDigest,omitempty"`
	ExportPlanDigest string `json:"exportPlanDigest,omitempty"`
	Reviewer         string `json:"reviewer,omitempty"`
	Notes            string `json:"notes,omitempty"`
}

// GateCReview is an immutable, exact manifest/hash/space/backup/rollback
// review record — the final review gate before any hypothetical apply.
// Executable is always forced to false by CreateGateCReview regardless of
// what the caller supplies: this repository has no apply/publish/delete/
// prune/rollback capability, and Gate C is defined (ADR-0007) to end in a
// non-executable record, full stop.
type GateCReview struct {
	ID           string           `json:"id"`
	CreatedAt    string           `json:"createdAt"`
	GateBID      string           `json:"gateBId"`
	Analysis     ManifestAnalysis `json:"analysis"`
	RollbackPlan string           `json:"rollbackPlan,omitempty"`
	Reviewer     string           `json:"reviewer,omitempty"`
	Notes        string           `json:"notes,omitempty"`
	Executable   bool             `json:"executable"`
}

// CreateGateAReview stamps review.CreatedAt from clock, computes its
// immutable artifact ID from its content, and persists it as a
// create-if-absent workspace artifact under "gate-reviews/a/<id>.json".
// Two calls with byte-identical content (including CreatedAt — pass a
// fixed clock in tests to exercise this) are idempotent; recording another
// review of the same inputs at a different time is a distinct, additional
// historical record, not a conflict.
func CreateGateAReview(paths workspace.Paths, review GateAReview, clock func() time.Time) (GateAReview, bool, string, error) {
	if review.InventoryDigest == "" {
		return GateAReview{}, false, "", fmt.Errorf("gate A review requires an inventory digest")
	}
	review.CreatedAt = clock().UTC().Format(time.RFC3339)
	id, err := gateID(review)
	if err != nil {
		return GateAReview{}, false, "", err
	}
	review.ID = id
	created, path, err := workspace.WriteArtifact(paths, "gate-reviews/a/"+id+".json", review)
	return review, created, path, err
}

// CreateGateBReview requires a reference to the Gate A review it follows
// (gateAID, as returned by CreateGateAReview). It enforces Gate sequencing
// by more than a naive prefix check: the referenced Gate A artifact must
// actually exist on disk, decode as a GateAReview, carry the exact ID it is
// referenced by, and pass an immutability check (its content must still
// hash to that same ID) before a Gate B review can be recorded at all.
func CreateGateBReview(paths workspace.Paths, review GateBReview, clock func() time.Time) (GateBReview, bool, string, error) {
	if err := verifyPriorGate(paths, "a", review.GateAID, func() any { return &GateAReview{} }); err != nil {
		return GateBReview{}, false, "", fmt.Errorf("gate B review requires a valid, verifiable prior gate A review: %w", err)
	}
	if review.PolicyDigest == "" {
		return GateBReview{}, false, "", fmt.Errorf("gate B review requires a policy digest")
	}
	review.CreatedAt = clock().UTC().Format(time.RFC3339)
	id, err := gateID(review)
	if err != nil {
		return GateBReview{}, false, "", err
	}
	review.ID = id
	created, path, err := workspace.WriteArtifact(paths, "gate-reviews/b/"+id+".json", review)
	return review, created, path, err
}

// CreateGateCReview requires a reference to the Gate B review it follows
// (gateBID), verified the same way CreateGateBReview verifies its Gate A
// reference (existence, decode, ID match, and immutability), forces
// Executable to false unconditionally, and additionally requires that
// review.Analysis.ManifestDigest matches a plan that was actually persisted
// through this workspace's own plan-building endpoints (see
// PersistedPlanDigestExists) — a Gate C review can never be recorded
// against an arbitrary hypothetical manifest that was never actually
// planned.
func CreateGateCReview(paths workspace.Paths, review GateCReview, clock func() time.Time) (GateCReview, bool, string, error) {
	if err := verifyPriorGate(paths, "b", review.GateBID, func() any { return &GateBReview{} }); err != nil {
		return GateCReview{}, false, "", fmt.Errorf("gate C review requires a valid, verifiable prior gate B review: %w", err)
	}
	if review.Analysis.ManifestDigest == "" {
		return GateCReview{}, false, "", fmt.Errorf("gate C review requires a manifest analysis")
	}
	tied, err := PersistedPlanDigestExists(paths, review.Analysis.ManifestDigest)
	if err != nil {
		return GateCReview{}, false, "", fmt.Errorf("verify gate C manifest digest: %w", err)
	}
	if !tied {
		return GateCReview{}, false, "", fmt.Errorf("gate C review requires an analysis tied to a manifest previously persisted by this workspace")
	}
	review.Executable = false
	review.CreatedAt = clock().UTC().Format(time.RFC3339)
	id, err := gateID(review)
	if err != nil {
		return GateCReview{}, false, "", err
	}
	review.ID = id
	created, path, err := workspace.WriteArtifact(paths, "gate-reviews/c/"+id+".json", review)
	return review, created, path, err
}

// verifyPriorGate loads, decodes, and integrity-checks the prior-gate
// artifact of the given letter ("a" or "b") referenced by id. It returns an
// error — describing only the symbolic id, never a filesystem path — when
// the reference:
//
//   - is not even shaped like a gate-<letter>-... id (a cheap check that
//     avoids a filesystem read for an obviously forged reference);
//   - does not exist as a persisted artifact;
//   - fails to decode as valid JSON for the expected review type;
//   - decodes to a stored ID different from the id it was referenced by
//     (a forged or mismatched reference); or
//   - decodes to content whose own recomputed content-derived ID no longer
//     equals the referenced id (evidence the artifact was tampered with
//     after being written, defeating the create-if-absent immutability
//     guarantee WriteArtifact is supposed to provide).
func verifyPriorGate(paths workspace.Paths, letter, id string, newValue func() any) error {
	prefix := "gate-" + letter + "-"
	if id == "" || !strings.HasPrefix(id, prefix) {
		return fmt.Errorf("%q is not a valid gate-%s review id", id, letter)
	}
	value := newValue()
	found, err := workspace.ReadArtifact(paths, "gate-reviews/"+letter+"/"+id+".json", value)
	if err != nil {
		return fmt.Errorf("read referenced gate-%s review %q: %w", letter, id, err)
	}
	if !found {
		return fmt.Errorf("referenced gate-%s review %q does not exist", letter, id)
	}
	storedID, err := gateReviewID(value)
	if err != nil {
		return fmt.Errorf("inspect referenced gate-%s review %q: %w", letter, id, err)
	}
	if storedID != id {
		return fmt.Errorf("referenced gate-%s review %q does not match its stored id %q", letter, id, storedID)
	}
	expectedID, err := recomputeGateReviewID(value)
	if err != nil {
		return fmt.Errorf("verify referenced gate-%s review %q integrity: %w", letter, id, err)
	}
	if expectedID != id {
		return fmt.Errorf("referenced gate-%s review %q failed an immutability check", letter, id)
	}
	return nil
}

// gateReviewID extracts the ID field from a *GateAReview/*GateBReview
// pointer produced by newValue in verifyPriorGate.
func gateReviewID(value any) (string, error) {
	switch v := value.(type) {
	case *GateAReview:
		return v.ID, nil
	case *GateBReview:
		return v.ID, nil
	default:
		return "", fmt.Errorf("unsupported gate review type %T", value)
	}
}

// recomputeGateReviewID recomputes gateID for value with its ID field
// cleared, exactly as CreateGateAReview/CreateGateBReview did when the
// artifact was first written, so verifyPriorGate can confirm the stored
// content still hashes to its own claimed id.
func recomputeGateReviewID(value any) (string, error) {
	switch v := value.(type) {
	case *GateAReview:
		clean := *v
		clean.ID = ""
		return gateID(clean)
	case *GateBReview:
		clean := *v
		clean.ID = ""
		return gateID(clean)
	default:
		return "", fmt.Errorf("unsupported gate review type %T", value)
	}
}

// gateID derives a deterministic artifact id from value's content (with ID
// left at its zero value, since the id is derived from everything else).
// The returned id is prefixed so CreateGateBReview/CreateGateCReview can
// cheaply validate that a caller-supplied prior-gate reference at least has
// the right shape without reading it back from disk.
func gateID(value any) (string, error) {
	digest, err := manifest.Digest(value)
	if err != nil {
		return "", fmt.Errorf("digest gate review: %w", err)
	}
	prefix := "gate-a"
	switch value.(type) {
	case GateBReview:
		prefix = "gate-b"
	case GateCReview:
		prefix = "gate-c"
	}
	return prefix + "-" + digest[:32], nil
}
