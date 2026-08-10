package metadata

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
)

// TestLiveMetadata is an opt-in regression gate for local development. It
// reads existing sources without writing them and never logs paths, account
// identifiers, GUIDs, or titles.
func TestLiveMetadata(t *testing.T) {
	if os.Getenv("GAMELIB_LIVE_METADATA") != "1" {
		t.Skip("set GAMELIB_LIVE_METADATA=1 to test installed local sources read-only")
	}
	steamRoot := filepath.Join(os.Getenv("ProgramFiles(x86)"), "Steam")
	accounts, err := os.ReadDir(filepath.Join(steamRoot, "userdata"))
	if err != nil {
		t.Fatal("Steam userdata is unavailable")
	}
	var configs []string
	var roots []model.Root
	for _, account := range accounts {
		if !account.IsDir() || !isNumericName(account.Name()) {
			continue
		}
		config := filepath.Join(steamRoot, "userdata", account.Name(), "config")
		configs = append(configs, config)
		grid := filepath.Join(config, "grid")
		if info, err := os.Stat(grid); err == nil && info.IsDir() {
			roots = append(roots, model.Root{Kind: "steam-grid", Path: grid})
		}
	}
	steam := ResolveSteam(SteamLocations{
		InstallRoots: []string{steamRoot}, AccountConfigDirs: configs,
	})
	if len(steam.Titles) == 0 {
		t.Fatal("installed Steam sources resolved zero titles")
	}

	playniteFiles := filepath.Join(os.Getenv("APPDATA"), "Playnite", "library", "files")
	playnite := ReadPlaynite(filepath.Join(filepath.Dir(playniteFiles), "games.db"))
	if playnite.Status != playniteStatusOK || len(playnite.Games) == 0 {
		t.Fatalf("installed Playnite source resolved zero games (status %s)", playnite.Status)
	}
	roots = append(roots, model.Root{Kind: "playnite-library", Path: playniteFiles})
	all := ResolveAll(roots)
	if len(all.Aliases) == 0 {
		t.Fatal("installed sources produced zero exact Playnite-to-store joins")
	}
	t.Logf("live metadata: steam_titles=%d playnite_games=%d exact_joins=%d",
		len(steam.Titles), len(playnite.Games), len(all.Aliases))
}

func isNumericName(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}
