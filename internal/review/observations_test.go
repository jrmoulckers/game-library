package review

import (
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
)

func sampleObservations() []model.Observation {
	return []model.Observation{
		{
			RootID: "steam", RootKind: "steam-grid", RelativePath: "123.png",
			SHA256: sha256Like(0), Size: 10, System: "",
			Media:        model.MediaFacts{Extension: "png", MIME: "image/png", Role: "grid", Width: 600, Height: 900},
			IdentityHint: "steam:123",
		},
		{
			RootID: "retro", RootKind: "esde-media", RelativePath: "n64/covers/game.jpg",
			SHA256: sha256Like(1), Size: 20, System: "n64",
			Media:        model.MediaFacts{Extension: "jpg", MIME: "image/jpeg", Role: "cover", Width: 300, Height: 300},
			IdentityHint: "retro:n64:game",
		},
		{
			RootID: "steam", RootKind: "steam-grid", RelativePath: "456p.png",
			SHA256: sha256Like(2), Size: 30, System: "",
			Media:        model.MediaFacts{Extension: "png", MIME: "image/png", Role: "portrait", Width: 600, Height: 900},
			IdentityHint: "steam:456",
		},
	}
}

func sampleSnapshot() Snapshot {
	observations := sampleObservations()
	index := make(map[string]model.Observation, len(observations))
	for _, o := range observations {
		index[ObservationID(o.RootID, o.RelativePath)] = o
	}
	return Snapshot{
		Inventory: model.Inventory{
			Privacy:      "private",
			Observations: observations,
			Issues: []model.ValidationIssue{
				{RootID: "retro", RelativePath: "n64/covers/game.jpg", Code: "media-type-mismatch", Message: "bad"},
			},
		},
		index: index,
	}
}

func TestListObservationsReturnsAllWithoutFilter(t *testing.T) {
	page := ListObservations(sampleSnapshot(), model.PolicyFile{}, nil, ObservationFilter{}, 0, 0)
	if page.Total != 3 {
		t.Fatalf("Total = %d, want 3", page.Total)
	}
	if page.Page != 1 || page.PageSize != 50 {
		t.Fatalf("expected default pagination, got page=%d pageSize=%d", page.Page, page.PageSize)
	}
	if len(page.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(page.Items))
	}
	// Deterministic ordering: rootID then relativePath.
	if page.Items[0].Observation.RootID != "retro" {
		t.Fatalf("expected retro root first, got %q", page.Items[0].Observation.RootID)
	}
}

func TestListObservationsFiltersBySource(t *testing.T) {
	page := ListObservations(sampleSnapshot(), model.PolicyFile{}, nil, ObservationFilter{Source: "steam"}, 1, 50)
	if page.Total != 2 {
		t.Fatalf("Total = %d, want 2", page.Total)
	}
	for _, item := range page.Items {
		if item.Observation.RootID != "steam" {
			t.Fatalf("unexpected root in filtered results: %q", item.Observation.RootID)
		}
	}
}

func TestListObservationsFiltersBySystem(t *testing.T) {
	page := ListObservations(sampleSnapshot(), model.PolicyFile{}, nil, ObservationFilter{System: "n64"}, 1, 50)
	if page.Total != 1 {
		t.Fatalf("Total = %d, want 1", page.Total)
	}
}

func TestListObservationsFiltersByIdentityPrefix(t *testing.T) {
	page := ListObservations(sampleSnapshot(), model.PolicyFile{}, nil, ObservationFilter{Identity: "steam"}, 1, 50)
	if page.Total != 2 {
		t.Fatalf("Total = %d, want 2", page.Total)
	}
}

func TestListObservationsFiltersByRole(t *testing.T) {
	page := ListObservations(sampleSnapshot(), model.PolicyFile{}, nil, ObservationFilter{Role: "portrait"}, 1, 50)
	if page.Total != 1 {
		t.Fatalf("Total = %d, want 1", page.Total)
	}
}

func TestListObservationsFiltersByDimensions(t *testing.T) {
	page := ListObservations(sampleSnapshot(), model.PolicyFile{}, nil, ObservationFilter{Dimensions: "300x300"}, 1, 50)
	if page.Total != 1 {
		t.Fatalf("Total = %d, want 1", page.Total)
	}
}

