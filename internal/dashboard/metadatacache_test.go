package dashboard

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jrmoulckers/game-library/internal/model"
)

func TestMetadataCacheStartsAndInvalidates(t *testing.T) {
	cache := &metadataCache{}
	cache.start(nil, false)
	waitForMetadataStatus(t, cache, "ready")
	cache.invalidate()
	catalog, status := cache.current()
	if status != "idle" || catalog.Titles == nil {
		t.Fatalf("invalidated cache = %#v, %q", catalog, status)
	}
	if len(catalog.Diagnostics) != 0 || len(catalog.Titles) != 0 {
		t.Fatal("invalidated cache retained metadata")
	}
}

func TestMetadataCacheWithholdsCatalogDuringChangedRefresh(t *testing.T) {
	base := t.TempDir()
	media := filepath.Join(base, "downloaded_media")
	gamelist := filepath.Join(base, "gamelists", "n64", "gamelist.xml")
	if err := os.MkdirAll(filepath.Dir(gamelist), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(media, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(title string) {
		t.Helper()
		xml := `<gameList><game><path>Example.rom</path><name>` + title + `</name></game></gameList>`
		if err := os.WriteFile(gamelist, []byte(xml), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	roots := []model.Root{{Kind: "esde-media", Path: media}}
	cache := &metadataCache{}
	write("First Synthetic Title")
	cache.start(roots, false)
	waitForMetadataStatus(t, cache, "ready")
	write("A Different Synthetic Title")
	cache.start(roots, true)
	catalog, status := cache.current()
	if status != "loading" || len(catalog.Titles) != 0 || len(catalog.Aliases) != 0 {
		t.Fatalf("stale catalog remained visible: status=%q catalog=%#v", status, catalog)
	}
	waitForMetadataStatus(t, cache, "ready")
}

func waitForMetadataStatus(t *testing.T, cache *metadataCache, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, status := cache.current()
		if status == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("metadata status did not become %q", want)
		}
		time.Sleep(time.Millisecond)
	}
}
