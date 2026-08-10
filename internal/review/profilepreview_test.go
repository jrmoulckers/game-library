package review

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
)

func steamProfile() model.Profile {
	return model.Profile{
		Version: model.SchemaVersion, ID: "example", Name: "Example",
		Games: []model.ProfileGame{{
			ID:         "steam:123",
			Identities: map[string]string{"steam": "123"},
			Assets: map[string]model.AssetSelection{
				"grid": {SHA256: strings.Repeat("a", 64), Extension: "png"},
			},
		}},
	}
}

func retroOnlyProfile() model.Profile {
	return model.Profile{
		Version: model.SchemaVersion, ID: "retro-only", Name: "Retro only",
		Games: []model.ProfileGame{{
			ID:    "retro:n64:example",
			Retro: &model.RetroTarget{System: "n64", Stem: "Example"},
			Assets: map[string]model.AssetSelection{
				"cover": {SHA256: strings.Repeat("b", 64), Extension: "jpg"},
			},
		}},
	}
}

func TestPreviewExportPlanForDeckyWithGridArtwork(t *testing.T) {
	preview, err := PreviewExportPlan(steamProfile(), "decky")
	if err != nil {
		t.Fatal(err)
	}
	if !preview.HasGridArtwork {
		t.Fatal("expected HasGridArtwork=true for a profile with a steam grid asset")
	}
	if preview.DeckyProfile == nil {
		t.Fatal("expected a synthesized Decky profile for the decky adapter")
	}
	if preview.DeckyProfile.Artwork == nil || *preview.DeckyProfile.Artwork != "example" {
		t.Fatalf("expected artwork id 'example', got %+v", preview.DeckyProfile.Artwork)
	}
	if preview.DeckyProfile.Version != 1 {
		t.Fatalf("expected Decky v1 schema version, got %d", preview.DeckyProfile.Version)
	}
}

func TestPreviewExportPlanForDeckyWithoutArtworkStillGetsEmptyMarkerAndValidates(t *testing.T) {
	preview, err := PreviewExportPlan(retroOnlyProfile(), "decky")
	if err != nil {
		t.Fatal(err)
	}
	if preview.HasGridArtwork {
		t.Fatal("expected HasGridArtwork=false for a retro-only profile with no steam grid asset")
	}
	if preview.DeckyProfile == nil || preview.DeckyProfile.Artwork == nil {
		t.Fatal("expected a non-nil artwork id even for the .deck-profile-empty fallback")
	}
	foundMarker := false
	for _, action := range preview.Plan.Actions {
		if strings.HasSuffix(action.DestinationPath, ".deck-profile-empty") {
			foundMarker = true
		}
	}
	if !foundMarker {
		t.Fatal("expected the export plan to include the .deck-profile-empty marker action")
	}
}

func TestPreviewExportPlanMarshalsExplicitEmptyMods(t *testing.T) {
	preview, err := PreviewExportPlan(steamProfile(), "decky")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(preview.DeckyProfile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"mods":[]`) {
		t.Fatalf("expected explicit empty mods array, got %s", encoded)
	}
}

func TestPreviewExportPlanForNonDeckyAdapterHasNoDeckyProfile(t *testing.T) {
	preview, err := PreviewExportPlan(steamProfile(), "steam")
	if err != nil {
		t.Fatal(err)
	}
	if preview.DeckyProfile != nil {
		t.Fatal("expected no Decky profile for the steam adapter")
	}
}

func TestPreviewProfileResolvePassesThroughToProfilePackage(t *testing.T) {
	root := t.TempDir()
	resolution, err := PreviewProfileResolve(steamProfile(), root)
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Complete {
		t.Fatal("expected the resolution to be incomplete when the catalog root has no assets")
	}
}