func TestListObservationsFiltersByPolicyOutcome(t *testing.T) {
	policyFile := model.PolicyFile{
		Version: model.SchemaVersion, Default: "tracked-external",
		Rules: []model.PolicyRule{{Source: "steam", Mode: "managed"}},
	}
	page := ListObservations(sampleSnapshot(), policyFile, nil, ObservationFilter{PolicyOutcome: "managed"}, 1, 50)
	if page.Total != 2 {
		t.Fatalf("Total = %d, want 2", page.Total)
	}
	for _, item := range page.Items {
		if item.PolicyMode != "managed" || !item.PolicyRuleMatched {
			t.Fatalf("expected matched managed rule, got %+v", item)
		}
	}
}

func TestListObservationsPolicyOutcomeFilterRequiresPolicy(t *testing.T) {
	// With no policy supplied (zero value), no observation can be said to
	// resolve to any particular outcome, so a policy-outcome filter must
	// exclude everything rather than guessing.
	page := ListObservations(sampleSnapshot(), model.PolicyFile{}, nil, ObservationFilter{PolicyOutcome: "managed"}, 1, 50)
	if page.Total != 0 {
		t.Fatalf("Total = %d, want 0", page.Total)
	}
}

func TestListObservationsFiltersByTheme(t *testing.T) {
	profiles := []model.Profile{{
		Version: model.SchemaVersion, ID: "p1", Name: "P1", Theme: "retro-neon",
		Games: []model.ProfileGame{{ID: "g1", Assets: map[string]model.AssetSelection{
			"grid": {SHA256: sha256Like(0), Extension: "png"},
		}}},
	}}
	page := ListObservations(sampleSnapshot(), model.PolicyFile{}, profiles, ObservationFilter{Theme: "retro-neon"}, 1, 50)
	if page.Total != 1 {
		t.Fatalf("Total = %d, want 1", page.Total)
	}
	if len(page.Items[0].Themes) != 1 || page.Items[0].Themes[0] != "retro-neon" {
		t.Fatalf("expected theme membership attached, got %+v", page.Items[0].Themes)
	}
}

func TestListObservationsFiltersByValidation(t *testing.T) {
	issues := ListObservations(sampleSnapshot(), model.PolicyFile{}, nil, ObservationFilter{Validation: "issues"}, 1, 50)
	if issues.Total != 1 {
		t.Fatalf("issues Total = %d, want 1", issues.Total)
	}
	clean := ListObservations(sampleSnapshot(), model.PolicyFile{}, nil, ObservationFilter{Validation: "clean"}, 1, 50)
	if clean.Total != 2 {
		t.Fatalf("clean Total = %d, want 2", clean.Total)
	}
}

func TestListObservationsCombinesFiltersWithAND(t *testing.T) {
	page := ListObservations(sampleSnapshot(), model.PolicyFile{}, nil, ObservationFilter{Source: "steam", Role: "grid"}, 1, 50)
	if page.Total != 1 {
		t.Fatalf("Total = %d, want 1", page.Total)
	}
}

func TestListObservationsPaginates(t *testing.T) {
	first := ListObservations(sampleSnapshot(), model.PolicyFile{}, nil, ObservationFilter{}, 1, 2)
	if len(first.Items) != 2 || first.Total != 3 {
		t.Fatalf("first page = %+v", first)
	}
	second := ListObservations(sampleSnapshot(), model.PolicyFile{}, nil, ObservationFilter{}, 2, 2)
	if len(second.Items) != 1 || second.Total != 3 {
		t.Fatalf("second page = %+v", second)
	}
	third := ListObservations(sampleSnapshot(), model.PolicyFile{}, nil, ObservationFilter{}, 3, 2)
	if len(third.Items) != 0 {
		t.Fatalf("expected an empty third page, got %+v", third)
	}
}

func TestListObservationsNegativePageAndPageSizeDefault(t *testing.T) {
	page := ListObservations(sampleSnapshot(), model.PolicyFile{}, nil, ObservationFilter{}, -1, -1)
	if page.Page != 1 || page.PageSize != 50 {
		t.Fatalf("expected defaults for invalid page/pageSize, got page=%d pageSize=%d", page.Page, page.PageSize)
	}
}
