package organizer

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/review"
)

var (
	trailingTags = regexp.MustCompile(`(?i)(?:\s*[\(\[][^\(\)\[\]]*(?:[\)\]])\s*)+$`)
	spaceRuns    = regexp.MustCompile(`[\s._-]+`)
)

var systemAliases = map[string]string{
	"3ds":       "Nintendo 3DS",
	"gc":        "Nintendo GameCube",
	"gamecube":  "Nintendo GameCube",
	"gb":        "Game Boy",
	"gba":       "Game Boy Advance",
	"gbc":       "Game Boy Color",
	"genesis":   "Sega Genesis",
	"megadrive": "Sega Mega Drive",
	"n64":       "Nintendo 64",
	"nds":       "Nintendo DS",
	"nes":       "Nintendo Entertainment System",
	"ps2":       "PlayStation 2",
	"ps3":       "PlayStation 3",
	"psp":       "PlayStation Portable",
	"saturn":    "Sega Saturn",
	"snes":      "Super Nintendo Entertainment System",
	"switch":    "Nintendo Switch",
	"wii":       "Nintendo Wii",
	"wiiu":      "Nintendo Wii U",
}

var expectedRoles = []string{
	"grid", "portrait", "cover", "hero", "logo", "icon", "banner",
	"marquee", "screenshot", "fanart", "miximage", "physicalmedia",
	"manual", "video",
}

type Catalog struct {
	Platforms      []Platform `json:"platforms"`
	Games          []Game     `json:"games"`
	NeedsAttention int        `json:"needsAttention"`
	ScannedAt      string     `json:"scannedAt,omitempty"`
}

type Platform struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	GameCount       int      `json:"gameCount"`
	ArtworkCount    int      `json:"artworkCount"`
	Coverage        int      `json:"coverage"`
	MissingArtCount int      `json:"missingArtCount"`
	PreviewIDs      []string `json:"previewIds,omitempty"`
	Assets          []Asset  `json:"assets,omitempty"`
}

type Game struct {
	ID             string            `json:"id"`
	Title          string            `json:"title"`
	PlatformID     string            `json:"platformId"`
	PlatformName   string            `json:"platformName"`
	Identities     map[string]string `json:"identities"`
	Assets         []Asset           `json:"assets"`
	MissingRoles   []string          `json:"missingRoles"`
	Profiles       []string          `json:"profiles,omitempty"`
	Fallbacks      []Fallback        `json:"fallbacks,omitempty"`
	NeedsAttention bool              `json:"needsAttention"`
	RawTitle       string            `json:"rawTitle,omitempty"`
}

type Fallback struct {
	Frontend string   `json:"frontend"`
	Roles    []string `json:"roles"`
	Message  string   `json:"message"`
}

type Asset struct {
	ID           string   `json:"id"`
	SHA256       string   `json:"sha256"`
	Role         string   `json:"role"`
	Width        int      `json:"width,omitempty"`
	Height       int      `json:"height,omitempty"`
	Aspect       string   `json:"aspect,omitempty"`
	MIME         string   `json:"mime"`
	Extension    string   `json:"extension,omitempty"`
	Size         int64    `json:"size"`
	SourceID     string   `json:"sourceId"`
	SourceName   string   `json:"sourceName"`
	Location     string   `json:"location"`
	SharedCopies int      `json:"sharedCopies"`
	Profiles     []string `json:"profiles,omitempty"`
}

func SystemName(key string) string {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if name := systemAliases[normalized]; name != "" {
		return name
	}
	if normalized == "" {
		return "Other"
	}
	words := strings.Fields(spaceRuns.ReplaceAllString(normalized, " "))
	if len(words) == 0 {
		return "Other"
	}
	for i := range words {
		words[i] = strings.ToUpper(words[i][:1]) + words[i][1:]
	}
	return strings.Join(words, " ")
}

func CleanRetroTitle(stem string) string {
	raw := strings.TrimSpace(stem)
	for {
		cleaned := strings.TrimSpace(trailingTags.ReplaceAllString(raw, ""))
		if cleaned == raw {
			break
		}
		raw = cleaned
	}
	cleaned := strings.TrimSpace(spaceRuns.ReplaceAllString(raw, " "))
	if cleaned == "" {
		return "Untitled game"
	}
	return cleaned
}

