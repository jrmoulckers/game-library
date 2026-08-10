package profile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jrmoulckers/game-library/internal/config"
	"github.com/jrmoulckers/game-library/internal/manifest"
	"github.com/jrmoulckers/game-library/internal/model"
)

var (
	fullSHA256 = regexp.MustCompile(`^[0-9a-f]{64}$`)
	uuid       = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	numericID  = regexp.MustCompile(`^[0-9]+$`)
	extension  = regexp.MustCompile(`^[a-z0-9]{1,8}$`)
)

var assetRoles = map[string]struct{}{
	"grid": {}, "portrait": {}, "hero": {}, "logo": {}, "icon": {},
	"cover": {}, "background": {}, "banner": {}, "marquee": {}, "fanart": {},
	"screenshot": {}, "titlescreen": {}, "miximage": {}, "physicalmedia": {},
	"manual": {}, "video": {}, "custom": {},
}

func Load(path string) (model.Profile, error) {
	var profile model.Profile
	data, err := os.ReadFile(path)
	if err != nil {
		return profile, err
	}
	if err := json.Unmarshal(data, &profile); err != nil {
		return profile, err
	}
	return profile, Validate(profile)
}

func Validate(profile model.Profile) error {
	if profile.Version != model.SchemaVersion {
		return fmt.Errorf("profile version must be %d", model.SchemaVersion)
	}
	if !config.IsSafeID(profile.ID) {
		return fmt.Errorf("profile id %q is not path-safe", profile.ID)
	}
	if strings.TrimSpace(profile.Name) == "" {
		return fmt.Errorf("profile name is required")
	}
	seenGames := make(map[string]struct{})
	for _, game := range profile.Games {
		if game.ID == "" {
			return fmt.Errorf("profile game id is required")
		}
		if _, ok := seenGames[game.ID]; ok {
			return fmt.Errorf("duplicate profile game %q", game.ID)
		}
		seenGames[game.ID] = struct{}{}
		if len(game.Assets) == 0 {
			return fmt.Errorf("profile game %q has no assets", game.ID)
		}
		if game.Retro != nil {
			if !config.IsSafeID(game.Retro.System) || !safeRelativePath(game.Retro.Stem) {
				return fmt.Errorf("profile game %q has an unsafe retro target", game.ID)
			}
		}
		for role, asset := range game.Assets {
			if _, ok := assetRoles[role]; !ok {
				return fmt.Errorf("profile game %q has unsupported asset role %q", game.ID, role)
			}
			if !fullSHA256.MatchString(asset.SHA256) {
				return fmt.Errorf("profile game %q role %q has invalid SHA-256", game.ID, role)
			}
			if !extension.MatchString(strings.ToLower(asset.Extension)) {
				return fmt.Errorf("profile game %q role %q has invalid extension", game.ID, role)
			}
		}
	}
	return nil
}

func Resolve(profile model.Profile, catalogRoot string) (model.ProfileResolution, error) {
	if err := Validate(profile); err != nil {
		return model.ProfileResolution{}, err
	}
	revision, err := revision(profile)
	if err != nil {
		return model.ProfileResolution{}, err
	}
	result := model.ProfileResolution{
		Version: model.SchemaVersion, ToolVersion: model.ToolVersion,
		ProfileID: profile.ID, Complete: true, Revision: revision,
	}
	for _, game := range sortedGames(profile.Games) {
		roles := sortedRoles(game.Assets)
		for _, role := range roles {
			asset := game.Assets[role]
			relative := canonicalAssetPath(asset)
			full := filepath.Join(catalogRoot, filepath.FromSlash(relative))
			available, issue := verifyAsset(full, asset.SHA256)
			result.Assets = append(result.Assets, model.ResolvedProfileAsset{
				GameID: game.ID, Role: role, SHA256: asset.SHA256,
				Extension:     strings.TrimPrefix(asset.Extension, "."),
				CanonicalPath: relative, Available: available,
			})
			if issue != "" {
				result.Complete = false
				result.Issues = append(result.Issues, model.ValidationIssue{
					Code: "asset-unavailable", RelativePath: relative, Message: issue,
				})
			}
		}
	}
	return result, nil
}

