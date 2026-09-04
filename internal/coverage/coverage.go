// Package coverage answers the two questions an artwork collection
// actually raises day to day: which profiles have media for a given game,
// and which games have media in a given profile.
//
// Both answers carry the platform the profile belongs to and the hardware
// that platform runs on, because "Steam Minimalist" only means something
// once you know it is Steam and that Steam is on three devices.
//
// Artwork is never matched across platforms. A Steam capsule and a retro
// screenshot for the same title are separate things in separate profiles,
// which is what the owner asked for. The only sharing is within a
// platform: a profile is stored once and reaches every device that runs
// that platform, so those devices are in parity by construction.
package coverage

import (
	"sort"
	"strings"

	"github.com/jrmoulckers/game-library/internal/organizer"
	"github.com/jrmoulckers/game-library/internal/topology"
)

// DeviceRef is a piece of hardware a profile reaches.
type DeviceRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RoleCount is how many files a profile holds for one artwork role.
type RoleCount struct {
	Role  string `json:"role"`
	Count int    `json:"count"`
}

// ProfileGame is one game's presence inside a profile.
type ProfileGame struct {
	GameID     string   `json:"gameId"`
	Title      string   `json:"title"`
	PlatformID string   `json:"platformId"`
	Roles      []string `json:"roles"`
	AssetCount int      `json:"assetCount"`
}

// Profile is a named artwork set for one platform, with what it holds.
type Profile struct {
	Key          string        `json:"key"`
	Name         string        `json:"name"`
	PlatformID   string        `json:"platformId"`
	PlatformName string        `json:"platformName"`
	ArtworkSet   string        `json:"artworkSet,omitempty"`
	Devices      []DeviceRef   `json:"devices"`
	GameCount    int           `json:"gameCount"`
	AssetCount   int           `json:"assetCount"`
	Roles        []RoleCount   `json:"roles,omitempty"`
	Games        []ProfileGame `json:"games"`
	// Empty marks a profile that has been declared but holds no artwork
	// yet. This is a normal state, not a fault.
	Empty bool `json:"empty"`
}

// GameProfile is one profile's coverage of a single game.
type GameProfile struct {
	Key          string      `json:"key"`
	Name         string      `json:"name"`
	PlatformID   string      `json:"platformId"`
	PlatformName string      `json:"platformName"`
	Devices      []DeviceRef `json:"devices"`
	Roles        []string    `json:"roles"`
	AssetCount   int         `json:"assetCount"`
	// Covered is false for a profile that applies to this game's platform
	// but holds no artwork for it. Those are the gaps worth filling.
	Covered bool `json:"covered"`
}

// Game is a single title and how every relevant profile covers it.
type Game struct {
	GameID       string        `json:"gameId"`
	Title        string        `json:"title"`
	PlatformID   string        `json:"platformId"`
	PlatformName string        `json:"platformName"`
	Profiles     []GameProfile `json:"profiles"`
	CoveredCount int           `json:"coveredCount"`
}

// UnboundSet is an artwork set present in the catalog that no declared
// profile claims. Surfacing these is the difference between "you have no
// profiles" and "you have artwork nobody has named yet".
type UnboundSet struct {
	ArtworkSet string `json:"artworkSet"`
	GameCount  int    `json:"gameCount"`
	AssetCount int    `json:"assetCount"`
}

// Surface is a live frontend directory on this machine, as opposed to the
// canonical catalog. It is reported separately because a live directory
// is a published copy: it can drift from the profile it came from, and it
// is never the source of truth.
type Surface struct {
	SourceID   string `json:"sourceId"`
	SourceName string `json:"sourceName"`
	RootKind   string `json:"rootKind"`
	GameCount  int    `json:"gameCount"`
	AssetCount int    `json:"assetCount"`
}

// Report is the full two-way coverage picture.
type Report struct {
	Profiles     []Profile    `json:"profiles"`
	Games        []Game       `json:"games"`
	Unbound      []UnboundSet `json:"unbound"`
	LiveSurfaces []Surface    `json:"liveSurfaces"`
}

// platformOf maps an organizer platform id onto a topology platform id.
// Organizer platforms are fine-grained for retro systems ("retro:snes"),
// while a person thinks in terms of one "Retro gaming" platform, so every
// retro system folds into the same topology platform.
func platformOf(organizerPlatformID string) string {
	if strings.HasPrefix(organizerPlatformID, "retro:") {
		return "retro"
	}
	return organizerPlatformID
}

// held is what one artwork set holds for one game.
type held struct {
	roles  map[string]struct{}
	assets int
}

