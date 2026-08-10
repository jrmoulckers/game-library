package media

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
)

func TestInferRole(t *testing.T) {
	tests := map[string]string{
		"123.png":      "grid",
		"123p.jpg":     "portrait",
		"123_hero.png": "hero",
		"123_logo.png": "logo",
		"123_icon.png": "icon",
	}

	for path, expected := range tests {
		if actual := InferRole("steam-grid", path); actual != expected {
			t.Fatalf("InferRole(%q) = %q, want %q", path, actual, expected)
		}
	}
	if actual := InferRole("playnite-extra", "games/id/Logo.png"); actual != "logo" {
		t.Fatalf("Playnite logo role = %q", actual)
	}
	if actual := InferRole("playnite-library", "12345678-1234-1234-9234-1234567890ab.png"); actual != "cover" {
		t.Fatalf("Playnite library role = %q", actual)
	}
	if actual := InferRole("esde-media", "n64/manuals/Game.pdf"); actual != "manual" {
		t.Fatalf("ES-DE manual role = %q", actual)
	}
	if actual := InferRole("steam-grid", "123.json"); actual != "" {
		t.Fatalf("Steam JSON role = %q, want empty", actual)
	}
}

func TestInspectEmptyMarker(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".deck-profile-empty")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	facts, err := Inspect(path, "decky-catalog", "artwork/steam-default/grid/.deck-profile-empty")
	if err != nil {
		t.Fatal(err)
	}
	if facts.MIME == "" {
		t.Fatal("expected a MIME classification")
	}
	if facts.Extension != "" {
		t.Fatalf("empty marker extension = %q, want empty", facts.Extension)
	}
}

func TestValidateTypeRejectsExtensionMismatch(t *testing.T) {
	err := ValidateType(model.MediaFacts{Extension: "png", MIME: "text/plain; charset=utf-8"})
	if err == nil {
		t.Fatal("expected extension/MIME mismatch")
	}
}

func TestIdentityHints(t *testing.T) {
	if actual := InferIdentityHint("steam-grid", "123p.png", ""); actual != "steam:123" {
		t.Fatalf("Steam hint = %q", actual)
	}
	if actual := InferIdentityHint("playnite-extra", "games/12345678-1234-1234-9234-1234567890ab/Logo.png", ""); actual != "playnite:12345678-1234-1234-9234-1234567890ab" {
		t.Fatalf("Playnite hint = %q", actual)
	}
	if actual := InferIdentityHint("playnite-library", "12345678-1234-1234-9234-1234567890ab.png", ""); actual != "playnite:12345678-1234-1234-9234-1234567890ab" {
		t.Fatalf("Playnite file hint = %q", actual)
	}
	root := model.Root{Kind: "esde-media"}
	system := InferSystem(root, "n64/covers/Example Game.png")
	if actual := InferIdentityHint(root.Kind, "n64/covers/Example Game.png", system); actual != "retro:n64:example-game" {
		t.Fatalf("Retro hint = %q", actual)
	}
}
