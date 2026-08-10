package organizer

import (
	"testing"
	"time"

	"github.com/jrmoulckers/game-library/internal/metadata"
	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/review"
)

func TestSystemNameAndRetroTitle(t *testing.T) {
	tests := map[string]string{
		"n64": "Nintendo 64", "gc": "Nintendo GameCube",
		"nds": "Nintendo DS", "switch": "Nintendo Switch",
		"pc_engine": "Pc Engine",
	}

	for input, want := range tests {
		if got := SystemName(input); got != want {
			t.Errorf("SystemName(%q) = %q, want %q", input, got, want)
		}

	}
	if got := CleanRetroTitle("Synthetic Quest (Region) [Revision]"); got != "Synthetic Quest" {
		t.Fatalf("CleanRetroTitle = %q", got)
	}
}

func TestBuildUsesResolvedTitleAndExactAlias(t *testing.T) {
	snapshot := review.Snapshot{Inventory: model.Inventory{Observations: []model.Observation{
		observation("steam", "steam-grid", "123.png", "steam:123", "", "grid", "steam"),
		observation("playnite", "playnite-library", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa.png", "playnite:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "", "cover", "playnite"),
	}}}
	builder := metadata.NewBuilder()
	builder.AddTitle("steam:123", "Synthetic Resolved Game", "steam-appinfo")
	builder.AddTitle("playnite:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "Synthetic Library Name", "playnite")
	builder.AddAlias("playnite:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "steam:123")
	catalog := BuildWithMetadata(snapshot, nil, builder.Build())
	if len(catalog.Games) != 1 {
		t.Fatalf("games = %#v", catalog.Games)
	}

	game := catalog.Games[0]
	if game.ID != "steam:123" || game.Title != "Synthetic Resolved Game" || len(game.Assets) != 2 {
		t.Fatalf("merged game = %+v", game)
	}

	if game.Identities["playnite"] == "" || game.Identities["steam"] != "123" {
		t.Fatalf("identities = %#v", game.Identities)
	}

}

func TestBuildKeepsLossyRetroStemsSeparate(t *testing.T) {
	snapshot := review.Snapshot{Inventory: model.Inventory{Observations: []model.Observation{
		observation("retro", "esde-media", "n64/covers/A-B.png", "retro:n64:a-b", "n64", "cover", "one"),
		observation("retro", "esde-media", "n64/covers/A B.png", "retro:n64:a-b", "n64", "cover", "two"),
	}}}
	catalog := Build(snapshot, nil)
	if len(catalog.Games) != 2 {
		t.Fatalf("retro games were merged: %#v", catalog.Games)
	}

	if catalog.Games[0].ID == catalog.Games[1].ID {
		t.Fatalf("retro identities collided: %#v", catalog.Games)
	}
}

func TestAliasedProfileGameUsesCanonicalPlatform(t *testing.T) {
	profiles := []model.Profile{{Games: []model.ProfileGame{{
		ID: "playnite:synthetic", Identities: map[string]string{"playnite": "synthetic"},
		Assets: map[string]model.AssetSelection{},
	}}}}
	builder := metadata.NewBuilder()
	builder.AddAlias("playnite:synthetic", "steam:42")
	catalog := BuildWithMetadata(review.Snapshot{}, profiles, builder.Build())
	if len(catalog.Games) != 1 || catalog.Games[0].PlatformID != "steam" {
		t.Fatalf("aliased profile platform = %#v", catalog.Games)
	}

}

func TestAliasedObservationUsesCanonicalPlatform(t *testing.T) {
	snapshot := review.Snapshot{Inventory: model.Inventory{Observations: []model.Observation{
		observation("playnite", "playnite-library", "synthetic.png", "playnite:synthetic", "", "cover", "playnite"),
	}}}
	builder := metadata.NewBuilder()
	builder.AddAlias("playnite:synthetic", "steam:42")
	catalog := BuildWithMetadata(snapshot, nil, builder.Build())
	if len(catalog.Games) != 1 || catalog.Games[0].ID != "steam:42" || catalog.Games[0].PlatformID != "steam" {
		t.Fatalf("aliased observation platform = %#v", catalog.Games)
	}
}

func TestBuildGroupsGamesAndPreservesFallbacks(t *testing.T) {
	snapshot := review.Snapshot{
		ScannedAt: time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC),
		Inventory: model.Inventory{Observations: []model.Observation{
			observation("steam", "steam-grid", "123.png", "steam:123", "", "grid", "same"),
			observation("catalog", "decky-catalog", "artwork/deck-default/grid/123p.png", "steam:123", "", "portrait", "same"),
			observation("retro", "esde-media", "n64/covers/Synthetic Quest (Region).png", "retro:n64:synthetic-quest-region", "n64", "cover", "retro"),
			observation("retro", "esde-media", "n64/marquees/system.png", "retro:n64:system", "n64", "marquee", "platform"),
			observation("unknown", "generic", "Loose File.png", "", "", "", "loose"),
		}},
	}
	profiles := []model.Profile{{
		Name: "Deck default", Games: []model.ProfileGame{
			{
				ID: "steam:123", Assets: map[string]model.AssetSelection{
					"grid": {SHA256: "same"},
				},
			},
			{
				ID: "steam:999", Identities: map[string]string{"steam": "999"},
				Assets: map[string]model.AssetSelection{},
			},
		},
	}}
	catalog := Build(snapshot, profiles)
	if len(catalog.Games) != 4 {
		t.Fatalf("games = %d, want 4", len(catalog.Games))
	}
	if catalog.NeedsAttention != 1 {
		t.Fatalf("needs attention = %d", catalog.NeedsAttention)
	}
	game, ok := FindGame(catalog, "steam:123")
	if !ok || len(game.Assets) != 2 {
		t.Fatalf("steam game = %+v, found %v", game, ok)
	}
	if game.Title != "Steam app 123" || game.Assets[0].SharedCopies != 2 {
		t.Fatalf("unexpected grouped game: %+v", game)
	}
	if len(game.Fallbacks) != 1 || game.Fallbacks[0].Frontend != "Steam" {
		t.Fatalf("fallbacks = %#v", game.Fallbacks)
	}
	if len(game.Profiles) != 1 || game.Profiles[0] != "Deck default" {
		t.Fatalf("profile usage = %#v", game.Profiles)
	}
	missing, ok := FindGame(catalog, "steam:999")
	if !ok || len(missing.Assets) != 0 || len(missing.MissingRoles) == 0 {
		t.Fatalf("profile-only game = %+v", missing)
	}
	retro, ok := FindGame(catalog, "retro:n64:synthetic-quest-region")
	if !ok || retro.Title != "Synthetic Quest" || retro.RawTitle != "Synthetic Quest (Region)" {
		t.Fatalf("retro game = %+v", retro)
	}
	for _, platform := range catalog.Platforms {
		if platform.ID == "retro:n64" && len(platform.Assets) != 1 {
			t.Fatalf("platform assets = %#v", platform.Assets)
		}
	}
}

func observation(root, kind, path, identity, system, role, hash string) model.Observation {
	return model.Observation{
		RootID: root, RootKind: kind, RelativePath: path, IdentityHint: identity,
		System: system, SHA256: hash, Size: 42,
		Media: model.MediaFacts{MIME: "image/png", Extension: "png", Width: 600, Height: 900, Role: role},
	}
}
