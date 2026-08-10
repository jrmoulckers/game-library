package review

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/jrmoulckers/game-library/internal/manifest"
	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/profile"
	"github.com/jrmoulckers/game-library/internal/workspace"
)

// PersistPlan writes plan as an immutable, create-if-absent workspace
// artifact under "plans/<kind>/<operationId>.json". Because
// manifest.BuildImport/profile.BuildBundlePlan/profile.BuildExportPlan are
// deterministic (same inputs always produce the same OperationID and
// content), calling this twice for the same logical plan is idempotent
// (created=false, no error); calling it for a plan whose OperationID
// collides with a previously persisted plan of different content is
// rejected with workspace.ErrConflict.
func PersistPlan(paths workspace.Paths, plan model.Manifest) (created bool, path string, err error) {
	name := "plans/" + plan.Kind + "/" + plan.OperationID + ".json"
	return workspace.WriteArtifact(paths, name, plan)
}

// BuildAndPersistImportPlan builds the same import plan `gamelib import
// plan` would (via manifest.BuildImport) and persists it as an immutable
// workspace artifact.
func BuildAndPersistImportPlan(paths workspace.Paths, inventory model.Inventory, policyFile model.PolicyFile) (plan model.Manifest, created bool, path string, err error) {
	plan, err = manifest.BuildImport(inventory, policyFile)
	if err != nil {
		return model.Manifest{}, false, "", err
	}
	created, path, err = PersistPlan(paths, plan)
	return plan, created, path, err
}

// BuildAndPersistBundlePlan builds the same bundle plan `gamelib bundle
// plan` would (via profile.BuildBundlePlan) and persists it as an
// immutable workspace artifact.
func BuildAndPersistBundlePlan(paths workspace.Paths, profileDraft model.Profile, catalogRoot string) (plan model.Manifest, resolution model.ProfileResolution, created bool, path string, err error) {
	plan, resolution, err = profile.BuildBundlePlan(profileDraft, catalogRoot)
	if err != nil {
		return model.Manifest{}, model.ProfileResolution{}, false, "", err
	}
	created, path, err = PersistPlan(paths, plan)
	return plan, resolution, created, path, err
}

// BuildAndPersistExportPlan builds the same export plan `gamelib export
// plan` would (via profile.BuildExportPlan) and persists it as an
// immutable workspace artifact.
func BuildAndPersistExportPlan(paths workspace.Paths, profileDraft model.Profile, adapter string) (plan model.Manifest, created bool, path string, err error) {
	plan, err = profile.BuildExportPlan(adapter, profileDraft)
	if err != nil {
		return model.Manifest{}, false, "", err
	}
	created, path, err = PersistPlan(paths, plan)
	return plan, created, path, err
}

// PersistedPlanDigestExists reports whether any plan artifact previously
// persisted under paths.Artifacts/"plans" has a manifest.Digest equal to
// digest. This is the integrity link Gate C review requires (see
// CreateGateCReview): an analysis can only be approved once it is tied to a
// manifest this workspace actually planned and recorded via one of the
// plan-building endpoints, never an arbitrary hypothetical manifest a
// caller invented only for the gate review request.
func PersistedPlanDigestExists(paths workspace.Paths, digest string) (bool, error) {
	if digest == "" {
		return false, nil
	}
	root := filepath.Join(paths.Artifacts, "plans")
	found := false
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return workspace.SanitizeFSError(err)
		}
		if d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return workspace.SanitizeFSError(readErr)
		}
		var plan model.Manifest
		if err := json.Unmarshal(data, &plan); err != nil {
			// Skip files this package did not write rather than failing
			// the whole scan over one unreadable entry.
			return nil
		}
		got, digestErr := manifest.Digest(plan)
		if digestErr != nil {
			return nil
		}
		if got == digest {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, fs.ErrNotExist) {
		return false, fmt.Errorf("scan persisted plan artifacts: %w", walkErr)
	}
	return found, nil
}
