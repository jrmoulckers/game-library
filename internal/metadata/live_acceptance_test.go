package metadata

import (
	"os"
	"strings"
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
)

// Synthetic fixtures alone previously passed CI while every parser failed
// against real files, because the fixtures had been shaped to match the
// parsers. This test runs the real readers against real local sources when an
// operator points it at them:
//
//	GAMELIB_LIVE_STEAM_GRID=<...>\userdata\<account>\config\grid
//	GAMELIB_LIVE_PLAYNITE=<...>\Playnite\library\files
//
// It is skipped by default, so CI and other machines are unaffected. It only
// ever reports counts, never titles, paths, or identifiers.
func TestLiveSourcesResolveRealTitles(t *testing.T) {
	grid := os.Getenv("GAMELIB_LIVE_STEAM_GRID")
	playnite := os.Getenv("GAMELIB_LIVE_PLAYNITE")
	if grid == "" && playnite == "" {
		t.Skip("set GAMELIB_LIVE_STEAM_GRID and/or GAMELIB_LIVE_PLAYNITE to verify against real local sources")
	}

	var roots []model.Root
	if grid != "" {
		roots = append(roots, model.Root{Kind: "steam-grid", Path: grid})
	}
	if playnite != "" {
		roots = append(roots, model.Root{Kind: "playnite-library", Path: playnite})
	}

	catalog := ResolveAll(roots)
	steamTitles, playniteTitles, joins := 0, 0, 0
	for identity := range catalog.Titles {
		switch {
		case strings.HasPrefix(identity, "steam:"):
			steamTitles++
		case strings.HasPrefix(identity, "playnite:"):
			playniteTitles++
		}
	}
	for from, to := range catalog.Aliases {
		if strings.HasPrefix(from, "playnite:") && strings.HasPrefix(to, "steam:") {
			joins++
		}
	}
	t.Logf("steam titles=%d playnite titles=%d playnite->steam joins=%d diagnostics=%d",
		steamTitles, playniteTitles, joins, len(catalog.Diagnostics))

	if grid != "" && steamTitles == 0 {
		t.Error("a real Steam installation resolved no titles")
	}
	if playnite != "" && playniteTitles == 0 {
		t.Error("a real Playnite library resolved no titles (a running Playnite locks the database, which is reported as busy rather than failing here)")
	}
	if grid != "" && playnite != "" && joins == 0 {
		t.Error("no Playnite game canonicalised onto a Steam identity")
	}
}
