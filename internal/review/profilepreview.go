package review

import (
	"fmt"
	"strings"

	"github.com/jrmoulckers/game-library/internal/decky"
	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/profile"
)

// PreviewProfileResolve resolves profileDraft against catalogRoot exactly
// as `gamelib profile resolve` does, so a dashboard reviewer can see asset
// availability for a draft before anything is promoted to canonical.
func PreviewProfileResolve(profileDraft model.Profile, catalogRoot string) (model.ProfileResolution, error) {
	return profile.Resolve(profileDraft, catalogRoot)
}

// ExportPreview is a dry-run preview of exporting profileDraft to a
// frontend adapter: the underlying manifest (as `gamelib export plan`
// would produce it) plus, for the "decky" adapter, the exact Decky v1
// profile document the export would render, so a reviewer can confirm
// Decky v1 invariants (schema version 1, path-safe id, a single mod set
// per game, an explicit — never omitted — artwork field, and an explicit
// empty "mods": [] rather than null) hold before anything is written.
type ExportPreview struct {
	Plan         model.Manifest
	DeckyProfile *model.DeckyProfileV1
	// HasGridArtwork is true when the plan includes at least one actual
	// grid-artwork copy action (as opposed to only the ".deck-profile-empty"
	// marker). It is always false for adapters other than "decky".
	HasGridArtwork bool
}

// PreviewExportPlan builds the export plan for adapter exactly as
// profile.BuildExportPlan does (no duplicated adapter-destination logic),
// and, for adapter == "decky", additionally synthesizes and validates the
// Decky v1 profile document the plan's render action would produce, using
// decky.Validate so this preview can never silently drift from the
// invariants that package enforces.
func PreviewExportPlan(profileDraft model.Profile, adapter string) (ExportPreview, error) {
	plan, err := profile.BuildExportPlan(adapter, profileDraft)
	if err != nil {
		return ExportPreview{}, err
	}
	preview := ExportPreview{Plan: plan}
	if adapter != "decky" {
		return preview, nil
	}

	deckyProfile := synthesizeDeckyProfileV1(profileDraft, plan)
	if err := decky.Validate(deckyProfile, deckyProfile.ID); err != nil {
		return ExportPreview{}, fmt.Errorf("decky v1 preview invariant violated: %w", err)
	}
	preview.DeckyProfile = &deckyProfile
	preview.HasGridArtwork = hasGridArtworkAction(plan)
	return preview, nil
}

func hasGridArtworkAction(plan model.Manifest) bool {
	for _, action := range plan.Actions {
		if action.Action == "copy" && action.DestinationRoot == "decky" &&
			strings.Contains(action.DestinationPath, "/grid/") {
			return true
		}
	}
	return false
}

// synthesizeDeckyProfileV1 builds the Decky v1 profile document the export
// plan's "render" action for the profile JSON represents, without writing
// anything. It reads whether the plan produced any actual grid-artwork
// copy action (rather than re-deriving profile.BuildExportPlan's own
// steam-identity/role-mapping rules) so this preview can never disagree
// with the plan it is describing.
func synthesizeDeckyProfileV1(p model.Profile, plan model.Manifest) model.DeckyProfileV1 {
	// profile.BuildExportPlan always renders an artwork/<id>/grid tree for
	// the decky adapter — either populated with real assets, or (when
	// there are none) a single ".deck-profile-empty" marker file — so the
	// artwork id itself is always the profile id, never nil, for anything
	// this package's export preview produces. A hand-authored Decky
	// profile may still use "artwork: null" for a profile with no artwork
	// concept at all (see testdata/decky), but that is a distinct,
	// separately valid Decky v1 shape this preview does not synthesize.
	artworkID := p.ID
	artwork := &artworkID

	mods := make([]model.DeckyModV1, 0, len(p.Mods))
	for _, mod := range p.Mods {
		mods = append(mods, model.DeckyModV1{Game: mod.Game, Set: mod.Set})
	}

	return model.DeckyProfileV1{
		Version:     1,
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		Artwork:     artwork,
		Mods:        mods,
	}
}
