package review

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jrmoulckers/game-library/internal/manifest"
	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/workspace"
)

func TestListHistoryIsEmptyForAFreshWorkspace(t *testing.T) {
	paths := workspace.NewPaths(t.TempDir())
	entries, err := ListHistory(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no history yet, got %+v", entries)
	}
}

func TestListHistoryIncludesPersistedPlansAndGates(t *testing.T) {
	paths := workspace.NewPaths(t.TempDir())
	plan, err := manifest.NewPlan("import-plan", []model.Action{{Action: "skip", Reason: "r"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := PersistPlan(paths, plan); err != nil {
		t.Fatal(err)
	}
	gateA, _, _, err := CreateGateAReview(paths, GateAReview{InventoryDigest: "inv"}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	gateB, _, _, err := CreateGateBReview(paths, GateBReview{GateAID: gateA.ID, PolicyDigest: "p"}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := manifest.Digest(plan)
	if err != nil {
		t.Fatal(err)
	}
	gateC, _, _, err := CreateGateCReview(paths, GateCReview{GateBID: gateB.ID, Analysis: ManifestAnalysis{ManifestDigest: digest}}, time.Now)
	if err != nil {
		t.Fatal(err)
	}

	entries, err := ListHistory(paths)
	if err != nil {
		t.Fatal(err)
	}
	byType := make(map[string][]HistoryEntry)
	for _, e := range entries {
		byType[e.Type] = append(byType[e.Type], e)
		if e.Digest == "" {
			t.Fatalf("expected a non-empty digest for %+v", e)
		}
		if !e.Verified {
			t.Fatalf("expected every legitimately-created artifact to verify: %+v", e)
		}
	}
	if len(byType["plan"]) != 1 || byType["plan"][0].ID != plan.OperationID || byType["plan"][0].Kind != "import-plan" {
		t.Fatalf("unexpected plan history: %+v", byType["plan"])
	}
	if len(byType["gate-a"]) != 1 || byType["gate-a"][0].ID != gateA.ID {
		t.Fatalf("unexpected gate-a history: %+v", byType["gate-a"])
	}
	if len(byType["gate-b"]) != 1 || byType["gate-b"][0].ID != gateB.ID {
		t.Fatalf("unexpected gate-b history: %+v", byType["gate-b"])
	}
	if len(byType["gate-c"]) != 1 || byType["gate-c"][0].ID != gateC.ID {
		t.Fatalf("unexpected gate-c history: %+v", byType["gate-c"])
	}
}

// TestListHistoryNeverListsForwardLookingAppliedOrRollbackFiles covers
// issue #5's requirement that this package must never treat a forward-
// looking applied/rollback record as real history, even defensively, if
// one somehow appeared under the local artifacts directory.
func TestListHistoryNeverListsForwardLookingAppliedOrRollbackFiles(t *testing.T) {
	paths := workspace.NewPaths(t.TempDir())
	dir := filepath.Join(paths.Artifacts, "plans", "import-plan")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "applied.json"), []byte(`{"executed":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rolled-back.json"), []byte(`{"rolledBack":true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := ListHistory(paths)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.ID == "applied" || e.ID == "rolled-back" {
			t.Fatalf("must never surface a forward-looking applied/rollback file as history: %+v", e)
		}
	}
	if len(entries) != 0 {
		t.Fatalf("expected no legitimate history entries, got %+v", entries)
	}
}

// TestListHistoryDetectsTamperedArtifacts ensures a directly-modified
// on-disk artifact (bypassing CreateGateAReview/PersistPlan entirely, as an
// attacker with local filesystem access might) is reported as unverified
// rather than silently accepted.
func TestListHistoryDetectsTamperedArtifacts(t *testing.T) {
	paths := workspace.NewPaths(t.TempDir())
	gateA, _, path, err := CreateGateAReview(paths, GateAReview{InventoryDigest: "inv"}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	tampered := gateA
	tampered.InventoryDigest = "tampered-after-the-fact"
	if err := os.WriteFile(path, mustMarshal(t, tampered), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := ListHistory(paths)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if e.Type == "gate-a" && e.ID == gateA.ID {
			found = true
			if e.Verified {
				t.Fatal("expected a tampered artifact to be reported as unverified")
			}
		}
	}
	if !found {
		t.Fatal("expected the tampered gate-a artifact to still be listed")
	}
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(data, '\n')
}
