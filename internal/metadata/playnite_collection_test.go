package metadata

import (
	"path/filepath"
	"testing"
)

// Playnite's real collection is "Game"; older databases and fixtures use
// "games". Both must resolve, and neither may be assumed.
func TestPlayniteAcceptsRealAndLegacyCollectionNames(t *testing.T) {
	for _, collection := range []string{"Game", "games"} {
		path := filepath.Join(t.TempDir(), "games.db")
		writeSyntheticPlayniteFixtureNamed(t, path, collection,
			"65bd1f5b-0000-4000-8000-000000000001", "Collection Title", "440", SteamPluginID)
		result := ReadPlaynite(path)
		if result.Status != playniteStatusOK {
			t.Fatalf("collection %q: status = %q, want ok", collection, result.Status)
		}
		if len(result.Games) != 1 || result.Games[0].Name != "Collection Title" {
			t.Fatalf("collection %q: games = %#v", collection, result.Games)
		}
	}
}
