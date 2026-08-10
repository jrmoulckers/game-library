package dashboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
)

// writeCatalogProfile lays down a catalog profile plus its artwork folder
// the same way the synced catalog stores them on a real device.
func writeCatalogProfile(t *testing.T, root, id, name, desc string, artwork []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "profiles"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := map[string]any{
		"version":     1,
		"id":          id,
		"name":        name,
		"description": desc,
		"artwork":     id,
		"mods":        []any{},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "profiles", id+".json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "artwork", id, "grid")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, file := range artwork {
		if err := os.WriteFile(filepath.Join(dir, file), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func catalogConfig(root string) model.Config {
	cfg := exampleConfig()
	cfg.Roots = []model.Root{{ID: "catalog", Kind: "decky-catalog", Path: root}}
	return cfg
}

func TestReviewProfilesListsLiveCatalogProfiles(t *testing.T) {
	handler, token := newTestHandler(t)
	root := t.TempDir()
	writeCatalogProfile(t, root, "deck-default", "Deck Default", "The preserved Steam Deck artwork set.", []string{"1.png", "2.png"})

	// A profile that deliberately carries no artwork still lists, and its
	// empty marker must not be counted as an asset.
	writeCatalogProfile(t, root, "steam-default", "Steam Default", "Use Steam's built-in artwork.", nil)
	if err := os.WriteFile(filepath.Join(root, "artwork", "steam-default", "grid", ".deck-profile-empty"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if rec := mutate(t, handler, token, "PUT", "/api/config", configRequest{Config: catalogConfig(root)}); rec.Code != 200 {
		t.Fatalf("PUT config status = %d: %s", rec.Code, rec.Body.String())
	}

	rec := fetch(t, handler, "/api/review/profiles")
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got []profileDraftSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected both live profiles, got %+v", got)
	}
	deck, steam := got[0], got[1]
	if deck.ID != "deck-default" || steam.ID != "steam-default" {
		t.Fatalf("expected profiles sorted by id, got %+v", got)
	}
	if deck.Source != "catalog" || !deck.ReadOnly {
		t.Fatalf("live profiles must be read-only catalog entries: %+v", deck)
	}
	if deck.Artwork != 2 {
		t.Fatalf("deck-default artwork = %d, want 2", deck.Artwork)
	}
	if steam.Artwork != 0 {
		t.Fatalf("the empty marker must not count as artwork, got %d", steam.Artwork)
	}
	if deck.Description == "" {
		t.Fatal("expected the catalog description to be surfaced")
	}
}

func TestReviewProfilesPrefersDraftOverLiveProfile(t *testing.T) {
	handler, token := newTestHandler(t)
	root := t.TempDir()
	writeCatalogProfile(t, root, "deck-default", "Deck Default", "live copy", []string{"1.png"})
	if rec := mutate(t, handler, token, "PUT", "/api/config", configRequest{Config: catalogConfig(root)}); rec.Code != 200 {
		t.Fatalf("PUT config status = %d: %s", rec.Code, rec.Body.String())
	}

	draft := map[string]any{
		"baseDigest": "",
		"profile":    map[string]any{"version": 1, "id": "deck-default", "name": "Deck Default", "games": []any{}},
	}
	if rec := mutate(t, handler, token, "PUT", "/api/drafts/profiles/deck-default", draft); rec.Code != 200 {
		t.Fatalf("PUT draft status = %d: %s", rec.Code, rec.Body.String())
	}

	rec := fetch(t, handler, "/api/review/profiles")
	var got []profileDraftSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("a draft must not duplicate the live profile it shadows: %+v", got)
	}
	if got[0].Source != "draft" || got[0].ReadOnly {
		t.Fatalf("the editable draft must win the listing: %+v", got[0])
	}
}

func TestReviewProfilesToleratesMissingCatalog(t *testing.T) {
	handler, token := newTestHandler(t)
	missing := filepath.Join(t.TempDir(), "not-synced-yet")
	if err := os.MkdirAll(missing, 0o755); err != nil {
		t.Fatal(err)
	}
	if rec := mutate(t, handler, token, "PUT", "/api/config", configRequest{Config: catalogConfig(missing)}); rec.Code != 200 {
		t.Fatalf("PUT config status = %d: %s", rec.Code, rec.Body.String())
	}
	rec := fetch(t, handler, "/api/review/profiles")
	if rec.Code != 200 {
		t.Fatalf("a catalog with no profiles must not break the view: %d %s", rec.Code, rec.Body.String())
	}
	var got []profileDraftSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no profiles, got %+v", got)
	}
}
