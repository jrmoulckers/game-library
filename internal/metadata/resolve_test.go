package metadata

import (
	"path/filepath"
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
)

func TestSteamLocationsFromConfiguredGridRoots(t *testing.T) {
	install := filepath.Join(t.TempDir(), "Steam")
	config := filepath.Join(install, "userdata", "100", "config")
	got := steamLocationsFromRoots([]model.Root{
		{Kind: "steam-grid", Path: filepath.Join(config, "grid")},
		{Kind: "steam-grid", Path: filepath.Join(install, "unexpected")},
		{Kind: "generic", Path: filepath.Join(config, "grid")},
	})
	if len(got.InstallRoots) != 1 || got.InstallRoots[0] != install {
		t.Fatalf("install roots = %#v", got.InstallRoots)
	}

	if len(got.AccountConfigDirs) != 1 || got.AccountConfigDirs[0] != config {
		t.Fatalf("account configs = %#v", got.AccountConfigDirs)
	}

}

func TestPlayniteDatabasesFromConfiguredRoots(t *testing.T) {
	base := t.TempDir()
	want := filepath.Join(base, "library", "games.db")
	got := playniteDatabasesFromRoots([]model.Root{
		{Kind: "playnite-library", Path: filepath.Join(base, "library", "files")},
		{Kind: "playnite-extra", Path: filepath.Join(base, "ExtraMetadata")},
	})
	if len(got) != 1 || got[0] != want {
		t.Fatalf("Playnite databases = %#v", got)
	}
}
