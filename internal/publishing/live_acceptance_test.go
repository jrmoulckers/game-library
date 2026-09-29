package publishing

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jrmoulckers/game-library/internal/inventory"
	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/topology"
	"github.com/jrmoulckers/game-library/internal/workspace"
)

// This opt-in acceptance only reads real catalog and target bytes. It neither
// stages exports nor saves targets nor invokes Publish or Rollback.
// Logs contain counts only, never game IDs, paths or hashes.
func TestLiveReadOnlyProfileParity(t *testing.T) {
	root := os.Getenv("GAMELIB_LIVE_WORKSPACE")
	grid := os.Getenv("GAMELIB_LIVE_STEAM_GRID")
	if root == "" || grid == "" {
		t.Skip("set GAMELIB_LIVE_WORKSPACE and GAMELIB_LIVE_STEAM_GRID for read-only acceptance")
	}
	paths := workspace.NewPaths(root)
	cfg, found, err := workspace.LoadActiveConfig(paths.Config)
	if err != nil || !found {
		t.Fatal("real local source configuration unavailable")
	}
	doc, _, err := topology.Load(paths.Topology)
	if err != nil {
		t.Fatal("real local topology unavailable")
	}
	accepted := 0
	for _, declared := range doc.Profiles {
		if declared.Platform != "steam" || declared.Artwork == "" {
			continue
		}
		source, err := FromArtwork(cfg, declared)
		if err != nil {
			for _, root := range cfg.Roots {
				if root.Kind != "decky-catalog" {
					continue
				}
				scanned, scanErr := inventory.Scan([]model.Root{{ID: "acceptance", Kind: "steam-grid",
					Path: filepath.Join(root.Path, "artwork", declared.Artwork, "grid")}})
				if scanErr == nil {
					counts := map[string]int{}
					for _, issue := range scanned.Issues {
						counts[issue.Code+": "+issue.Message]++
					}
					t.Logf("source issues by type: %v", counts)
				}
			}
			t.Fatalf("real bound profile could not produce a safe export: %v", err)
		}
		target := Target{ID: "pc-steam", Device: "pc", Platform: "steam", Name: "PC Steam", Adapter: "steam", Root: grid}
		plan, err := Build(source, target)
		if err != nil {
			t.Fatalf("real profile preview rejected: %v", err)
		}
		games := map[string]bool{}
		for _, action := range source.Plan.Actions {
			games[action.Metadata["gameId"]] = true
		}
		t.Logf("bound profiles=1 games=%d files=%d matching=%d missing=%d different=%d unknown=%d blocked=%d exportable=%d",
			len(games), len(plan.Files), plan.Matching, plan.Missing, plan.Different, plan.Unknown, plan.Blocked, len(plan.Files)-plan.Blocked)
		t.Logf("publishable files=%d matching=%d missing=%d different=%d unknown=%d",
			plan.Publishable.Files, plan.Publishable.Matching, plan.Publishable.Missing, plan.Publishable.Different, plan.Publishable.Unknown)
		if plan.State != "observed" || len(plan.Files) == 0 || plan.Unknown != 0 {
			t.Fatal("real local Steam target was not fully observed")
		}
		var eligible Counts
		for _, file := range plan.Files {
			if file.Blocked {
				continue
			}
			eligible.Files++
			switch file.Status {
			case "matching":
				eligible.Matching++
			case "missing":
				eligible.Missing++
			case "different":
				eligible.Different++
			default:
				eligible.Unknown++
			}
		}
		if eligible != plan.Publishable || eligible.Files != len(plan.Files)-plan.Blocked {
			t.Fatal("publishable counts do not match the exact eligible file preview")
		}
		if output := os.Getenv("GAMELIB_LIVE_PREVIEW_FILE"); output != "" {
			replacements := []File{}
			for _, file := range plan.Files {
				if !file.Blocked && file.Status == "different" {
					replacements = append(replacements, file)
				}
			}
			// Only an explicitly requested private report is written. This
			// is not export staging and does not authorize any copy.
			if err := workspace.WriteLocalJSON(output, struct {
				Plan         Plan   `json:"plan"`
				Target       Target `json:"target"`
				Replacements []File `json:"replacements"`
			}{plan, target, replacements}); err != nil {
				t.Fatal("could not save the requested private dry-run")
			}
			t.Logf("private dry-run replacements=%d", len(replacements))
		}
		if again, err := Build(source, target); err != nil || again.Digest != plan.Digest {
			t.Fatal("real read-only preview is not deterministic")
		}
		target.Root = ""
		remote, err := Build(source, target)
		if err != nil || remote.State != "unobserved" || remote.Unknown != len(plan.Files) || remote.Matching != 0 {
			t.Fatal("unobserved device was incorrectly credited with parity")
		}
		accepted++
	}
	if accepted == 0 {
		t.Fatal("no bound Steam profile available for real-library acceptance")
	}
}
