package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
)

func TestBuildImportIsDeterministic(t *testing.T) {
	inventory := model.Inventory{
		Version: 1, Privacy: "private",
		Observations: []model.Observation{{
			RootID: "source", RelativePath: "123.png", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Size: 10, Media: model.MediaFacts{Extension: "png", Role: "grid"},
		}},
	}
	policy := model.PolicyFile{
		Version: 1, Default: "managed",
	}
	first, err := BuildImport(inventory, policy)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildImport(inventory, policy)
	if err != nil {
		t.Fatal(err)
	}
	if first.OperationID != second.OperationID || first.SourceDigest != second.SourceDigest {
		t.Fatalf("plans are not deterministic: %+v %+v", first, second)
	}
	if first.Actions[0].Action != "copy" {
		t.Fatalf("action = %q", first.Actions[0].Action)
	}
}

func TestBuildImportRejectsUnsafeHash(t *testing.T) {
	inventory := model.Inventory{
		Version: 1, Privacy: "private",
		Observations: []model.Observation{{
			RootID: "source", RelativePath: "asset.png",
			SHA256: "../../unsafe.....................................................",
			Media:  model.MediaFacts{Extension: "png"},
		}},
	}
	_, err := BuildImport(inventory, model.PolicyFile{Version: 1, Default: "managed"})
	if err == nil {
		t.Fatal("expected unsafe SHA-256 rejection")
	}
}

func TestBuildImportRejectsUnsafeSourcePath(t *testing.T) {
	inventory := model.Inventory{
		Version: 1, Privacy: "private",
		Observations: []model.Observation{{
			RootID: "source", RelativePath: "../asset.png",
			SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Media:  model.MediaFacts{Extension: "png"},
		}},
	}
	_, err := BuildImport(inventory, model.PolicyFile{Version: 1, Default: "managed"})
	if err == nil {
		t.Fatal("expected unsafe source path rejection")
	}
}

func TestVerifyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyFile(path, "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"); err != nil {
		t.Fatal(err)
	}
	if err := VerifyFile(path, "bad"); err == nil {
		t.Fatal("expected digest mismatch")
	}
}
