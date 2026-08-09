package inventory

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
)

func TestScanAndSanitize(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	writePNG(t, filepath.Join(first, "123.png"))
	data, err := os.ReadFile(filepath.Join(first, "123.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(second, "123.png"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Scan([]model.Root{
		{ID: "one", Kind: "steam-grid", Path: first},
		{ID: "two", Kind: "steam-grid", Path: second},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Observations) != 2 {
		t.Fatalf("observations = %d", len(result.Observations))
	}
	if result.DuplicateSummary.Groups != 1 || result.DuplicateSummary.CrossRootGroups != 1 {
		t.Fatalf("unexpected duplicate summary: %+v", result.DuplicateSummary)
	}
	if result.Roots[0].Dimensions["2x3"] != 1 {
		t.Fatalf("dimensions: %+v", result.Roots[0].Dimensions)
	}
	sanitized := Sanitize(result)
	if sanitized.Privacy != "sanitized" || len(sanitized.Observations) != 0 {
		t.Fatalf("sanitized inventory leaked observations: %+v", sanitized)
	}
}

func writePNG(t *testing.T, path string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	img := image.NewRGBA(image.Rect(0, 0, 2, 3))
	img.Set(0, 0, color.White)
	if err := png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
}
