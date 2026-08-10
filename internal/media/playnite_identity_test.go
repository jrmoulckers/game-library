package media

import "testing"

// Playnite stores assets as <gameGuid>/<assetGuid>.<ext>. Keying on the file
// stem invented one game per asset file, so a 93-game library appeared as 277.
func TestPlayniteIdentityUsesGameDirectoryNotAssetFile(t *testing.T) {
	const game = "00981458-a636-4d82-945f-4ddb53ba60af"
	const asset = "0134715d-8e59-4aa3-9e73-6a2960f86090"
	if got := InferIdentityHint("playnite-library", game+"/"+asset+".jpg", ""); got != "playnite:"+game {
		t.Fatalf("InferIdentityHint = %q, want the game directory identity", got)
	}
	// Several assets in one directory must collapse onto a single game.
	first := InferIdentityHint("playnite-library", game+"/"+asset+".ico", "")
	second := InferIdentityHint("playnite-library", game+"/f1c0051c-8fb5-48cb-b289-d049d61f1483.jpg", "")
	if first != second {
		t.Fatalf("assets in one game directory split into %q and %q", first, second)
	}
	// A flat asset with no game directory still falls back to its own name.
	if got := InferIdentityHint("playnite-library", asset+".png", ""); got != "playnite:"+asset {
		t.Fatalf("flat layout fallback = %q", got)
	}
}
