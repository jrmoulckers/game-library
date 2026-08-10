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

func TestResolveDetailedReportsWinningRule(t *testing.T) {
	observation := model.Observation{
		RootID: "retro", System: "n64", SHA256: "abc",
		Media: model.MediaFacts{Role: "video"},
	}
	file := model.PolicyFile{
		Version: 1,
		Default: "tracked-external",
		Rules: []model.PolicyRule{
			{Source: "retro", Mode: "managed"},
			{AssetSHA256: "abc", Mode: "quarantined"},
		},
	}
	mode, rule, matched := ResolveDetailed(file, observation)
	if mode != "quarantined" {
		t.Fatalf("mode = %q", mode)
	}
	if !matched || rule != 1 {
		t.Fatalf("expected rule index 1 matched, got rule=%d matched=%v", rule, matched)
	}
}

func TestResolveDetailedReportsNoMatchAgainstDefault(t *testing.T) {
	observation := model.Observation{RootID: "other"}
	file := model.PolicyFile{
		Version: 1,
		Default: "tracked-external",
		Rules:   []model.PolicyRule{{Source: "retro", Mode: "managed"}},
	}
	mode, rule, matched := ResolveDetailed(file, observation)
	if mode != "tracked-external" {
		t.Fatalf("mode = %q", mode)
	}
	if matched || rule != -1 {
		t.Fatalf("expected no rule matched, got rule=%d matched=%v", rule, matched)
	}
}
