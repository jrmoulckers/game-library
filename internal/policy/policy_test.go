package policy

import (
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
)

func TestResolvePrecedence(t *testing.T) {
	observation := model.Observation{
		RootID: "retro", System: "n64", SHA256: "abc",
		Media: model.MediaFacts{Role: "video"},
	}
	file := model.PolicyFile{
		Version: 1,
		Default: "tracked-external",
		Rules: []model.PolicyRule{
			{Source: "retro", Mode: "managed"},
			{System: "n64", Mode: "promote-on-approval"},
			{Role: "video", Mode: "tracked-external"},
			{AssetSHA256: "abc", Mode: "quarantined"},
		},
	}
	if err := Validate(file); err != nil {
		t.Fatal(err)
	}
	if actual := Resolve(file, observation); actual != "quarantined" {
		t.Fatalf("Resolve() = %q", actual)
	}
}

func TestDuplicateSelectorRejected(t *testing.T) {
	file := model.PolicyFile{
		Version: 1, Default: "tracked-external",
		Rules: []model.PolicyRule{
			{System: "n64", Mode: "managed"},
			{System: "n64", Mode: "quarantined"},
		},
	}
	if err := Validate(file); err == nil {
		t.Fatal("expected duplicate selector error")
	}
}