func Build(snapshot review.Snapshot, profiles []model.Profile) Catalog {
	hashCopies := make(map[string]int)
	hashProfiles := make(map[string]map[string]struct{})
	for _, observation := range snapshot.Inventory.Observations {
		hashCopies[observation.SHA256]++
	}
	for _, profile := range profiles {
		for _, game := range profile.Games {
			for _, asset := range game.Assets {
				if hashProfiles[asset.SHA256] == nil {
					hashProfiles[asset.SHA256] = make(map[string]struct{})
				}
				hashProfiles[asset.SHA256][profile.Name] = struct{}{}
			}
		}
	}

	games := make(map[string]*Game)
	platformAssets := make(map[string][]Asset)
	for _, profile := range profiles {
		for _, profileGame := range profile.Games {
			id, platformID, platformName, title := profileGameIdentity(profileGame)
			if id == "" || games[id] != nil {
				continue
			}
			games[id] = &Game{
				ID: id, Title: title, PlatformID: platformID, PlatformName: platformName,
				Identities: profileGame.Identities,
			}
		}
	}
	for _, observation := range snapshot.Inventory.Observations {
		if platformID, ok := platformAssetPlatform(observation); ok {
			platformAssets[platformID] = append(platformAssets[platformID], buildAsset(observation, hashCopies, hashProfiles))
			continue
		}

		id := observation.IdentityHint
		attention := id == ""
		if id == "" {
			id = "unmapped:" + review.ObservationID(observation.RootID, observation.RelativePath)
		}
		game := games[id]
		if game == nil {
			platformID, platformName := platformFor(observation)
			title, raw := displayTitle(id, observation)
			game = &Game{
				ID: id, Title: title, RawTitle: raw,
				PlatformID: platformID, PlatformName: platformName,
				Identities: identityMap(id), NeedsAttention: attention,
			}
			games[id] = game
		} else {
			if game.Identities == nil {
				game.Identities = make(map[string]string)
			}
			for namespace, value := range identityMap(id) {
				game.Identities[namespace] = value
			}
		}
		role := observation.Media.Role
		if role == "" {
			role = "other"
		}
		asset := buildAsset(observation, hashCopies, hashProfiles)
		asset.Role = role
		game.Assets = append(game.Assets, asset)
	}

	result := Catalog{ScannedAt: snapshot.ScannedAt.UTC().Format("2006-01-02T15:04:05Z07:00")}
	platforms := make(map[string]*Platform)
	for _, game := range games {
		sort.Slice(game.Assets, func(i, j int) bool {
			if game.Assets[i].Role != game.Assets[j].Role {
				return game.Assets[i].Role < game.Assets[j].Role
			}
			return game.Assets[i].ID < game.Assets[j].ID
		})
		game.MissingRoles = missingRoles(game.Assets)
		game.Profiles = gameProfiles(*game, profiles)
		game.Fallbacks = fallbackExplanations(*game)
		if game.NeedsAttention {
			result.NeedsAttention++
		}

		result.Games = append(result.Games, *game)

		platform := platforms[game.PlatformID]
		if platform == nil {
			platform = &Platform{ID: game.PlatformID, Name: game.PlatformName}
			platforms[game.PlatformID] = platform
		}
		platform.GameCount++
		platform.ArtworkCount += len(game.Assets)
		if len(game.Assets) == 0 {
			platform.MissingArtCount++
		}
		for _, asset := range game.Assets {
			if len(platform.PreviewIDs) < 4 && isRepresentative(asset.Role) {
				platform.PreviewIDs = append(platform.PreviewIDs, asset.ID)
			}
		}
	}
	sort.Slice(result.Games, func(i, j int) bool {
		if result.Games[i].PlatformName != result.Games[j].PlatformName {
			return result.Games[i].PlatformName < result.Games[j].PlatformName
		}
		if result.Games[i].Title != result.Games[j].Title {
			return result.Games[i].Title < result.Games[j].Title
		}
		return result.Games[i].ID < result.Games[j].ID
	})
	for _, platform := range platforms {
		platform.Assets = platformAssets[platform.ID]
		for _, asset := range platform.Assets {
			if len(platform.PreviewIDs) < 4 {
				platform.PreviewIDs = append(platform.PreviewIDs, asset.ID)
			}
		}
		if platform.GameCount > 0 {
			platform.Coverage = (platform.GameCount - platform.MissingArtCount) * 100 / platform.GameCount
		}
		result.Platforms = append(result.Platforms, *platform)
	}
	sort.Slice(result.Platforms, func(i, j int) bool {
		if result.Platforms[i].Name != result.Platforms[j].Name {
			return result.Platforms[i].Name < result.Platforms[j].Name
		}
		return result.Platforms[i].ID < result.Platforms[j].ID
	})
	return result
}