// Build computes the coverage report from an already-scanned catalog and
// the owner's declared topology. It re-reads nothing from disk.
func Build(catalog organizer.Catalog, doc topology.Document) Report {
	sets := make(map[string]map[string]*held)
	surfaces := make(map[string]*Surface)
	surfaceGames := make(map[string]map[string]struct{})
	titles := make(map[string]organizer.Game, len(catalog.Games))

	for _, game := range catalog.Games {
		titles[game.ID] = game
		for _, asset := range game.Assets {
			if asset.ArtworkSet != "" {
				contents := sets[asset.ArtworkSet]
				if contents == nil {
					contents = make(map[string]*held)
					sets[asset.ArtworkSet] = contents
				}
				entry := contents[game.ID]
				if entry == nil {
					entry = &held{roles: make(map[string]struct{})}
					contents[game.ID] = entry
				}
				entry.roles[asset.Role] = struct{}{}
				entry.assets++
				continue
			}
			// Anything that is not catalog payload is a live frontend
			// directory on this machine.
			if asset.RootKind == "" || asset.RootKind == "decky-catalog" {
				continue
			}
			surface := surfaces[asset.SourceID]
			if surface == nil {
				surface = &Surface{
					SourceID:   asset.SourceID,
					SourceName: asset.SourceName,
					RootKind:   asset.RootKind,
				}
				surfaces[asset.SourceID] = surface
				surfaceGames[asset.SourceID] = make(map[string]struct{})
			}
			surface.AssetCount++
			if _, seen := surfaceGames[asset.SourceID][game.ID]; !seen {
				surfaceGames[asset.SourceID][game.ID] = struct{}{}
				surface.GameCount++
			}
		}
	}

	report := Report{
		Profiles:     []Profile{},
		Games:        []Game{},
		Unbound:      []UnboundSet{},
		LiveSurfaces: []Surface{},
	}
	claimed := make(map[string]struct{}, len(doc.Profiles))

	for _, declared := range doc.Profiles {
		profile := Profile{
			Key:          declared.Key(),
			Name:         declared.Name,
			PlatformID:   declared.Platform,
			PlatformName: doc.PlatformName(declared.Platform),
			ArtworkSet:   declared.Artwork,
			Devices:      deviceRefs(doc.DevicesFor(declared.Platform)),
			Games:        []ProfileGame{},
		}
		if declared.Artwork != "" {
			claimed[declared.Artwork] = struct{}{}
		}
		contents := sets[declared.Artwork]
		if len(contents) == 0 {
			profile.Empty = true
			report.Profiles = append(report.Profiles, profile)
			continue
		}
		roleTotals := make(map[string]int)
		for gameID, entry := range contents {
			roles := make([]string, 0, len(entry.roles))
			for role := range entry.roles {
				roles = append(roles, role)
				roleTotals[role]++
			}
			sort.Strings(roles)
			game := titles[gameID]
			title := game.Title
			if title == "" {
				title = gameID
			}
			profile.Games = append(profile.Games, ProfileGame{
				GameID:     gameID,
				Title:      title,
				PlatformID: game.PlatformID,
				Roles:      roles,
				AssetCount: entry.assets,
			})
			profile.AssetCount += entry.assets
		}
		sortProfileGames(profile.Games)
		profile.GameCount = len(profile.Games)
		profile.Roles = sortedRoles(roleTotals)
		report.Profiles = append(report.Profiles, profile)
	}

	for set, contents := range sets {
		if _, ok := claimed[set]; ok {
			continue
		}
		unbound := UnboundSet{ArtworkSet: set, GameCount: len(contents)}
		for _, entry := range contents {
			unbound.AssetCount += entry.assets
		}
		report.Unbound = append(report.Unbound, unbound)
	}
	sort.Slice(report.Unbound, func(i, j int) bool {
		return report.Unbound[i].ArtworkSet < report.Unbound[j].ArtworkSet
	})

	// Game-facing view: every profile whose platform matches the game's
	// platform is relevant, whether or not it holds anything.
	byPlatform := make(map[string][]Profile)
	for _, profile := range report.Profiles {
		byPlatform[profile.PlatformID] = append(byPlatform[profile.PlatformID], profile)
	}
	for _, game := range catalog.Games {
		entry := Game{
			GameID:       game.ID,
			Title:        game.Title,
			PlatformID:   game.PlatformID,
			PlatformName: game.PlatformName,
			Profiles:     []GameProfile{},
		}
		for _, profile := range byPlatform[platformOf(game.PlatformID)] {
			gp := GameProfile{
				Key:          profile.Key,
				Name:         profile.Name,
				PlatformID:   profile.PlatformID,
				PlatformName: profile.PlatformName,
				Devices:      profile.Devices,
				Roles:        []string{},
			}
			if entryHeld := sets[profile.ArtworkSet][game.ID]; entryHeld != nil {
				roles := make([]string, 0, len(entryHeld.roles))
				for role := range entryHeld.roles {
					roles = append(roles, role)
				}
				sort.Strings(roles)
				gp.Roles = roles
				gp.AssetCount = entryHeld.assets
				gp.Covered = true
				entry.CoveredCount++
			}
			entry.Profiles = append(entry.Profiles, gp)
		}
		report.Games = append(report.Games, entry)
	}

	for _, surface := range surfaces {
		report.LiveSurfaces = append(report.LiveSurfaces, *surface)
	}
	sort.Slice(report.LiveSurfaces, func(i, j int) bool {
		return report.LiveSurfaces[i].SourceID < report.LiveSurfaces[j].SourceID
	})
	return report
}

func sortProfileGames(games []ProfileGame) {
	sort.Slice(games, func(i, j int) bool {
		left, right := strings.ToLower(games[i].Title), strings.ToLower(games[j].Title)
		if left != right {
			return left < right
		}
		return games[i].GameID < games[j].GameID
	})
}

func deviceRefs(devices []topology.Device) []DeviceRef {
	refs := make([]DeviceRef, 0, len(devices))
	for _, device := range devices {
		refs = append(refs, DeviceRef{ID: device.ID, Name: device.Name})
	}
	return refs
}

func sortedRoles(totals map[string]int) []RoleCount {
	roles := make([]RoleCount, 0, len(totals))
	for role, count := range totals {
		roles = append(roles, RoleCount{Role: role, Count: count})
	}
	sort.Slice(roles, func(i, j int) bool {
		if roles[i].Count != roles[j].Count {
			return roles[i].Count > roles[j].Count
		}
		return roles[i].Role < roles[j].Role
	})
	return roles
}
