package organizer

import (
	"testing"

	"github.com/jrmoulckers/game-library/internal/metadata"
	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/review"
)

func TestExactMetadataAndIdenticalBytesDoNotShareProfileMembership(t *testing.T) {
	snapshot := review.Snapshot{Inventory: model.Inventory{Observations: []model.Observation{
		observation("steam", "steam-grid", "42.png", "steam:42", "", "grid", "same-bytes"),
		observation("playnite", "playnite-library", "synthetic.png", "playnite:synthetic", "", "cover", "same-bytes"),
	}}}
	profiles := []model.Profile{
		{Name: "Steam only", Games: []model.ProfileGame{{ID: "steam:42",
			Identities: map[string]string{"steam": "42", "playnite": "synthetic"},
			Assets:     map[string]model.AssetSelection{"grid": {SHA256: "same-bytes"}}}}},
		{Name: "Playnite only", Games: []model.ProfileGame{{ID: "playnite:synthetic",
			Identities: map[string]string{"steam": "42", "playnite": "synthetic"},
			Assets:     map[string]model.AssetSelection{"cover": {SHA256: "same-bytes"}}}}},
	}
	titles := metadata.NewBuilder()
	titles.AddAlias("playnite:synthetic", "steam:42")
	catalog := BuildWithMetadata(snapshot, profiles, titles.Build())
	if len(catalog.Games) != 2 {
		t.Fatal("exact metadata aliases merged platform artwork")
	}
	for _, game := range catalog.Games {
		name, frontend := "Steam only", "Steam"
		if game.PlatformID == "playnite" {
			name, frontend = "Playnite only", "Playnite"
		}
		if len(game.Profiles) != 1 || game.Profiles[0] != name ||
			len(game.Assets) != 1 || len(game.Assets[0].Profiles) != 1 || game.Assets[0].Profiles[0] != name {
			t.Fatalf("cross-platform profile membership: %+v", game)
		}
		if len(game.Fallbacks) != 1 || game.Fallbacks[0].Frontend != frontend {
			t.Fatalf("cross-platform fallback: %+v", game.Fallbacks)
		}
	}
}
