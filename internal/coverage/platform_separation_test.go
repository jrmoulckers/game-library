package coverage

import (
	"testing"

	"github.com/jrmoulckers/game-library/internal/organizer"
)

func TestWrongPlatformBindingDoesNotLeakEitherCoverageDirection(t *testing.T) {
	doc := testDoc()
	doc.Profiles[4].Artwork = "deck-default"
	catalog := organizer.Catalog{Games: []organizer.Game{
		{ID: "steam:42", PlatformID: "steam", Assets: []organizer.Asset{asset("deck-default", "grid")}},
		{ID: "playnite:synthetic", PlatformID: "playnite"},
	}}
	report := Build(catalog, doc)
	for _, profile := range report.Profiles {
		if profile.PlatformID == "playnite" && (profile.GameCount != 0 || profile.AssetCount != 0 || !profile.Empty) {
			t.Fatalf("wrong-platform set leaked profile-facing coverage: %+v", profile)
		}
	}
	for _, game := range report.Games {
		if game.PlatformID == "playnite" && game.CoveredCount != 0 {
			t.Fatalf("wrong-platform set leaked game-facing coverage: %+v", game)
		}
	}
}
