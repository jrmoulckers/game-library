package metadata

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
)

func TestResolveAllJoinsPlayniteToSteamWithRealTitle(t *testing.T) {
	base := t.TempDir()
	steam := filepath.Join(base, "Steam")
	grid := filepath.Join(steam, "userdata", "100", "config", "grid")
	if err := os.MkdirAll(grid, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(steam, "appcache", "appinfo.vdf"), testAppInfoV29(map[uint32]string{
		42: "Synthetic Steam Title",
	}))

	playnite := filepath.Join(base, "Playnite")
	files := filepath.Join(playnite, "library", "files")
	if err := os.MkdirAll(files, 0o755); err != nil {
		t.Fatal(err)
	}
	guid := "12345678-1234-1234-9234-1234567890ab"
	writeSyntheticPlayniteFixture(
		t, filepath.Join(playnite, "library", "games.db"),
		guid, "Synthetic Playnite Title", "42", SteamPluginID,
	)

	catalog := ResolveAll([]model.Root{
		{Kind: "steam-grid", Path: grid},
		{Kind: "playnite-library", Path: files},
	})
	if got := catalog.Canonical("playnite:" + guid); got != "steam:42" {
		t.Fatalf("canonical identity = %q", got)
	}
	title, ok := catalog.Title("playnite:" + guid)
	if !ok || title.Title != "Synthetic Steam Title" {
		t.Fatalf("resolved title = %+v, %v", title, ok)
	}
	if catalog.Stores["playnite:"+guid] != "Steam" {
		t.Fatalf("store label = %q", catalog.Stores["playnite:"+guid])
	}
}
