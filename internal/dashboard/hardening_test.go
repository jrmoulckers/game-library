package dashboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/review"
	"github.com/jrmoulckers/game-library/internal/workspace"
)

func TestReviewMediaResponsesNeverLeakAPathOnError(t *testing.T) {
	handler, _, paths := newTestHandlerWithOptions(t, Options{})
	cfg, root := reviewSourceConfig(t)
	writeActiveConfigForTest(t, paths, cfg)

	rec := fetch(t, handler, "/api/review/media/not-a-real-id")
	if rec.Code != 404 {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), root) {
		t.Fatal("media error response must never leak the underlying filesystem root path")
	}
}

// TestReviewMediaRejectsNonImageMIMEInline covers issue #7: the inline
// thumbnail endpoint only ever serves image/* MIME types.
func TestReviewMediaRejectsNonImageMIMEInline(t *testing.T) {
	handler, _, paths := newTestHandlerWithOptions(t, Options{})
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "manual.bin"), []byte("%PDF-1.4 fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := model.Config{
		Version: model.SchemaVersion,
		Roots:   []model.Root{{ID: "source", Kind: "generic", Path: root}},
		Policy:  model.PolicyFile{Version: model.SchemaVersion, Default: "tracked-external"},
	}
	writeActiveConfigForTest(t, paths, cfg)

	id := review.ObservationID("source", "manual.bin")
	rec := fetch(t, handler, "/api/review/media/"+id)
	if rec.Code != 403 {
		t.Fatalf("status = %d, want 403 for a non-image inline preview", rec.Code)
	}
}

// TestReviewMediaSetsETagFromObservedSHA256 covers issue #7's ETag
// requirement.
func TestReviewMediaSetsETagFromObservedSHA256(t *testing.T) {
	handler, _, paths := newTestHandlerWithOptions(t, Options{})
	cfg, _ := reviewSourceConfig(t)
	writeActiveConfigForTest(t, paths, cfg)

	id := review.ObservationID("source", "123.png")
	rec := fetch(t, handler, "/api/review/media/"+id)
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("expected a non-empty ETag header")
	}
}

// TestReviewMediaDownloadSetsContentDispositionAttachment covers issue #7:
// the separate download path always serves as an attachment.
func TestReviewMediaDownloadSetsContentDispositionAttachment(t *testing.T) {
	handler, _, paths := newTestHandlerWithOptions(t, Options{})
	cfg, _ := reviewSourceConfig(t)
	writeActiveConfigForTest(t, paths, cfg)

	id := review.ObservationID("source", "123.png")
	rec := fetch(t, handler, "/api/review/media/"+id+"/download")
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	disposition := rec.Header().Get("Content-Disposition")
	if !strings.HasPrefix(disposition, "attachment;") {
		t.Fatalf("Content-Disposition = %q, want an attachment", disposition)
	}
	if rec.Header().Get("ETag") == "" {
		t.Fatal("expected a non-empty ETag header on the download path too")
	}
}

// TestReviewMediaDownloadRejectsDisallowedMIME covers the download path's
// own allowlist: it does not simply proxy whatever media.Inspect sniffed.
func TestReviewMediaDownloadRejectsDisallowedMIME(t *testing.T) {
	handler, _, paths := newTestHandlerWithOptions(t, Options{})
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "weird.bin"), []byte("just some random bytes, not a known type"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := model.Config{
		Version: model.SchemaVersion,
		Roots:   []model.Root{{ID: "source", Kind: "generic", Path: root}},
		Policy:  model.PolicyFile{Version: model.SchemaVersion, Default: "tracked-external"},
	}
	writeActiveConfigForTest(t, paths, cfg)

	id := review.ObservationID("source", "weird.bin")
	rec := fetch(t, handler, "/api/review/media/"+id+"/download")
	if rec.Code != 403 {
		t.Fatalf("status = %d, want 403 for a disallowed download MIME type", rec.Code)
	}
}

func TestReviewProfileDraftsListEndpoint(t *testing.T) {
	handler, _, paths := newTestHandlerWithOptions(t, Options{})
	profileDraft := exampleReviewProfile("listed")
	profileDraft.Theme = "retro-neon"
	if _, err := workspace.SaveProfileDraft(paths, "", profileDraft); err != nil {
		t.Fatal(err)
	}

	rec := fetch(t, handler, "/api/review/profiles")
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var summaries []profileDraftSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &summaries); err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 || summaries[0].ID != "listed" || summaries[0].Theme != "retro-neon" {
		t.Fatalf("unexpected summaries: %+v", summaries)
	}
}

// TestValidateSetupRootsEndpoint covers issue #5.
func TestValidateSetupRootsEndpoint(t *testing.T) {
	handler, token, _ := newTestHandlerWithOptions(t, Options{})
	existingDir := t.TempDir()
	filePath := filepath.Join(existingDir, "not-a-dir.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	body := map[string]any{
		"roots": []model.Root{
			{ID: "good", Kind: "generic", Path: existingDir},
			{ID: "missing", Kind: "generic", Path: filepath.Join(existingDir, "does-not-exist")},
			{ID: "not-a-dir", Kind: "generic", Path: filePath},
			{ID: "Good", Kind: "generic", Path: existingDir},
		},
	}
	rec := mutate(t, handler, token, "POST", "/api/config/validate-roots", body)
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var view validateRootsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(view.Results))
	}
	byID := make(map[string]rootValidationResult)
	for _, res := range view.Results {
		byID[res.ID] = res
	}
	if !byID["good"].Exists || !byID["good"].IsDir || !byID["good"].Readable {
		t.Fatalf("unexpected 'good' result: %+v", byID["good"])
	}
	if byID["missing"].Exists {
		t.Fatalf("unexpected 'missing' result: %+v", byID["missing"])
	}
	if !byID["not-a-dir"].Exists || byID["not-a-dir"].IsDir {
		t.Fatalf("unexpected 'not-a-dir' result: %+v", byID["not-a-dir"])
	}
	if len(view.CaseCollisions) != 1 || len(view.CaseCollisions[0]) != 2 {
		t.Fatalf("expected exactly one case collision group of size 2, got %+v", view.CaseCollisions)
	}
	if strings.Contains(rec.Body.String(), existingDir) {
		t.Fatal("validate-roots response must never echo a filesystem path")
	}
}
