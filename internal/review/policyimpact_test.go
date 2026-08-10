package review

import (
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
)

func TestBuildPolicyImpactReportsModesAndWinningRule(t *testing.T) {
	snapshot := Snapshot{Inventory: model.Inventory{Observations: []model.Observation{
		{RootID: "steam", RelativePath: "1.png", SHA256: sha256Like(0)},
		{RootID: "retro", RelativePath: "2.png", SHA256: sha256Like(1)},
	}}}
	policyFile := model.PolicyFile{
		Version: model.SchemaVersion, Default: "tracked-external",
		Rules: []model.PolicyRule{{Source: "steam", Mode: "managed"}},
	}
	view, err := BuildPolicyImpact(snapshot, policyFile)
	if err != nil {
		t.Fatal(err)
	}
	if view.Counts["managed"] != 1 || view.Counts["tracked-external"] != 1 {
		t.Fatalf("unexpected counts: %+v", view.Counts)
	}
	var steamEntry PolicyImpactEntry
	for _, e := range view.Entries {
		if e.RootID == "steam" {
			steamEntry = e
		}
	}
	if steamEntry.Mode != "managed" || !steamEntry.MatchedRule || steamEntry.MatchedRuleIndex != 0 {
		t.Fatalf("unexpected steam entry: %+v", steamEntry)
	}
}

func TestBuildPolicyImpactRejectsInvalidPolicy(t *testing.T) {
	snapshot := Snapshot{}
	invalid := model.PolicyFile{Version: model.SchemaVersion, Default: "not-a-mode"}
	if _, err := BuildPolicyImpact(snapshot, invalid); err == nil {
		t.Fatal("expected an error for an invalid policy")
	}
}
