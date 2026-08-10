package review

import (
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
)

func TestBuildIdentityViewAttachesObservationIDs(t *testing.T) {
	snapshot := Snapshot{
		Inventory: model.Inventory{
			Observations: []model.Observation{
				{RootID: "steam", RelativePath: "123.png", IdentityHint: "steam:123"},
				{RootID: "unknown", RelativePath: "mystery.png"},
			},
		},
	}
	view := BuildIdentityView(snapshot)
	if len(view.Proposals) != 1 {
		t.Fatalf("expected 1 proposal, got %d", len(view.Proposals))
	}
	if view.Proposals[0].ID != ObservationID("steam", "123.png") {
		t.Fatalf("proposal ID mismatch")
	}
	if len(view.Unmapped) != 1 {
		t.Fatalf("expected 1 unmapped item, got %d", len(view.Unmapped))
	}
	if view.Unmapped[0].ID != ObservationID("unknown", "mystery.png") {
		t.Fatalf("unmapped ID mismatch")
	}
}
