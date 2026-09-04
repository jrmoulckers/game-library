package coverage

import (
	"testing"

	"github.com/jrmoulckers/game-library/internal/organizer"
	"github.com/jrmoulckers/game-library/internal/topology"
)

// asset builds a catalog asset belonging to a named artwork set.
func asset(set, role string) organizer.Asset {
	return organizer.Asset{Role: role, ArtworkSet: set, RootKind: "decky-catalog", SourceID: "catalog"}
}

// liveAsset builds an asset from a live frontend directory.
func liveAsset(sourceID, kind, role string) organizer.Asset {
	return organizer.Asset{Role: role, RootKind: kind, SourceID: sourceID, SourceName: kind}
}

func testDoc() topology.Document {
	doc := topology.Default()
	for i := range doc.Profiles {
		if doc.Profiles[i].Key() == "steam/standard" {
			doc.Profiles[i].Artwork = "deck-default"
		}
	}
	return doc
}

func TestProfileReportsTheGamesItHolds(t *testing.T) {
	catalog := organizer.Catalog{Games: []organizer.Game{
		{ID: "steam:10", Title: "Counter-Strike", PlatformID: "steam", PlatformName: "Steam", Assets: []organizer.Asset{
			asset("deck-default", "grid"),
			asset("deck-default", "hero"),
			asset("deck-default", "portrait"),
		}},
		{ID: "steam:20", Title: "Team Fortress", PlatformID: "steam", PlatformName: "Steam", Assets: []organizer.Asset{
			asset("deck-default", "grid"),
		}},
	}}

	report := Build(catalog, testDoc())

	var standard Profile
	for _, profile := range report.Profiles {
		if profile.Key == "steam/standard" {
			standard = profile
		}
	}
	if standard.Key == "" {
		t.Fatal("the bound profile is missing from the report")
	}
	if standard.Empty {
		t.Fatal("a profile backed by real artwork must not report as empty")
	}
	if standard.GameCount != 2 || standard.AssetCount != 4 {
		t.Fatalf("games=%d assets=%d, want 2 and 4", standard.GameCount, standard.AssetCount)
	}
	if standard.Games[0].Title != "Counter-Strike" {
		t.Fatalf("games should be sorted by title, got %+v", standard.Games)
	}
	if len(standard.Games[0].Roles) != 3 {
		t.Fatalf("expected three roles for the first game, got %+v", standard.Games[0].Roles)
	}
}

func TestProfileWithNoArtworkIsEmptyNotMissing(t *testing.T) {
	report := Build(organizer.Catalog{}, testDoc())
	if len(report.Profiles) != len(topology.Default().Profiles) {
		t.Fatalf("every declared profile must appear, got %d", len(report.Profiles))
	}
	for _, profile := range report.Profiles {
		if !profile.Empty {
			t.Fatalf("profile %q should be empty with no catalog", profile.Key)
		}
		if profile.Games == nil {
			t.Fatalf("profile %q must serialise an empty game list, not null", profile.Key)
		}
	}
}

func TestGameShowsEveryProfileOnItsPlatformIncludingGaps(t *testing.T) {
	catalog := organizer.Catalog{Games: []organizer.Game{
		{ID: "steam:10", Title: "Counter-Strike", PlatformID: "steam", PlatformName: "Steam", Assets: []organizer.Asset{
			asset("deck-default", "grid"),
		}},
	}}

	report := Build(catalog, testDoc())
	if len(report.Games) != 1 {
		t.Fatalf("expected one game, got %d", len(report.Games))
	}
	game := report.Games[0]
	// Steam declares four profiles, so all four are relevant to a Steam
	// game even though only one holds artwork.
	if len(game.Profiles) != 4 {
		t.Fatalf("expected all four Steam profiles, got %d", len(game.Profiles))
	}
	if game.CoveredCount != 1 {
		t.Fatalf("covered=%d, want 1", game.CoveredCount)
	}
	covered := 0
	for _, profile := range game.Profiles {
		if profile.PlatformID != "steam" {
			t.Fatalf("a Playnite or retro profile must not apply to a Steam game: %+v", profile)
		}
		if profile.Covered {
			covered++
			if len(profile.Devices) != 3 {
				t.Fatalf("a Steam profile reaches three devices, got %d", len(profile.Devices))
			}
		} else if len(profile.Roles) != 0 {
			t.Fatalf("an uncovered profile must report no roles: %+v", profile)
		}
	}
	if covered != 1 {
		t.Fatalf("exactly one profile should cover this game, got %d", covered)
	}
}

