package metadata

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jrmoulckers/game-library/internal/model"
)

// ResolveAll reads optional local metadata for the configured artwork roots.
// Every provider is best-effort: a provider diagnostic never prevents titles
// from another local source from being returned.
func ResolveAll(roots []model.Root) Catalog {
	builder := NewBuilder()
	steam := ResolveSteam(steamLocationsFromRoots(roots))
	for appID, record := range steam.Titles {
		builder.AddTitle("steam:"+strconv.FormatUint(uint64(appID), 10), record.Title, record.Source)
	}
	for _, diagnostic := range steam.Diagnostics {
		builder.AddDiagnostic(diagnostic)
	}
	for _, database := range playniteDatabasesFromRoots(roots) {
		playnite := ReadPlaynite(database)
		if playnite.Status != playniteStatusOK {
			for _, diagnostic := range playnite.Diagnostics {
				builder.AddDiagnostic(diagnostic)
			}
			continue
		}
		if len(playnite.Games) == 0 {
			builder.AddDiagnostic(Diagnostic{
				Source: "playnite", Status: "unavailable",
				Message: "Playnite titles unavailable - the local games collection contained no readable games.",
			})
			continue
		}
		builder.AddSource(SourceState{
			ID: "playnite-metadata", Name: "Playnite game titles",
			Status: "ready", ItemCount: len(playnite.Games),
		})
		for _, game := range playnite.Games {
			identity := "playnite:" + strings.ToLower(game.PlayniteGUID)
			builder.AddTitle(identity, game.Name, "playnite")
			builder.AddStore(identity, StoreName(game.PluginID))
			if StoreName(game.PluginID) != "Steam" {
				continue
			}
			appID, err := strconv.ParseUint(strings.TrimSpace(game.GameID), 10, 32)
			if err != nil || appID == 0 {
				continue
			}
			builder.AddAlias(identity, "steam:"+strconv.FormatUint(appID, 10))
		}
	}
	builder.Merge(ResolveESDE(roots, ESDEFileSystem{}))
	return builder.Build()
}

func steamLocationsFromRoots(roots []model.Root) SteamLocations {
	var locations SteamLocations
	for _, root := range roots {
		if root.Kind != "steam-grid" {
			continue
		}

		grid := filepath.Clean(root.Path)
		config := filepath.Dir(grid)
		account := filepath.Dir(config)
		userdata := filepath.Dir(account)
		if !strings.EqualFold(filepath.Base(grid), "grid") ||
			!strings.EqualFold(filepath.Base(config), "config") ||
			!strings.EqualFold(filepath.Base(userdata), "userdata") {
			continue
		}
		if _, err := strconv.ParseUint(filepath.Base(account), 10, 64); err != nil {
			continue
		}
		locations.AccountConfigDirs = append(locations.AccountConfigDirs, config)
		locations.InstallRoots = append(locations.InstallRoots, filepath.Dir(userdata))
	}
	return locations
}

func playniteDatabasesFromRoots(roots []model.Root) []string {
	seen := make(map[string]string)
	for _, root := range roots {
		var database string
		switch root.Kind {
		case "playnite-library":
			database = filepath.Join(filepath.Dir(filepath.Clean(root.Path)), "games.db")
		case "playnite-extra":
			database = filepath.Join(filepath.Dir(filepath.Clean(root.Path)), "library", "games.db")
		default:
			continue
		}
		key := database
		if filepath.Separator == '\\' {
			key = strings.ToLower(key)
		}
		seen[key] = database
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, seen[key])
	}
	return result
}
