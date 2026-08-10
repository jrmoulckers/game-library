package review

import (
	"strings"
	"testing"
	"time"

	"github.com/jrmoulckers/game-library/internal/manifest"
	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/workspace"
)

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func TestCreateGateAReviewRequiresInventoryDigest(t *testing.T) {
	paths := workspace.NewPaths(t.TempDir())
	_, _, _, err := CreateGateAReview(paths, GateAReview{}, time.Now)
	if err == nil {
		t.Fatal("expected an error when InventoryDigest is empty")
	}
}

func TestCreateGateAReviewIsIdempotentForIdenticalSubmission(t *testing.T) {
	paths := workspace.NewPaths(t.TempDir())
	clock := fixedClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	input := GateAReview{InventoryDigest: "inv-digest", IdentityDigest: "id-digest"}

	first, created, path, err := CreateGateAReview(paths, input, clock)
	if err != nil {
		t.Fatal(err)
	}
	if !created || path == "" {
		t.Fatal("expected the first submission to create a new artifact")
	}
	if !strings.HasPrefix(first.ID, "gate-a-") {
		t.Fatalf("expected a gate-a- prefixed id, got %q", first.ID)
	}

	second, created2, _, err := CreateGateAReview(paths, input, clock)
	if err != nil {
		t.Fatal(err)
	}
	if created2 {
		t.Fatal("expected the identical resubmission (same clock) to be idempotent")
	}
	if second.ID != first.ID {
		t.Fatal("expected the same id for identical content and timestamp")
	}
}

