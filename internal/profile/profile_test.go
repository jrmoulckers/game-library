package profile

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
)

func TestResolveAndPlans(t *testing.T) {
	content := []byte("asset")
	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])
	root := t.TempDir()
	contentPath := filepath.Join(root, "library", "assets", "sha256", hash[:2], hash, "content.png")
	if err := os.MkdirAll(filepath.Dir(contentPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(contentPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	value := model.Profile{
		Version: 1, ID: "test", Name: "Test",
		Games: []model.ProfileGame{{
			ID: "steam:123",
			Identities: map[string]string{
				"steam":    "123",
				"playnite": "12345678-1234-1234-9234-1234567890ab",
			},
			Assets: map[string]model.AssetSelection{
				"grid": {SHA256: hash, Extension: "png"},
			},
		}},
	}
	resolution, err := Resolve(value, root)
	if err != nil {
		t.Fatal(err)
	}
	if !resolution.Complete {
		t.Fatalf("resolution incomplete: %+v", resolution.Issues)
	}
	bundle, _, err := BuildBundlePlan(value, root)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Actions[0].Action != "copy" {
		t.Fatalf("bundle action = %q", bundle.Actions[0].Action)
	}
	steam, err := BuildExportPlan("steam", value)
	if err != nil {
		t.Fatal(err)
	}
	if steam.Actions[0].DestinationPath != "123.png" {
		t.Fatalf("Steam destination = %q", steam.Actions[0].DestinationPath)
	}
	playnite, err := BuildExportPlan("playnite", value)
	if err == nil {
		t.Fatalf("expected unsupported Playnite grid error, got %+v", playnite)
	}
}

func TestMissingAssetBlocksBundle(t *testing.T) {
	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	value := model.Profile{
		Version: 1, ID: "test", Name: "Test",
		Games: []model.ProfileGame{{
			ID: "steam:123",
			Assets: map[string]model.AssetSelection{
				"grid": {SHA256: hash, Extension: "png"},
			},
		}},
	}
	plan, resolution, err := BuildBundlePlan(value, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Complete || plan.Actions[0].Action != "blocked" {
		t.Fatalf("expected blocked bundle: %+v %+v", resolution, plan)
	}
}

func TestRomMExportRejectsUnsafeIdentity(t *testing.T) {
	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	value := model.Profile{
		Version: 1, ID: "test", Name: "Test",
		Games: []model.ProfileGame{{
			ID:         "retro:n64:test",
			Identities: map[string]string{"romm": "../../../unsafe"},
			Assets: map[string]model.AssetSelection{
				"manual": {SHA256: hash, Extension: "pdf"},
			},
		}},
	}
	if _, err := BuildExportPlan("romm", value); err == nil {
		t.Fatal("expected unsafe RomM identity to produce no supported export")
	}
}

func TestValidateRejectsUnsafeRole(t *testing.T) {
	value := model.Profile{
		Version: 1, ID: "test", Name: "Test",
		Games: []model.ProfileGame{{
			ID: "steam:123",
			Assets: map[string]model.AssetSelection{
				"../escape": {
					SHA256:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					Extension: "png",
				},
			},
		}},
	}
	if err := Validate(value); err == nil {
		t.Fatal("expected unsafe role rejection")
	}
}

func TestPlayniteRejectsNonPNGLogo(t *testing.T) {
	value := model.Profile{
		Version: 1, ID: "test", Name: "Test",
		Games: []model.ProfileGame{{
			ID:         "steam:123",
			Identities: map[string]string{"playnite": "12345678-1234-1234-1234-1234567890ab"},
			Assets: map[string]model.AssetSelection{
				"logo": {
					SHA256:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					Extension: "jpg",
				},
			},
		}},
	}
	if _, err := BuildExportPlan("playnite", value); err == nil {
		t.Fatal("expected Playnite logo format rejection")
	}
}