func BuildBundlePlan(profile model.Profile, catalogRoot string) (model.Manifest, model.ProfileResolution, error) {
	resolution, err := Resolve(profile, catalogRoot)
	if err != nil {
		return model.Manifest{}, resolution, err
	}
	var actions []model.Action
	for _, asset := range resolution.Assets {
		destination := filepath.ToSlash(filepath.Join(
			"bundles", profile.ID, resolution.Revision, "assets",
			safePathKey(asset.GameID), asset.Role+"."+asset.Extension,
		))
		action := model.Action{
			Action: "copy", SourceRoot: "catalog", SourcePath: asset.CanonicalPath,
			SourceSHA256: asset.SHA256, DestinationRoot: "catalog",
			DestinationPath: destination, ExpectedDestination: "absent-or-same-hash",
			Reason: "materialize retained profile bundle asset",
		}
		if !asset.Available {
			action.Action = "blocked"
			action.Reason = "canonical asset is unavailable; promotion is required before bundle build"
		}
		actions = append(actions, action)
	}
	actions = append(actions, model.Action{
		Action: "render", DestinationRoot: "catalog",
		DestinationPath:     filepath.ToSlash(filepath.Join("bundles", profile.ID, resolution.Revision, "bundle.lock.json")),
		ExpectedDestination: "absent-or-same-hash",
		Reason:              "write deterministic bundle lock after all assets verify",
		Metadata:            map[string]string{"profileId": profile.ID, "revision": resolution.Revision},
	})
	plan, err := manifest.NewPlan("bundle-plan", actions, "dry-run only; current.json is not updated by this plan")
	return plan, resolution, err
}

func BuildExportPlan(adapter string, profile model.Profile) (model.Manifest, error) {
	if err := Validate(profile); err != nil {
		return model.Manifest{}, err
	}
	var actions []model.Action
	for _, game := range sortedGames(profile.Games) {
		for _, role := range sortedRoles(game.Assets) {
			asset := game.Assets[role]
			destination, err := adapterDestination(adapter, profile.ID, game, role, asset)
			if err != nil {
				return model.Manifest{}, err
			}
			if destination == "" {
				continue
			}
			actions = append(actions, model.Action{
				Action: "copy", SourceRoot: "catalog", SourcePath: canonicalAssetPath(asset),
				SourceSHA256: asset.SHA256, DestinationRoot: adapter,
				DestinationPath: destination, ExpectedDestination: "hash-aware-snapshot-required",
				Reason:   "stage deterministic frontend export",
				Metadata: map[string]string{"gameId": game.ID, "role": role},
			})
		}
	}
	if adapter == "decky" {
		actions = append(actions, model.Action{
			Action: "render", DestinationRoot: "decky",
			DestinationPath:     filepath.ToSlash(filepath.Join("profiles", profile.ID+".json")),
			ExpectedDestination: "hash-aware-snapshot-required",
			Reason:              "render Decky v1 compatibility profile",
			Metadata:            map[string]string{"version": "1", "profileId": profile.ID},
		})
		if len(actions) == 1 {
			actions = append(actions, model.Action{
				Action: "render", DestinationRoot: "decky",
				DestinationPath:     filepath.ToSlash(filepath.Join("artwork", profile.ID, "grid", ".deck-profile-empty")),
				ExpectedDestination: "hash-aware-snapshot-required",
				Reason:              "represent an intentional empty artwork set",
			})
		}
	}
	if len(actions) == 0 {
		return model.Manifest{}, fmt.Errorf("profile has no assets supported by %s adapter", adapter)
	}
	return manifest.NewPlan(adapter+"-export-plan", actions,
		"dry-run/staging only; live frontend publication requires separate manifest approval")
}

