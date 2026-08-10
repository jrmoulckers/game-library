package metadata

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
)

func TestFingerprintChangesWithLocalMetadata(t *testing.T) {
	base := t.TempDir()
	media := filepath.Join(base, "downloaded_media")
	gamelist := filepath.Join(base, "gamelists", "n64", "gamelist.xml")
	if err := os.MkdirAll(filepath.Dir(gamelist), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(media, 0o755); err != nil {
		t.Fatal(err)
	}
	roots := []model.Root{{Kind: "esde-media", Path: media}}
	before := Fingerprint(roots)
	if err := os.WriteFile(gamelist, []byte("<gameList/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	after := Fingerprint(roots)
	if before == after {
		t.Fatal("metadata fingerprint did not change")
	}
}