func fallbackExplanations(game Game) []Fallback {
	missing := make(map[string]struct{}, len(game.MissingRoles))
	for _, role := range game.MissingRoles {
		missing[role] = struct{}{}
	}
	var definitions []Fallback
	switch {
	case game.Identities["steam"] != "":
		definitions = append(definitions, Fallback{
			Frontend: "Steam", Roles: presentRoles(missing, "grid", "portrait", "hero", "logo", "icon"),
			Message: "Steam uses its built-in artwork for these missing roles.",
		})
	case game.Identities["playnite"] != "":
		definitions = append(definitions, Fallback{
			Frontend: "Playnite", Roles: presentRoles(missing, "cover", "logo", "icon"),
			Message: "Playnite uses its default placeholder or theme fallback for these roles.",
		})
	case strings.HasPrefix(game.ID, "retro:"):
		definitions = append(definitions, Fallback{
			Frontend: "ES-DE", Roles: presentRoles(missing, "cover", "marquee", "screenshot", "miximage", "video"),
			Message: "ES-DE falls back to the active theme when these media roles are missing.",
		})
	}
	var result []Fallback
	for _, fallback := range definitions {
		if len(fallback.Roles) > 0 {
			result = append(result, fallback)
		}
	}
	return result
}

func presentRoles(missing map[string]struct{}, roles ...string) []string {
	var result []string
	for _, role := range roles {
		if _, ok := missing[role]; ok {
			result = append(result, role)
		}
	}
	return result
}

func profileGameIdentity(game model.ProfileGame) (string, string, string, string) {
	if steam := game.Identities["steam"]; steam != "" {
		return "steam:" + steam, "steam", "Steam", "Steam app " + steam
	}
	if playnite := game.Identities["playnite"]; playnite != "" {
		return "playnite:" + strings.ToLower(playnite), "playnite", "Playnite", "Playnite game " + shortID(playnite)
	}
	if game.Retro != nil && game.Retro.System != "" {
		id := game.ID
		if !strings.HasPrefix(id, "retro:") {
			id = "retro:" + game.Retro.System + ":" + game.Retro.Stem
		}
		return id, "retro:" + game.Retro.System, SystemName(game.Retro.System), CleanRetroTitle(game.Retro.Stem)
	}
	if strings.HasPrefix(game.ID, "steam:") {
		value := strings.TrimPrefix(game.ID, "steam:")
		return game.ID, "steam", "Steam", "Steam app " + value
	}
	return game.ID, "other", "Other", CleanRetroTitle(game.ID)
}

func buildAsset(observation model.Observation, hashCopies map[string]int, hashProfiles map[string]map[string]struct{}) Asset {
	return Asset{
		ID:           review.ObservationID(observation.RootID, observation.RelativePath),
		SHA256:       observation.SHA256,
		Role:         observation.Media.Role,
		Width:        observation.Media.Width,
		Height:       observation.Media.Height,
		Aspect:       aspect(observation.Media.Width, observation.Media.Height),
		MIME:         observation.Media.MIME,
		Extension:    observation.Media.Extension,
		Size:         observation.Size,
		SourceID:     observation.RootID,
		SourceName:   sourceName(observation.RootKind),
		Location:     observation.RootID + ":" + filepath.ToSlash(observation.RelativePath),
		SharedCopies: hashCopies[observation.SHA256],
		Profiles:     sortedSet(hashProfiles[observation.SHA256]),
	}
}

