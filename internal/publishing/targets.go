// Package publishing stages copy-first exports and explicitly approved local
// Steam artwork copies. It never promotes generated content into a catalog.
package publishing

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jrmoulckers/game-library/internal/config"
	"github.com/jrmoulckers/game-library/internal/topology"
	"github.com/jrmoulckers/game-library/internal/workspace"
)

// Target describes an actual frontend separately from hardware applicability.
// An empty root is unobserved; configured unavailable roots are offline.
// Root paths exist only in private, host-local configuration and API previews.
type Target struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Device   string `json:"device"`
	Platform string `json:"platform"`
	Adapter  string `json:"adapter"`
	Root     string `json:"root,omitempty"`
}

type Targets struct {
	Version int      `json:"version"`
	Targets []Target `json:"targets"`
}

func TargetPath(paths workspace.Paths) string {
	return filepath.Join(paths.Root, "config", "targets.json")
}

func LoadTargets(paths workspace.Paths, doc topology.Document) (Targets, error) {
	data, err := os.ReadFile(TargetPath(paths))
	if errors.Is(err, os.ErrNotExist) {
		result := Targets{Version: 1, Targets: []Target{}}
		for _, device := range doc.Devices {
			for _, platform := range device.Platforms {
				adapter := ""
				if platform == "steam" {
					adapter = "steam"
				}
				result.Targets = append(result.Targets, Target{
					ID: device.ID + "-" + platform, Name: device.Name + " · " + doc.PlatformName(platform),
					Device: device.ID, Platform: platform, Adapter: adapter,
				})
			}
		}
		return result, nil
	}
	if err != nil {
		return Targets{}, workspace.SanitizeFSError(err)
	}
	var result Targets
	if err := json.Unmarshal(data, &result); err != nil {
		return Targets{}, fmt.Errorf("decode frontend targets: %w", err)
	}
	return result, ValidateTargets(result, doc)
}

func ValidateTargets(targets Targets, doc topology.Document) error {
	if targets.Version != 1 {
		return fmt.Errorf("targets version must be 1")
	}
	seen := map[string]bool{}
	for _, target := range targets.Targets {
		if !config.IsSafeID(target.ID) || target.Name == "" || seen[target.ID] {
			return fmt.Errorf("targets need unique path-safe IDs and names")
		}
		seen[target.ID] = true
		applicable := false
		for _, device := range doc.DevicesFor(target.Platform) {
			applicable = applicable || device.ID == target.Device
		}
		if !applicable {
			return fmt.Errorf("target %s does not match a declared device/platform", target.ID)
		}
		if target.Adapter == "steam" && target.Platform != "steam" {
			return fmt.Errorf("steam adapter requires the Steam platform")
		}
		if target.Root != "" && !filepath.IsAbs(target.Root) {
			return fmt.Errorf("target root must be a host-local absolute directory")
		}
	}
	return nil
}
