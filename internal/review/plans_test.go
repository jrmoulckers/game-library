package review

import (
	"errors"
	"strings"
	"testing"

	"github.com/jrmoulckers/game-library/internal/manifest"
	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/workspace"
)

func examplePrivateInventory() model.Inventory {
	return model.Inventory{
		Version: model.SchemaVersion, Privacy: "private",
		Observations: []model.Observation{{
			RootID: "source", RelativePath: "123.png", SHA256: strings.Repeat("a", 64),
			Size: 10, Media: model.MediaFacts{Extension: "png", Role: "grid"},
		}},
	}
}

func TestBuildAndPersistImportPlanIsIdempotent(t *testing.T) {
	paths := workspace.NewPaths(t.TempDir())
	inv := examplePrivateInventory()
	policyFile := model.PolicyFile{Version: model.SchemaVersion, Default: "managed"}

	plan, created, path, err := BuildAndPersistImportPlan(paths, inv, policyFile)
	if err != nil {
		t.Fatal(err)
	}
	if !created || path == "" {
		t.Fatalf("expected the first persist to create a new artifact, created=%v path=%q", created, path)
	}

	again, created2, _, err := BuildAndPersistImportPlan(paths, inv, policyFile)
	if err != nil {
		t.Fatal(err)
	}
	if created2 {
		t.Fatal("expected the second identical persist to be idempotent (created=false)")
	}
	if again.OperationID != plan.OperationID {
		t.Fatal("expected the same deterministic operation id")
	}
}

func TestBuildAndPersistImportPlanDiffersWithDifferentPolicy(t *testing.T) {
	paths := workspace.NewPaths(t.TempDir())
	inv := examplePrivateInventory()

	managed, _, _, err := BuildAndPersistImportPlan(paths, inv, model.PolicyFile{Version: model.SchemaVersion, Default: "managed"})
	if err != nil {
		t.Fatal(err)
	}
	quarantined, _, _, err := BuildAndPersistImportPlan(paths, inv, model.PolicyFile{Version: model.SchemaVersion, Default: "quarantined"})
	if err != nil {
		t.Fatal(err)
	}
	if managed.OperationID == quarantined.OperationID {
		t.Fatal("expected different policy outcomes to produce different operation ids")
	}
}

func TestPersistPlanConflictsOnSameNameDifferentContent(t *testing.T) {
	paths := workspace.NewPaths(t.TempDir())
	actions := []model.Action{{Action: "skip", Reason: "r"}}

	first, err := manifest.NewPlan("import-plan", actions, "warning-a")
	if err != nil {
		t.Fatal(err)
	}
	second, err := manifest.NewPlan("import-plan", actions, "warning-b")
	if err != nil {
		t.Fatal(err)
	}
	if first.OperationID != second.OperationID {
		t.Fatal("test setup expects identical operation ids (same actions) with differing warnings")
	}

	if _, _, err := PersistPlan(paths, first); err != nil {
		t.Fatal(err)
	}
	if _, _, err := PersistPlan(paths, second); !errors.Is(err, workspace.ErrConflict) {
		t.Fatalf("expected ErrConflict for same name/different content, got %v", err)
	}
}

func TestBuildAndPersistBundlePlan(t *testing.T) {
	paths := workspace.NewPaths(t.TempDir())
	catalog := t.TempDir()
	plan, resolution, created, path, err := BuildAndPersistBundlePlan(paths, steamProfile(), catalog)
	if err != nil {
		t.Fatal(err)
	}
	if !created || path == "" {
		t.Fatal("expected a newly created artifact")
	}
	if plan.Kind != "bundle-plan" {
		t.Fatalf("plan kind = %q", plan.Kind)
	}
	if resolution.Complete {
		t.Fatal("expected incomplete resolution: catalog has no assets")
	}
}

func TestBuildAndPersistExportPlan(t *testing.T) {
	paths := workspace.NewPaths(t.TempDir())
	plan, created, path, err := BuildAndPersistExportPlan(paths, steamProfile(), "steam")
	if err != nil {
		t.Fatal(err)
	}
	if !created || path == "" {
		t.Fatal("expected a newly created artifact")
	}
	if plan.Kind != "steam-export-plan" {
		t.Fatalf("plan kind = %q", plan.Kind)
	}
}
