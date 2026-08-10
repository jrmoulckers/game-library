package source

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jrmoulckers/game-library/internal/model"
)

const countLimit = 10000

type Environment struct {
	GOOS    string
	HomeDir string
	Getenv  func(string) string
	Stat    func(string) (fs.FileInfo, error)
	ReadDir func(string) ([]os.DirEntry, error)
	WalkDir func(string, fs.WalkDirFunc) error
}

type Candidate struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	System     string `json:"system,omitempty"`
	ItemCount  int    `json:"itemCount"`
	Configured bool   `json:"configured"`
}

func Detect(env Environment, configured []model.Root) []Candidate {
	env = withDefaults(env)
	var candidates []Candidate
	seen := make(map[string]struct{})
	var seenInfo []fs.FileInfo
	usedIDs := make(map[string]int)
	configuredPaths := make(map[string]struct{}, len(configured))
	for _, root := range configured {
		configuredPaths[pathKey(root.Path, env.GOOS)] = struct{}{}
	}
	add := func(id, kind, name, path string) {
		path = filepath.Clean(path)
		key := pathKey(path, env.GOOS)
		if path == "." || path == "" {
			return
		}
		if _, exists := seen[key]; exists {
			return
		}
		info, err := env.Stat(path)
		if err != nil || !info.IsDir() {
			return
		}
		for _, existing := range seenInfo {
			if os.SameFile(existing, info) {
				return
			}
		}
		seen[key] = struct{}{}
		seenInfo = append(seenInfo, info)
		if count := usedIDs[id]; count > 0 {
			usedIDs[id] = count + 1
			id = fmt.Sprintf("%s-%d", id, count+1)
		} else {
			usedIDs[id] = 1
		}
		_, isConfigured := configuredPaths[key]
		candidates = append(candidates, Candidate{
			ID: id, Kind: kind, Name: name, Path: path,
			ItemCount: countItems(env, path), Configured: isConfigured,
		})
	}

	if env.GOOS == "windows" {
		programFiles := env.Getenv("ProgramFiles(x86)")
		if programFiles == "" {
			programFiles = env.Getenv("ProgramFiles")
		}
		if programFiles != "" {
			detectSteam(env, filepath.Join(programFiles, "Steam", "userdata"), add)
		}
		appData := env.Getenv("APPDATA")
		if appData != "" {
			add("playnite-library", "playnite-library", "Playnite library", filepath.Join(appData, "Playnite", "library", "files"))
			add("playnite-extra", "playnite-extra", "Playnite ExtraMetadata", filepath.Join(appData, "Playnite", "ExtraMetadata"))
		}
	} else {
		detectSteam(env, filepath.Join(env.HomeDir, ".local", "share", "Steam", "userdata"), add)
		detectSteam(env, filepath.Join(env.HomeDir, ".steam", "steam", "userdata"), add)
		add("retrodeck-media", "esde-media", "RetroDECK / ES-DE artwork", filepath.Join(env.HomeDir, "retrodeck", "ES-DE", "downloaded_media"))
	}

	add("gaming-profiles-syncthing", "decky-catalog", "Deck Gaming Profiles", filepath.Join(env.HomeDir, "Syncthing", "GamingProfiles"))
	add("gaming-profiles-home", "decky-catalog", "Deck Gaming Profiles", filepath.Join(env.HomeDir, "GamingProfiles"))

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Name != candidates[j].Name {
			return candidates[i].Name < candidates[j].Name
		}
		return candidates[i].ID < candidates[j].ID
	})
	return candidates
}

func detectSteam(env Environment, userdata string, add func(string, string, string, string)) {
	accounts, err := env.ReadDir(userdata)
	if err != nil {
		return
	}
	for _, account := range accounts {
		if !account.IsDir() || !numeric(account.Name()) {
			continue
		}
		id := "steam-" + account.Name()
		name := fmt.Sprintf("Steam custom artwork - account %s", account.Name())
		add(id, "steam-grid", name, filepath.Join(userdata, account.Name(), "config", "grid"))
	}
}

func countItems(env Environment, root string) int {
	count := 0
	_ = env.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err
			}
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.Type().IsRegular() {
			count++
			if count >= countLimit {
				return fs.SkipAll
			}
		}
		return nil
	})
	return count
}

func withDefaults(env Environment) Environment {
	if env.Getenv == nil {
		env.Getenv = os.Getenv
	}
	if env.Stat == nil {
		env.Stat = os.Stat
	}
	if env.ReadDir == nil {
		env.ReadDir = os.ReadDir
	}
	if env.WalkDir == nil {
		env.WalkDir = filepath.WalkDir
	}
	return env
}

func numeric(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func pathKey(path, goos string) string {
	cleaned := filepath.Clean(path)
	if goos == "windows" {
		return strings.ToLower(cleaned)
	}
	return cleaned
}
