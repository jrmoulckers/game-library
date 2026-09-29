package publishing

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/jrmoulckers/game-library/internal/manifest"
	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/topology"
)

func pngFile(t *testing.T, path string) {
	t.Helper()
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	write(t, path, data.String())
}

func TestLegacySourceKeepsBlockedFilesAndBuiltInSemantics(t *testing.T) {
	root := t.TempDir()
	grid := filepath.Join(root, "artwork", "custom", "grid")
	pngFile(t, filepath.Join(grid, "123.png"))
	// Known non-image layout metadata is observed, never copied.
	write(t, filepath.Join(grid, "123.json"), `{"layout":true}`)
	write(t, filepath.Join(grid, "456.png"), "RIFFxxxxWEBPxxxx")
	pngFile(t, filepath.Join(grid, "789_icon.png"))
	pngFile(t, filepath.Join(grid, "789_icon.jpg"))
	// Both role variants are blocked, even when one has a media mismatch.
	pngFile(t, filepath.Join(grid, "789_icon.jpeg"))
	write(t, filepath.Join(grid, ".deck-profile-empty"), "")
	cfg := model.Config{Roots: []model.Root{{ID: "catalog", Kind: "decky-catalog", Path: root}}}
	source, err := FromArtwork(cfg, topology.Profile{Platform: "steam", Name: "Standard", Artwork: "custom"})
	if err != nil {
		t.Fatal(err)
	}
	target := Target{ID: "pc-steam", Platform: "steam", Adapter: "steam", Root: t.TempDir()}
	plan := preview(t, source, target)
	if len(plan.Files) != 6 || plan.Blocked != 5 {
		t.Fatalf("expected explicit blocked metadata/type/variants: %+v", plan)
	}
	for _, file := range plan.Files {
		if file.Path == ".deck-profile-empty" {
			t.Fatal("marker exported as artwork")
		}
	}
	builtIn := filepath.Join(root, "artwork", "steam-default", "grid")
	write(t, filepath.Join(builtIn, ".deck-profile-empty"), "")
	if _, err := FromArtwork(cfg, topology.Profile{Platform: "steam", Name: "Built in", Artwork: "steam-default"}); err == nil {
		t.Fatal("built-in artwork became a clearing export")
	}
	if _, err := FromArtwork(cfg, topology.Profile{Platform: "playnite", Name: "Standard", Artwork: "custom"}); err == nil {
		t.Fatal("Steam legacy payload exported as Playnite")
	}
}

func TestBlockedBytesNeverPublished(t *testing.T) {
	engine, source, target := fixture(t)
	source.Plan.Actions[0].Action = "blocked"
	source.Plan.Actions[0].Reason = "unsupported content"
	var err error
	source.Plan, err = manifest.NewPlan("steam-export-plan", source.Plan.Actions)
	if err != nil {
		t.Fatal(err)
	}
	plan := preview(t, source, target)
	if _, err := engine.Publish(source, target, plan.Digest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target.Root, "123.png")); !os.IsNotExist(err) {
		t.Fatal("blocked bytes were published")
	}
	if _, err := os.Stat(filepath.Join(target.Root, "123p.png")); err != nil {
		t.Fatal("valid bytes not published")
	}
	if _, err := engine.Publish(source, target, plan.Digest); err != nil {
		t.Fatal("blocked subset broke idempotence")
	}
}
