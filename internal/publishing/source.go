package publishing

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jrmoulckers/game-library/internal/inventory"
	"github.com/jrmoulckers/game-library/internal/manifest"
	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/profile"
	"github.com/jrmoulckers/game-library/internal/topology"
	"github.com/jrmoulckers/game-library/internal/workspace"
)

// Source is a validated adapter plan and the local roots used to resolve its
// symbolic paths. Live frontend sources are deliberately ineligible.
type Source struct {
	Key   string
	Plan  model.Manifest
	Roots map[string]string
}

// FromProfile reuses canonical adapter naming without introducing any mapping.
func FromProfile(key string, p model.Profile, catalog, adapter string) (Source, error) {
	plan, err := profile.BuildExportPlan(adapter, p)
	return Source{Key: key, Plan: plan, Roots: map[string]string{"catalog": catalog}}, err
}

// FromArtwork exports an existing legacy Steam set without changing its Decky
// profile or copying it into canonical storage. The empty marker is never art.
func FromArtwork(cfg model.Config, declared topology.Profile) (Source, error) {
	if declared.Platform != "steam" || declared.Artwork == "" {
		return Source{}, fmt.Errorf("publishing currently requires a bound Steam artwork set")
	}
	var root model.Root
	for _, candidate := range cfg.Roots {
		if candidate.Kind == "decky-catalog" {
			if root.ID != "" {
				return Source{}, fmt.Errorf("multiple catalogs require explicit source selection")
			}
			root = candidate
		}
	}
	if root.ID == "" {
		return Source{}, fmt.Errorf("no catalog source configured")
	}
	setPath := "artwork/" + declared.Artwork + "/grid"
	dir, err := safePath(root.Path, setPath, false)
	if err != nil {
		return Source{}, fmt.Errorf("artwork set unavailable: %w", err)
	}
	scanned, err := inventory.Scan([]model.Root{{ID: root.ID, Kind: "steam-grid", Path: dir}})
	if err != nil {
		return Source{}, workspace.SanitizeFSError(err)
	}
	blocked := map[string]string{}
	for _, issue := range scanned.Issues {
		if issue.Code != "media-type-mismatch" && issue.Code != "role-media-mismatch" {
			return Source{}, fmt.Errorf("artwork set scan has unsafe entries; resolve before export")
		}
		blocked[issue.RelativePath] = issue.Message
	}
	var actions []model.Action
	roles := map[string]bool{"grid": true, "portrait": true, "hero": true, "logo": true, "icon": true}
	for _, observation := range scanned.Observations {
		if filepath.Base(observation.RelativePath) == ".deck-profile-empty" {
			continue
		}
		if filepath.Base(observation.RelativePath) != observation.RelativePath {
			return Source{}, fmt.Errorf("steam artwork must use flat grid filenames")
		}
		if !strings.HasPrefix(observation.IdentityHint, "steam:") || !roles[observation.Media.Role] {
			actions = append(actions, model.Action{
				Action: "blocked", SourceRoot: root.ID, SourcePath: setPath + "/" + observation.RelativePath,
				SourceSHA256: observation.SHA256, SourceSize: observation.Size,
				DestinationRoot: "steam", DestinationPath: observation.RelativePath,
				Reason:   "unsupported or unidentified file; the artwork adapter does not copy layout metadata",
				Metadata: map[string]string{"gameId": observation.IdentityHint, "role": observation.Media.Role},
			})
			continue
		}
		p := model.Profile{Version: 1, ID: declared.Artwork, Name: declared.Name, Games: []model.ProfileGame{{
			ID:         observation.IdentityHint,
			Identities: map[string]string{"steam": strings.TrimPrefix(observation.IdentityHint, "steam:")},
			Assets: map[string]model.AssetSelection{observation.Media.Role: {
				SHA256: observation.SHA256, Extension: observation.Media.Extension,
			}},
		}}}
		adapterPlan, err := profile.BuildExportPlan("steam", p)
		if err != nil {
			// Preserve the exact unsupported file in the parity preview. It
			// is not converted, renamed or copied by this publisher.
			adapterPlan.Actions = []model.Action{{
				Action: "blocked", DestinationRoot: "steam", DestinationPath: observation.RelativePath,
				SourceSHA256: observation.SHA256, Reason: err.Error(),
				Metadata: map[string]string{"gameId": observation.IdentityHint, "role": observation.Media.Role},
			}}
		}
		for _, action := range adapterPlan.Actions {
			action.SourceRoot = root.ID
			action.SourcePath = setPath + "/" + observation.RelativePath
			action.SourceSize = observation.Size
			if reason := blocked[observation.RelativePath]; reason != "" {
				action.Action, action.Reason = "blocked", reason
			}
			actions = append(actions, action)
		}
	}
	if len(actions) == 0 {
		return Source{}, fmt.Errorf("profile contains no custom artwork; built-in Steam art is not a copy export")
	}
	variants := map[string][]int{}
	for index, action := range actions {
		if roles[action.Metadata["role"]] {
			key := strings.ToLower(strings.TrimSuffix(action.DestinationPath, filepath.Ext(action.DestinationPath)))
			variants[key] = append(variants[key], index)
		}
	}
	for _, indices := range variants {
		if len(indices) < 2 {
			continue
		}
		for _, index := range indices {
			actions[index].Action = "blocked"
			actions[index].Reason = "multiple filename variants for one Steam artwork role; choose an explicit asset before publishing"
		}
	}
	plan, err := manifest.NewPlan("steam-export-plan", actions, "copy-only; built-in artwork and empty markers are not published")
	return Source{Key: declared.Key(), Plan: plan, Roots: map[string]string{root.ID: root.Path}}, err
}
