package metadata

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jrmoulckers/game-library/internal/model"
)

func Fingerprint(roots []model.Root) string {
	paths := metadataPaths(roots)
	hash := sha256.New()
	for _, name := range paths {
		info, err := os.Stat(name)
		if err != nil {
			fmt.Fprintf(hash, "%s\x00missing\x00", name)
			continue
		}
		fmt.Fprintf(hash, "%s\x00%d\x00%d\x00", name, info.Size(), info.ModTime().UnixNano())
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func metadataPaths(roots []model.Root) []string {
	seen := make(map[string]struct{})
	add := func(name string) {
		if name == "" {
			return
		}
		name = filepath.Clean(name)
		key := name
		if filepath.Separator == '\\' {
			key = strings.ToLower(key)
		}
		seen[key] = struct{}{}
	}

	steam := steamLocationsFromRoots(roots)
	libraries := append([]string(nil), steam.InstallRoots...)
	for _, install := range steam.InstallRoots {
		appinfo := filepath.Join(install, "appcache", "appinfo.vdf")
		folders := filepath.Join(install, "steamapps", "libraryfolders.vdf")
		add(appinfo)
		add(folders)
		if data, err := os.ReadFile(folders); err == nil && len(data) <= steamMaxTextFile {
			if discovered, err := parseSteamLibraryFolders(data); err == nil {
				libraries = append(libraries, discovered...)
			}
		}
	}
	for _, account := range steam.AccountConfigDirs {
		add(filepath.Join(account, "shortcuts.vdf"))
	}
	for _, library := range uniqueSteamPaths(libraries) {
		manifestDir := filepath.Join(library, "steamapps")
		add(manifestDir)
		matches, globErr := filepath.Glob(filepath.Join(manifestDir, "appmanifest_*.acf"))
		// Best-effort: an unreadable library contributes no manifests to the
		// fingerprint, which is treated the same as a library with none.
		_ = globErr
		for _, match := range matches {
			add(match)
		}
	}

	for _, root := range roots {
		switch root.Kind {
		case "playnite-library":
			add(filepath.Join(filepath.Dir(filepath.Clean(root.Path)), "games.db"))
		case "playnite-extra":
			add(filepath.Join(filepath.Dir(filepath.Clean(root.Path)), "library", "games.db"))
		case "esde-media":
			gamelists := filepath.Join(filepath.Dir(filepath.Clean(root.Path)), "gamelists")
			add(gamelists)
			systems, readErr := os.ReadDir(gamelists)
			// Best-effort: an unreadable gamelists directory contributes no
			// per-system entries, the same as an empty one.
			_ = readErr
			for _, system := range systems {
				if system.IsDir() && safeSystemKey(system.Name()) {
					add(filepath.Join(gamelists, system.Name(), "gamelist.xml"))
				}
			}
		}
	}

	result := make([]string, 0, len(seen))
	for name := range seen {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}