func platformAssetPlatform(observation model.Observation) (string, bool) {
	if observation.RootKind != "esde-media" || observation.System == "" {
		return "", false
	}
	stem := strings.ToLower(strings.TrimSuffix(filepath.Base(filepath.ToSlash(observation.RelativePath)), filepath.Ext(observation.RelativePath)))
	system := strings.ToLower(observation.System)
	switch stem {
	case system, "system", "platform", "console":
		return "retro:" + system, true
	default:
		return "", false
	}
}

func FindGame(catalog Catalog, id string) (Game, bool) {
	for _, game := range catalog.Games {
		if game.ID == id {
			return game, true
		}
	}
	return Game{}, false
}

func platformFor(observation model.Observation) (string, string) {
	if strings.HasPrefix(observation.IdentityHint, "retro:") {
		system := observation.System
		if system == "" {
			parts := strings.Split(observation.IdentityHint, ":")
			if len(parts) > 2 {
				system = parts[1]
			}
		}
		return "retro:" + system, SystemName(system)
	}
	if strings.HasPrefix(observation.IdentityHint, "steam:") || observation.RootKind == "steam-grid" || observation.RootKind == "decky-catalog" {
		return "steam", "Steam"
	}
	if strings.HasPrefix(observation.IdentityHint, "playnite:") {
		return "playnite", "Playnite"
	}
	if observation.System != "" {
		return "system:" + observation.System, SystemName(observation.System)
	}
	return "other", "Other"
}

func displayTitle(id string, observation model.Observation) (string, string) {
	if strings.HasPrefix(id, "retro:") {
		stem := strings.TrimSuffix(filepath.Base(filepath.ToSlash(observation.RelativePath)), filepath.Ext(observation.RelativePath))
		return CleanRetroTitle(stem), stem
	}
	if strings.HasPrefix(id, "steam:") {
		return "Steam app " + strings.TrimPrefix(id, "steam:"), ""
	}
	if strings.HasPrefix(id, "playnite:") {
		return "Playnite game " + shortID(strings.TrimPrefix(id, "playnite:")), ""
	}
	stem := strings.TrimSuffix(filepath.Base(filepath.ToSlash(observation.RelativePath)), filepath.Ext(observation.RelativePath))
	return CleanRetroTitle(stem), stem
}

func shortID(value string) string {
	if len(value) <= 8 {
		return value
	}
	return value[:8] + "…"
}

func identityMap(id string) map[string]string {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 || parts[0] == "unmapped" {
		return map[string]string{}
	}
	return map[string]string{parts[0]: parts[1]}
}

func aspect(width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}
	return fmt.Sprintf("%.2f:1", float64(width)/float64(height))
}

func sourceName(kind string) string {
	switch kind {
	case "steam-grid":
		return "Steam custom artwork"
	case "playnite-library":
		return "Playnite library"
	case "playnite-extra":
		return "Playnite ExtraMetadata"
	case "decky-catalog":
		return "Deck Gaming Profiles"
	case "esde-media":
		return "RetroDECK / ES-DE"
	case "romm":
		return "RomM"
	default:
		return SystemName(kind)
	}
}

func sortedSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func missingRoles(assets []Asset) []string {
	have := make(map[string]struct{}, len(assets))
	for _, asset := range assets {
		have[asset.Role] = struct{}{}
	}
	var result []string
	for _, role := range expectedRoles {
		if _, ok := have[role]; !ok {
			result = append(result, role)
		}
	}
	return result
}

func gameProfiles(game Game, profiles []model.Profile) []string {
	seen := make(map[string]struct{})
	for _, profile := range profiles {
		for _, profileGame := range profile.Games {
			if profileGame.ID == game.ID {
				seen[profile.Name] = struct{}{}
				break
			}
			for namespace, value := range profileGame.Identities {
				if game.Identities[namespace] == value {
					seen[profile.Name] = struct{}{}
					break
				}
			}
		}
	}
	return sortedSet(seen)
}

func isRepresentative(role string) bool {
	switch role {
	case "portrait", "cover", "grid", "miximage", "hero":
		return true
	default:
		return false
	}
}
