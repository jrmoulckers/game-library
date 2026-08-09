package decky

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
)

func TestValidate(t *testing.T) {
	artwork := "deck-default"
	value := model.DeckyProfileV1{
		Version: 1, ID: "deck-default", Name: "Deck default", Artwork: &artwork,
		Mods: []model.DeckyModV1{{Game: "example", Set: "default"}},
	}
	if err := Validate(value, "deck-default"); err != nil {
		t.Fatal(err)
	}
	value.Mods = append(value.Mods, model.DeckyModV1{Game: "example", Set: "other"})
	if err := Validate(value, "deck-default"); err == nil {
		t.Fatal("expected duplicate game mod rejection")
	}
}

func TestValidateCatalogEmptyMarker(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "profiles"), 0o755); err != nil {
		t.Fatal(err)
	}
	grid := filepath.Join(root, "artwork", "steam-default", "grid")
	if err := os.MkdirAll(grid, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(grid, ".deck-profile-empty"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	artwork := "steam-default"
	value := model.DeckyProfileV1{
		Version: 1, ID: "steam-default", Name: "Steam default", Artwork: &artwork,
		Mods: []model.DeckyModV1{},
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "profiles", "steam-default.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCatalog(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(grid, "unexpected.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCatalog(root); err == nil {
		t.Fatal("expected mixed empty-marker payload rejection")
	}
}

func TestMissingModsDefaultsEmptyAndMarshalsExplicitly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mod-only.json")
	data := []byte(`{"version":1,"id":"mod-only","name":"Mod only","artwork":null}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	profile, err := LoadAndValidate(path)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"mods":[]`) {
		t.Fatalf("generated profile must include explicit empty mods: %s", encoded)
	}
}