func adapterDestination(adapter, profileID string, game model.ProfileGame, role string, asset model.AssetSelection) (string, error) {
	ext := strings.TrimPrefix(strings.ToLower(asset.Extension), ".")
	switch adapter {
	case "steam", "decky":
		appID := game.Identities["steam"]
		if !numericID.MatchString(appID) {
			return "", nil
		}
		suffixes := map[string]string{
			"grid": "", "portrait": "p", "hero": "_hero", "logo": "_logo", "icon": "_icon",
		}
		suffix, ok := suffixes[role]
		if !ok {
			return "", nil
		}
		if err := validateAdapterExtension(adapter, role, ext); err != nil {
			return "", err
		}
		name := appID + suffix + "." + ext
		if adapter == "decky" {
			return filepath.ToSlash(filepath.Join("artwork", profileID, "grid", name)), nil
		}
		return name, nil
	case "playnite":
		gameID := game.Identities["playnite"]
		if !uuid.MatchString(gameID) {
			return "", nil
		}
		names := map[string]string{"logo": "Logo.png", "video": "VideoTrailer.mp4"}
		name, ok := names[role]
		if !ok {
			return "", nil
		}
		if err := validateAdapterExtension(adapter, role, ext); err != nil {
			return "", err
		}
		return filepath.ToSlash(filepath.Join("games", strings.ToLower(gameID), name)), nil
	case "esde":
		if game.Retro == nil || game.Retro.System == "" || game.Retro.Stem == "" {
			return "", nil
		}
		directories := map[string]string{
			"cover": "covers", "fanart": "fanart", "manual": "manuals", "marquee": "marquees",
			"miximage": "miximages", "physicalmedia": "physicalmedia", "screenshot": "screenshots",
			"titlescreen": "titlescreens", "video": "videos",
		}
		dir, ok := directories[role]
		if !ok {
			return "", nil
		}
		if err := validateAdapterExtension(adapter, role, ext); err != nil {
			return "", err
		}
		return filepath.ToSlash(filepath.Join(game.Retro.System, dir, game.Retro.Stem+"."+ext)), nil
	case "romm":
		rommID := game.Identities["romm"]
		if !numericID.MatchString(rommID) {
			return "", nil
		}
		switch role {
		case "manual", "screenshot", "cover":
			if err := validateAdapterExtension(adapter, role, ext); err != nil {
				return "", err
			}
			return filepath.ToSlash(filepath.Join("api", "roms", rommID, role)), nil
		default:
			return "", nil
		}
	default:
		return "", fmt.Errorf("unsupported adapter %q", adapter)
	}
}

func canonicalAssetPath(asset model.AssetSelection) string {
	ext := strings.TrimPrefix(strings.ToLower(asset.Extension), ".")
	return filepath.ToSlash(filepath.Join(
		"library", "assets", "sha256", asset.SHA256[:2], asset.SHA256, "content."+ext,
	))
}

// verifyAsset opens path and hashes its content, reporting whether it
// matches expected. Any I/O failure other than "does not exist" is reduced
// to a generic message: the caller's Issues list — including from the
// dashboard's profile-resolve preview endpoint — must never carry a raw
// filesystem path (which os.PathError.Error() would otherwise embed)
// toward an API response.
func verifyAsset(path, expected string) (bool, string) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, "managed content is absent"
		}
		return false, "managed content could not be read"
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false, "managed content could not be read"
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != expected {
		return false, "managed content SHA-256 does not match profile lock"
	}
	return true, ""
}

func revision(profile model.Profile) (string, error) {
	data, err := json.Marshal(profile)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func sortedGames(games []model.ProfileGame) []model.ProfileGame {
	result := append([]model.ProfileGame(nil), games...)
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func sortedRoles(assets map[string]model.AssetSelection) []string {
	roles := make([]string, 0, len(assets))
	for role := range assets {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	return roles
}

func safePathKey(value string) string {
	var result strings.Builder
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || strings.ContainsRune("._-", r) {
			result.WriteRune(r)
		} else {
			result.WriteByte('-')
		}
	}
	base := strings.Trim(result.String(), "-")
	if base == "" {
		base = "item"
	}
	sum := sha256.Sum256([]byte(value))
	return base + "--" + hex.EncodeToString(sum[:6])
}

func safeRelativePath(value string) bool {
	if value == "" || strings.ContainsAny(value, "\\\x00") ||
		filepath.IsAbs(value) || filepath.VolumeName(value) != "" ||
		(len(value) >= 2 && value[1] == ':') {
		return false
	}
	clean := path.Clean(value)
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func validateAdapterExtension(adapter, role, ext string) error {
	allowed := false
	switch adapter {
	case "steam", "decky":
		if role == "icon" {
			allowed = ext == "png" || ext == "jpg" || ext == "jpeg" || ext == "ico"
		} else {
			allowed = ext == "png" || ext == "jpg" || ext == "jpeg"
		}
	case "playnite":
		allowed = (role == "logo" && ext == "png") || (role == "video" && ext == "mp4")
	case "esde":
		switch role {
		case "manual":
			allowed = ext == "pdf"
		case "video":
			allowed = ext == "mp4" || ext == "mkv" || ext == "avi" || ext == "mov" || ext == "webm"
		default:
			allowed = ext == "png" || ext == "jpg" || ext == "jpeg" || ext == "webp"
		}
	case "romm":
		if role == "manual" {
			allowed = ext == "pdf"
		} else {
			allowed = ext == "png" || ext == "jpg" || ext == "jpeg" || ext == "webp"
		}
	}
	if !allowed {
		return fmt.Errorf("%s adapter does not support .%s for role %s; create a derived canonical asset first", adapter, ext, role)
	}
	return nil
}