func TestCreateGateAReviewAtDifferentTimesProducesDistinctRecords(t *testing.T) {
	paths := workspace.NewPaths(t.TempDir())
	input := GateAReview{InventoryDigest: "inv-digest"}

	first, _, _, err := CreateGateAReview(paths, input, fixedClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}
	second, created, _, err := CreateGateAReview(paths, input, fixedClock(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected a review of the same inputs at a different time to be a new, distinct record")
	}
	if first.ID == second.ID {
		t.Fatal("expected different timestamps to produce different gate review ids")
	}
}

func TestGateSequencingRejectsGateBWithoutGateA(t *testing.T) {
	paths := workspace.NewPaths(t.TempDir())
	_, _, _, err := CreateGateBReview(paths, GateBReview{PolicyDigest: "p"}, time.Now)
	if err == nil {
		t.Fatal("expected an error when GateAID is missing")
	}
	_, _, _, err = CreateGateBReview(paths, GateBReview{GateAID: "not-a-real-id", PolicyDigest: "p"}, time.Now)
	if err == nil {
		t.Fatal("expected an error for a malformed GateAID")
	}
}

func TestGateSequencingAcceptsGateBReferencingARealGateA(t *testing.T) {
	paths := workspace.NewPaths(t.TempDir())
	gateA, _, _, err := CreateGateAReview(paths, GateAReview{InventoryDigest: "inv"}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	gateB, created, path, err := CreateGateBReview(paths, GateBReview{GateAID: gateA.ID, PolicyDigest: "policy-digest"}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if !created || path == "" {
		t.Fatal("expected a newly created gate B artifact")
	}
	if !strings.HasPrefix(gateB.ID, "gate-b-") {
		t.Fatalf("expected a gate-b- prefixed id, got %q", gateB.ID)
	}
}

func TestGateSequencingRejectsGateCWithoutGateB(t *testing.T) {
	paths := workspace.NewPaths(t.TempDir())
	analysis := ManifestAnalysis{ManifestDigest: "m"}
	_, _, _, err := CreateGateCReview(paths, GateCReview{Analysis: analysis}, time.Now)
	if err == nil {
		t.Fatal("expected an error when GateBID is missing")
	}
	_, _, _, err = CreateGateCReview(paths, GateCReview{GateBID: "not-a-real-id", Analysis: analysis}, time.Now)
	if err == nil {
		t.Fatal("expected an error for a malformed GateBID")
	}
}

func TestCreateGateCReviewForcesNonExecutable(t *testing.T) {
	paths := workspace.NewPaths(t.TempDir())
	gateA, _, _, err := CreateGateAReview(paths, GateAReview{InventoryDigest: "inv"}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	gateB, _, _, err := CreateGateBReview(paths, GateBReview{GateAID: gateA.ID, PolicyDigest: "p"}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	analysis := persistedAnalysisForTest(t, paths)

	// Even though the caller sets Executable: true, the persisted and
	// returned record must always force it back to false. Gate C never
	// authorizes execution, no matter what a caller supplies.
	input := GateCReview{
		GateBID:    gateB.ID,
		Analysis:   analysis,
		Executable: true,
	}
	gateC, created, path, err := CreateGateCReview(paths, input, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if !created || path == "" {
		t.Fatal("expected a newly created gate C artifact")
	}
	if gateC.Executable {
		t.Fatal("expected CreateGateCReview to force Executable=false regardless of input")
	}

	// And the persisted bytes on disk must also say executable:false —
	// not just the returned struct.
	data, readErr := readArtifactBytes(t, path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if strings.Contains(string(data), `"executable": true`) {
		t.Fatalf("persisted gate C artifact must never contain executable: true: %s", data)
	}
	if !strings.Contains(string(data), `"executable": false`) {
		t.Fatalf("persisted gate C artifact must explicitly record executable: false: %s", data)
	}
}

func TestCreateGateCReviewRequiresManifestAnalysis(t *testing.T) {
	paths := workspace.NewPaths(t.TempDir())
	gateA, _, _, err := CreateGateAReview(paths, GateAReview{InventoryDigest: "inv"}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	gateB, _, _, err := CreateGateBReview(paths, GateBReview{GateAID: gateA.ID, PolicyDigest: "p"}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, err = CreateGateCReview(paths, GateCReview{GateBID: gateB.ID}, time.Now)
	if err == nil {
		t.Fatal("expected an error when Analysis.ManifestDigest is empty")
	}
}

// persistedAnalysisForTest persists a minimal but real plan artifact and
// returns a ManifestAnalysis whose ManifestDigest ties to it, for tests
// that need a Gate C review to pass the persisted-manifest-digest check.
func persistedAnalysisForTest(t *testing.T, paths workspace.Paths) ManifestAnalysis {
	t.Helper()
	plan, err := manifestForTest()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := PersistPlan(paths, plan); err != nil {
		t.Fatal(err)
	}
	digest, err := manifest.Digest(plan)
	if err != nil {
		t.Fatal(err)
	}
	return ManifestAnalysis{ManifestDigest: digest}
}

func manifestForTest() (model.Manifest, error) {
	return manifest.NewPlan("import-plan", []model.Action{{Action: "skip", Reason: "r"}})
}

// TestCreateGateCReviewRejectsAnalysisNotTiedToAPersistedPlan covers issue
// #3: a Gate C review must reference an analysis whose ManifestDigest
// corresponds to a plan this workspace actually persisted, not an
// arbitrary hypothetical value the caller invented for the request.
func TestCreateGateCReviewRejectsAnalysisNotTiedToAPersistedPlan(t *testing.T) {
	paths := workspace.NewPaths(t.TempDir())
	gateA, _, _, err := CreateGateAReview(paths, GateAReview{InventoryDigest: "inv"}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	gateB, _, _, err := CreateGateBReview(paths, GateBReview{GateAID: gateA.ID, PolicyDigest: "p"}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, err = CreateGateCReview(paths, GateCReview{
		GateBID:  gateB.ID,
		Analysis: ManifestAnalysis{ManifestDigest: "not-a-real-persisted-manifest-digest"},
	}, time.Now)
	if err == nil {
		t.Fatal("expected an error when the analysis is not tied to a persisted plan")
	}
}

// TestGateSequencingRejectsForgedNonexistentAndWrongGateReferences covers
// issue #3's forged/nonexistent/wrong-gate-id requirement in depth: a
// syntactically valid but nonexistent id, a forged id (content that never
// actually hashes to the id it claims), and a real id from the wrong gate
// letter must all be rejected.
func TestGateSequencingRejectsForgedNonexistentAndWrongGateReferences(t *testing.T) {
	paths := workspace.NewPaths(t.TempDir())
	gateA, _, _, err := CreateGateAReview(paths, GateAReview{InventoryDigest: "inv"}, time.Now)
	if err != nil {
		t.Fatal(err)
	}

	// Nonexistent: shaped correctly, but never written.
	_, _, _, err = CreateGateBReview(paths, GateBReview{GateAID: "gate-a-" + strings.Repeat("0", 32), PolicyDigest: "p"}, time.Now)
	if err == nil {
		t.Fatal("expected an error for a nonexistent (but well-shaped) gate A reference")
	}

	// Forged: write an artifact directly under the expected gate-a path
	// whose stored ID does not actually match the content's own digest
	// (simulating post-write tampering, bypassing CreateGateAReview
	// entirely).
	forgedID := "gate-a-" + strings.Repeat("f", 32)
	forged := GateAReview{ID: forgedID, InventoryDigest: "tampered", CreatedAt: gateA.CreatedAt}
	if _, _, err := workspace.WriteArtifact(paths, "gate-reviews/a/"+forgedID+".json", forged); err != nil {
		t.Fatal(err)
	}
	_, _, _, err = CreateGateBReview(paths, GateBReview{GateAID: forgedID, PolicyDigest: "p"}, time.Now)
	if err == nil {
		t.Fatal("expected an error for a forged gate A reference whose content does not hash to its own id")
	}

	// Wrong gate: a real, valid Gate A id used where a Gate B id is
	// required.
	gateB, _, _, err := CreateGateBReview(paths, GateBReview{GateAID: gateA.ID, PolicyDigest: "p"}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	analysis := persistedAnalysisForTest(t, paths)
	_, _, _, err = CreateGateCReview(paths, GateCReview{GateBID: gateA.ID, Analysis: analysis}, time.Now)
	if err == nil {
		t.Fatal("expected an error when a gate A id is supplied where a gate B id is required")
	}

	// Sanity: the legitimate Gate B id still works.
	_, _, _, err = CreateGateCReview(paths, GateCReview{GateBID: gateB.ID, Analysis: analysis}, time.Now)
	if err != nil {
		t.Fatalf("expected the legitimate gate B reference to succeed, got %v", err)
	}
}

func TestGateArtifactsAreImmutable(t *testing.T) {
	paths := workspace.NewPaths(t.TempDir())
	clock := fixedClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	gateA, _, _, err := CreateGateAReview(paths, GateAReview{InventoryDigest: "inv", Notes: "first"}, clock)
	if err != nil {
		t.Fatal(err)
	}
	// A different logical review (different Notes) at the exact same
	// clock instant must never collide with or silently replace the
	// first: it gets its own id and its own file.
	other, created, _, err := CreateGateAReview(paths, GateAReview{InventoryDigest: "inv", Notes: "second"}, clock)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected a different logical review to be created as a new artifact")
	}
	if other.ID == gateA.ID {
		t.Fatal("expected different content to produce different gate review ids")
	}
}
