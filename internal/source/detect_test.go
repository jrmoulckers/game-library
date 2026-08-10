package source

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
)

func TestDetectWindowsSourcesAndMultipleSteamAccounts(t *testing.T) {
	root := t.TempDir()
	programFiles := filepath.Join(root, "Program Files")
	appData := filepath.Join(root, "Roaming")
	home := filepath.Join(root, "home")
	makeFile(t, filepath.Join(programFiles, "Steam", "userdata", "111", "config", "grid", "10.png"))
	makeFile(t, filepath.Join(programFiles, "Steam", "userdata", "222", "config", "grid", "20p.png"))
	makeFile(t, filepath.Join(programFiles, "Steam", "userdata", "not-an-account", "config", "grid", "ignored.png"))
	makeFile(t, filepath.Join(appData, "Playnite", "library", "files", "game.png"))
	makeFile(t, filepath.Join(appData, "Playnite", "ExtraMetadata", "games", "id", "Logo.png"))
	makeFile(t, filepath.Join(home, "Syncthing", "GamingProfiles", "profiles", "deck-default.json"))

	env := Environment{
		GOOS: "windows", HomeDir: home,
		Getenv: func(key string) string {
			switch key {
			case "ProgramFiles(x86)":
				return programFiles
			case "APPDATA":
				return appData
			default:
				return ""
			}
		},
	}
	got := Detect(env, []model.Root{{Path: filepath.Join(appData, "Playnite", "library", "files")}})
	if len(got) != 5 {
		t.Fatalf("candidates = %#v", got)
	}
	configured := 0
	for _, candidate := range got {
		if candidate.Configured {
			configured++
		}
		if candidate.ItemCount != 1 {
			t.Errorf("%s item count = %d", candidate.ID, candidate.ItemCount)
		}
	}
	if configured != 1 {
		t.Fatalf("configured candidates = %d", configured)
	}
}

func TestDetectLinuxSources(t *testing.T) {
	home := t.TempDir()
	makeFile(t, filepath.Join(home, ".local", "share", "Steam", "userdata", "333", "config", "grid", "30_hero.png"))
	makeFile(t, filepath.Join(home, "retrodeck", "ES-DE", "downloaded_media", "n64", "covers", "Game.png"))
	makeFile(t, filepath.Join(home, "GamingProfiles", "artwork", "deck-default", "grid", "30.png"))
	got := Detect(Environment{GOOS: "linux", HomeDir: home}, nil)
	if len(got) != 3 {
		t.Fatalf("candidates = %#v", got)
	}

}

func TestDetectKeepsDistinctLinuxSteamIDsUnique(t *testing.T) {
	home := t.TempDir()
	for _, base := range []string{
		filepath.Join(home, ".local", "share", "Steam"),
		filepath.Join(home, ".steam", "steam"),
	} {
		makeFile(t, filepath.Join(base, "userdata", "333", "config", "grid", "30.png"))
	}
	got := Detect(Environment{GOOS: "linux", HomeDir: home}, nil)
	ids := make(map[string]struct{})
	for _, candidate := range got {
		if candidate.Kind == "steam-grid" {
			ids[candidate.ID] = struct{}{}
		}
	}
	if len(ids) != 2 {
		t.Fatalf("Steam candidate IDs are not unique: %#v", got)
	}
}

func TestPathKeyRespectsFilesystemCaseRules(t *testing.T) {
	if pathKey("Artwork", "linux") == pathKey("artwork", "linux") {
		t.Fatal("Linux path keys must preserve case")
	}

	if pathKey("Artwork", "windows") != pathKey("artwork", "windows") {
		t.Fatal("Windows path keys must fold case")
	}

}

func TestSupportedStatesTreatsAbsentRetrodeckAsNeutral(t *testing.T) {
	states := SupportedStates([]Candidate{{Kind: "steam-grid"}}, nil)
	for _, state := range states {
		if state.Kind == "steam-grid" {
			t.Fatal("present Steam source should not get an absent state")
		}
		if state.Kind == "esde-media" {
			if state.Status != "not-on-this-device" || !strings.Contains(state.Message, "Not on this device") {
				t.Fatalf("RetroDECK state = %+v", state)
			}
			return
		}
	}
	t.Fatal("expected neutral RetroDECK state")
}

func TestDetectUsesCurrentPlatformDefaults(t *testing.T) {
	home := t.TempDir()
	env := Environment{
		GOOS: runtime.GOOS, HomeDir: home,
		Getenv: func(string) string { return "" },
	}
	if got := Detect(env, nil); len(got) != 0 {
		t.Fatalf("unexpected candidates = %#v", got)
	}
}

func makeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("synthetic"), 0o644); err != nil {
		t.Fatal(err)
	}
}
