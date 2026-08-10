package dashboard

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/organizer"
	"github.com/jrmoulckers/game-library/internal/review"
	"github.com/jrmoulckers/game-library/internal/workspace"
)

const reviewPNG = "\x89PNG\r\n\x1a\n" + "0123456789abcdefghijklmnopqrstuvwxyz"

func writeActiveConfigForTest(t *testing.T, paths workspace.Paths, cfg model.Config) {
	t.Helper()
	if err := workspace.WriteActiveConfig(paths.Config, "", cfg); err != nil {
		t.Fatal(err)
	}
}

func reviewSourceConfig(t *testing.T) (model.Config, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "grid"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "grid", "123.png"), []byte(reviewPNG), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := model.Config{
		Version: model.SchemaVersion,
		Roots:   []model.Root{{ID: "source", Kind: "steam-grid", Path: filepath.Join(root, "grid")}},
		Policy:  model.PolicyFile{Version: model.SchemaVersion, Default: "tracked-external"},
	}
	return cfg, root
}

func TestOrganizerCatalogAndGameUseGroupedReadModel(t *testing.T) {
	handler, _, paths := newTestHandlerWithOptions(t, Options{})
	cfg, _ := reviewSourceConfig(t)
	writeActiveConfigForTest(t, paths, cfg)

	var catalog organizer.Catalog
	var rec *httptest.ResponseRecorder
	deadline := time.Now().Add(5 * time.Second)
	for len(catalog.Games) == 0 && time.Now().Before(deadline) {
		rec = fetch(t, handler, "/api/organizer")
		if rec.Code != 200 {
			t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &catalog); err != nil {
			t.Fatal(err)
		}
		if len(catalog.Games) == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if len(catalog.Platforms) != 1 || len(catalog.Games) != 1 || catalog.Games[0].ID != "steam:123" {
		t.Fatalf("unexpected catalog: %+v", catalog)
	}

	rec = fetch(t, handler, "/api/organizer/games/steam:123")
	if rec.Code != 200 {
		t.Fatalf("game status = %d: %s", rec.Code, rec.Body.String())
	}
	var game organizer.Game
	if err := json.Unmarshal(rec.Body.Bytes(), &game); err != nil {
		t.Fatal(err)
	}
	if game.Title != "Steam app 123" || len(game.Assets) != 1 {
		t.Fatalf("unexpected game: %+v", game)
	}
}

func TestReviewMediaServesKnownIDAndRejectsUnknown(t *testing.T) {
	handler, _, paths := newTestHandlerWithOptions(t, Options{})
	cfg, _ := reviewSourceConfig(t)
	writeActiveConfigForTest(t, paths, cfg)

	id := review.ObservationID("source", "123.png")
	rec := fetch(t, handler, "/api/review/media/"+id)
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "image/png") {
		t.Fatalf("Content-Type = %q", rec.Header().Get("Content-Type"))
	}

	rec = fetch(t, handler, "/api/review/media/not-a-real-id")
	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404 for an unknown media id", rec.Code)
	}
}

func TestReviewMediaResponseNeverContainsAFilesystemPath(t *testing.T) {
	handler, _, paths := newTestHandlerWithOptions(t, Options{})
	cfg, root := reviewSourceConfig(t)
	writeActiveConfigForTest(t, paths, cfg)

	id := review.ObservationID("source", "123.png")
	rec := fetch(t, handler, "/api/review/media/"+id)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), root) {
		t.Fatal("media response must never leak the underlying filesystem root path")
	}
}

func TestReviewSurfaceHasNoApplyOrExecuteRoutes(t *testing.T) {
	handler, token, _ := newTestHandlerWithOptions(t, Options{})
	unregistered := []struct {
		method string
		path   string
	}{
		{"POST", "/api/review/gates/c/execute"},
		{"POST", "/api/review/plans/import/execute"},
		{"POST", "/api/review/apply"},
		{"POST", "/api/review/publish"},
		{"DELETE", "/api/review/plans/import"},
		{"DELETE", "/api/review/gates/a"},
		{"POST", "/api/review/rollback"},
	}
	for _, c := range unregistered {
		req := newRequest(c.method, c.path)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://"+testHost)
		req.Header.Set(csrfHeader, token)
		req.Body = httpBody(`{}`)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != 404 && rec.Code != 405 {
			t.Fatalf("%s %s: expected 404/405 (route must not exist/support this method), got %d", c.method, c.path, rec.Code)
		}
	}
}

func exampleReviewProfile(id string) model.Profile {
	return model.Profile{
		Version: model.SchemaVersion, ID: id, Name: "Example",
		Games: []model.ProfileGame{{
			ID:         "steam:123",
			Identities: map[string]string{"steam": "123"},
			Assets: map[string]model.AssetSelection{
				"grid": {SHA256: strings.Repeat("a", 64), Extension: "png"},
			},
		}},
	}
}
