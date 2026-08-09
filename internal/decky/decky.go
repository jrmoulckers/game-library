// Package decky reads and validates the frozen Decky v1 profile ABI, the
// generated legacy surface described by schemas/v1/decky-profile-v1.schema.json.
package decky

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jrmoulckers/game-library/internal/config"
	"github.com/jrmoulckers/game-library/internal/model"
)

func LoadAndValidate(path string) (model.DeckyProfileV1, error) {
	var profile model.DeckyProfileV1
	data, err := os.ReadFile(path)
	if err != nil {
		return profile, err
	}
	if err := json.Unmarshal(data, &profile); err != nil {
		return profile, err
	}
	if err := Validate(profile, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))); err != nil {
		return profile, err
	}
	return profile, nil
}

func ValidateCatalog(root string) error {
	profilesDir := filepath.Join(root, "profiles")
	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		return fmt.Errorf("read Decky profiles: %w", err)
	}
	var profilePaths []string
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".json" {
			continue
		}
		profilePaths = append(profilePaths, filepath.Join(profilesDir, entry.Name()))
	}
	sort.Strings(profilePaths)
	if len(profilePaths) == 0 {
		return fmt.Errorf("catalog has no Decky profile JSON files")
	}
	for _, profilePath := range profilePaths {
		profile, err := LoadAndValidate(profilePath)
		if err != nil {
			return err
		}
		if profile.Artwork != nil {
			grid := filepath.Join(root, "artwork", *profile.Artwork, "grid")
			if err := validatePayloadTree(grid, true); err != nil {
				return fmt.Errorf("profile %q artwork: %w", profile.ID, err)
			}
		}
		for _, mod := range profile.Mods {
			payload := filepath.Join(root, "mods", mod.Game, mod.Set)
			if err := validatePayloadTree(payload, false); err != nil {
				return fmt.Errorf("profile %q mod %s/%s: %w", profile.ID, mod.Game, mod.Set, err)
			}
		}
	}
	return nil
}

func Validate(profile model.DeckyProfileV1, filenameStem string) error {
	if profile.Version != 1 {
		return fmt.Errorf("decky profile version must be 1")
	}
	if !config.IsSafeID(profile.ID) {
		return fmt.Errorf("decky profile id %q is not path-safe", profile.ID)
	}
	if profile.ID != filenameStem {
		return fmt.Errorf("decky profile filename stem %q does not match id %q", filenameStem, profile.ID)
	}
	if strings.TrimSpace(profile.Name) == "" {
		return fmt.Errorf("decky profile name is required")
	}
	if profile.Artwork != nil && !config.IsSafeID(*profile.Artwork) {
		return fmt.Errorf("decky artwork id %q is not path-safe", *profile.Artwork)
	}
	seen := make(map[string]struct{})
	for _, mod := range profile.Mods {
		if !config.IsSafeID(mod.Game) || !config.IsSafeID(mod.Set) {
			return fmt.Errorf("decky mod game/set ids must be path-safe")
		}
		if _, ok := seen[mod.Game]; ok {
			return fmt.Errorf("decky profile contains more than one mod set for game %q", mod.Game)
		}
		seen[mod.Game] = struct{}{}
	}
	return nil
}

func validatePayloadTree(root string, allowEmptyMarker bool) error {
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("payload root is not a directory")
	}
	files := 0
	emptyMarker := false
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink is not allowed: %s", entry.Name())
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("non-regular file is not allowed: %s", entry.Name())
		}
		files++
		if filepath.Base(path) == ".deck-profile-empty" {
			emptyMarker = true
		}
		return nil
	})
	if err != nil {
		return err
	}
	if files == 0 {
		return fmt.Errorf("payload tree is empty")
	}
	if emptyMarker && (!allowEmptyMarker || files != 1) {
		return fmt.Errorf(".deck-profile-empty must be the only file in an intentional empty artwork set")
	}
	return nil
}