func TestArtworkIsNeverSharedAcrossPlatforms(t *testing.T) {
	// The same title on Steam and on a retro system are separate games
	// with separate artwork, and neither borrows the other's profiles.
	catalog := organizer.Catalog{Games: []organizer.Game{
		{ID: "steam:10", Title: "Doom", PlatformID: "steam", PlatformName: "Steam", Assets: []organizer.Asset{
			asset("deck-default", "grid"),
		}},
		{ID: "retro:snes:doom", Title: "Doom", PlatformID: "retro:snes", PlatformName: "SNES"},
	}}

	report := Build(catalog, testDoc())
	for _, game := range report.Games {
		switch game.PlatformID {
		case "steam":
			if game.CoveredCount != 1 {
				t.Fatalf("the Steam copy should be covered once, got %d", game.CoveredCount)
			}
		case "retro:snes":
			if game.CoveredCount != 0 {
				t.Fatal("the retro copy must not inherit Steam artwork")
			}
			// Retro systems all fold into the single "retro" platform.
			if len(game.Profiles) != 4 {
				t.Fatalf("expected the four retro profiles, got %d", len(game.Profiles))
			}
			for _, profile := range game.Profiles {
				if profile.PlatformID != "retro" {
					t.Fatalf("unexpected platform %q on a retro game", profile.PlatformID)
				}
			}
		}
	}
}

func TestUnclaimedArtworkSetsAreSurfaced(t *testing.T) {
	catalog := organizer.Catalog{Games: []organizer.Game{
		{ID: "steam:10", Title: "Counter-Strike", PlatformID: "steam", Assets: []organizer.Asset{
			asset("deck-default", "grid"),
			asset("mystery-set", "grid"),
			asset("mystery-set", "hero"),
		}},
	}}

	report := Build(catalog, testDoc())
	if len(report.Unbound) != 1 {
		t.Fatalf("expected exactly the unclaimed set, got %+v", report.Unbound)
	}
	if report.Unbound[0].ArtworkSet != "mystery-set" {
		t.Fatalf("wrong set reported: %+v", report.Unbound[0])
	}
	if report.Unbound[0].GameCount != 1 || report.Unbound[0].AssetCount != 2 {
		t.Fatalf("unexpected counts: %+v", report.Unbound[0])
	}
}

func TestLiveFrontendDirectoriesAreCountedSeparately(t *testing.T) {
	catalog := organizer.Catalog{Games: []organizer.Game{
		{ID: "steam:10", PlatformID: "steam", Assets: []organizer.Asset{
			asset("deck-default", "grid"),
			liveAsset("desktop-steam-grid", "steam-grid", "grid"),
			liveAsset("desktop-steam-grid", "steam-grid", "hero"),
		}},
		{ID: "steam:20", PlatformID: "steam", Assets: []organizer.Asset{
			liveAsset("desktop-steam-grid", "steam-grid", "grid"),
		}},
	}}

	report := Build(catalog, testDoc())
	if len(report.LiveSurfaces) != 1 {
		t.Fatalf("expected one live surface, got %+v", report.LiveSurfaces)
	}
	surface := report.LiveSurfaces[0]
	if surface.SourceID != "desktop-steam-grid" {
		t.Fatalf("unexpected surface: %+v", surface)
	}
	// Two games, three files: a repeated game must not inflate the count.
	if surface.GameCount != 2 || surface.AssetCount != 3 {
		t.Fatalf("games=%d assets=%d, want 2 and 3", surface.GameCount, surface.AssetCount)
	}
	// Catalog payload is the source of truth and is never a live surface.
	if surface.RootKind == "decky-catalog" {
		t.Fatal("the canonical catalog must not be reported as a live surface")
	}
}

func TestReportCollectionsAreNeverNull(t *testing.T) {
	report := Build(organizer.Catalog{}, topology.Document{Version: topology.Version})
	if report.Profiles == nil || report.Games == nil || report.Unbound == nil || report.LiveSurfaces == nil {
		t.Fatalf("every collection must serialise as [] rather than null: %+v", report)
	}
}
