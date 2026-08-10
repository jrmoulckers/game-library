package dashboard

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/review"
	"github.com/jrmoulckers/game-library/internal/workspace"
)

// TestPlanAndGateResponsesNeverContainAnAbsoluteWorkspacePath covers issue
// #1 end to end: every plan-persisting and gate-review endpoint response
// must reference its artifact only by a relative/symbolic name, never the
// workspace root or any other absolute filesystem path, and a sanitized
// error response must never leak one either.
func TestPlanAndGateResponsesNeverContainAnAbsoluteWorkspacePath(t *testing.T) {
	handler, token, paths := newTestHandlerWithOptions(t, Options{})
	cfg, _ := reviewSourceConfig(t)
	cfg.Policy.Default = "managed"
	writeActiveConfigForTest(t, paths, cfg)

	assertNoWorkspaceRoot := func(label string, rec *httptest.ResponseRecorder) {
		t.Helper()
		body := rec.Body.String()
		if strings.Contains(body, paths.Root) {
			t.Fatalf("%s: response leaked the workspace root: %s", label, body)
		}
	}

	rec := mutate(t, handler, token, "POST", "/api/review/plans/import", map[string]any{})
	if rec.Code != 200 {
		t.Fatalf("plan import: status = %d: %s", rec.Code, rec.Body.String())
	}
	assertNoWorkspaceRoot("plan import", rec)
	var planView planPersistedView
	if err := json.Unmarshal(rec.Body.Bytes(), &planView); err != nil {
		t.Fatal(err)
	}
	if planView.Artifact == "" || filepath.IsAbs(planView.Artifact) {
		t.Fatalf("expected a non-empty, non-absolute artifact reference, got %q", planView.Artifact)
	}

	rec = mutate(t, handler, token, "POST", "/api/review/gates/a", map[string]any{"inventoryDigest": "inv"})
	if rec.Code != 200 {
		t.Fatalf("gate A: status = %d: %s", rec.Code, rec.Body.String())
	}
	assertNoWorkspaceRoot("gate A", rec)
}

// TestReviewMediaResponsesNeverLeakAPathOnError covers issue #1's error-path
// requirement: a sanitized error for an unsafe/not-found media request must
// never contain the configured root's absolute path.
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

// TestReviewSnapshotIsCachedAcrossRequests covers issue #4: LoadSnapshot
// must not be invoked (i.e. the roots must not be rescanned) on every read
// request. We assert this indirectly: adding a new file to the configured
// root after the first request must not appear until an explicit refresh.
func TestReviewSnapshotIsCachedAcrossRequests(t *testing.T) {
	handler, token, paths := newTestHandlerWithOptions(t, Options{})
	cfg, root := reviewSourceConfig(t)
	writeActiveConfigForTest(t, paths, cfg)

	rec := fetch(t, handler, "/api/review/overview")
	var first review.Overview
	if err := json.Unmarshal(rec.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if first.Roots[0].FileCount != 1 {
		t.Fatalf("expected 1 file initially, got %d", first.Roots[0].FileCount)
	}

	// Add a second file directly to the configured root, bypassing the
	// dashboard entirely.
	if err := os.WriteFile(filepath.Join(root, "grid", "456.png"), []byte(reviewPNG), 0o644); err != nil {
		t.Fatal(err)
	}

	rec = fetch(t, handler, "/api/review/overview")
	var second review.Overview
	if err := json.Unmarshal(rec.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if second.Roots[0].FileCount != 1 {
		t.Fatalf("expected the cached snapshot to still report 1 file before any refresh, got %d", second.Roots[0].FileCount)
	}

	// POST /api/review/refresh must pick up the new file.
	rec = mutate(t, handler, token, "POST", "/api/review/refresh", map[string]any{})
	if rec.Code != 200 {
		t.Fatalf("refresh: status = %d: %s", rec.Code, rec.Body.String())
	}
	var refreshed struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &refreshed); err != nil {
		t.Fatal(err)
	}
	if refreshed.Status != "completed" {
		t.Fatalf("Status = %q, want completed", refreshed.Status)
	}

	rec = fetch(t, handler, "/api/review/overview")
	var third review.Overview
	if err := json.Unmarshal(rec.Body.Bytes(), &third); err != nil {
		t.Fatal(err)
	}
	if third.Roots[0].FileCount != 2 {
		t.Fatalf("expected the refreshed snapshot to report 2 files, got %d", third.Roots[0].FileCount)
	}
}

// TestReviewSnapshotInvalidatedAfterConfigUpdate covers issue #4: PUT
// /api/config must invalidate the cached snapshot.
func TestReviewSnapshotInvalidatedAfterConfigUpdate(t *testing.T) {
	handler, token, paths := newTestHandlerWithOptions(t, Options{})
	cfg, _ := reviewSourceConfig(t)
	writeActiveConfigForTest(t, paths, cfg)

	fetch(t, handler, "/api/review/overview")
	var view configView
	getRec := fetch(t, handler, "/api/config")
	if err := json.Unmarshal(getRec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}

	// Reconfigure to point at an entirely empty root.
	emptyRoot := t.TempDir()
	newCfg := cfg
	newCfg.Roots = []model.Root{{ID: "source", Kind: "steam-grid", Path: emptyRoot}}
	rec := mutate(t, handler, token, "PUT", "/api/config", configRequest{BaseDigest: view.Digest, Config: newCfg})
	if rec.Code != 200 {
		t.Fatalf("PUT config: status = %d: %s", rec.Code, rec.Body.String())
	}

	rec = fetch(t, handler, "/api/review/overview")
	var overview review.Overview
	if err := json.Unmarshal(rec.Body.Bytes(), &overview); err != nil {
		t.Fatal(err)
	}
	if overview.Roots[0].FileCount != 0 {
		t.Fatalf("expected the snapshot cache to be invalidated after a config update, got %d files", overview.Roots[0].FileCount)
	}
}

// TestReviewRefreshRequiresCSRF covers issue #4's "CSRF-protected" refresh
// requirement: the shared security middleware already enforces this for
// every POST route, but we assert it explicitly for this one.
func TestReviewRefreshRequiresCSRF(t *testing.T) {
	handler, _, paths := newTestHandlerWithOptions(t, Options{})
	cfg, _ := reviewSourceConfig(t)
	writeActiveConfigForTest(t, paths, cfg)

	req := newRequest("POST", "/api/review/refresh")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+testHost)
	// Deliberately omit the CSRF header.
	req.Body = httpBody(`{}`)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("status = %d, want 403 without a CSRF token", rec.Code)
	}
}

// TestReviewProfileDraftsListEndpoint covers issue #5.
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

// TestReviewThemesEndpoint covers issue #5.
func TestReviewThemesEndpoint(t *testing.T) {
	catalogRoot := t.TempDir()
	themeDir := filepath.Join(catalogRoot, "library", "themes", "retro-neon")
	if err := os.MkdirAll(themeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(themeDir, "theme.json"), []byte(`{"id":"retro-neon"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	handler, _, _ := newTestHandlerWithOptions(t, Options{CatalogRoot: catalogRoot})

	rec := fetch(t, handler, "/api/review/themes")
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var view struct {
		Themes []string `json:"themes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Themes) != 1 || view.Themes[0] != "retro-neon" {
		t.Fatalf("unexpected themes: %+v", view.Themes)
	}
	if strings.Contains(rec.Body.String(), catalogRoot) {
		t.Fatal("themes response must never leak the catalog root path")
	}
}

// TestReviewThemesRequiresCatalogRoot covers issue #5.
func TestReviewThemesRequiresCatalogRoot(t *testing.T) {
	handler, _, _ := newTestHandlerWithOptions(t, Options{})
	rec := fetch(t, handler, "/api/review/themes")
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 without a configured catalog root", rec.Code)
	}
}

// TestReviewAdapterStatusEndpoint covers issue #5.
func TestReviewAdapterStatusEndpoint(t *testing.T) {
	handler, _, paths := newTestHandlerWithOptions(t, Options{})
	writeActiveConfigForTest(t, paths, model.Config{
		Version: model.SchemaVersion,
		Roots:   []model.Root{{ID: "steam", Kind: "generic", Path: t.TempDir()}},
		Policy:  model.PolicyFile{Version: model.SchemaVersion, Default: "tracked-external"},
	})
	if _, err := workspace.SaveProfileDraft(paths, "", exampleReviewProfile("adaptertest")); err != nil {
		t.Fatal(err)
	}

	rec := fetch(t, handler, "/api/review/adapters")
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var statuses []adapterStatusView
	if err := json.Unmarshal(rec.Body.Bytes(), &statuses); err != nil {
		t.Fatal(err)
	}
	if len(statuses) != len(adapterNames) {
		t.Fatalf("expected %d adapters, got %d", len(adapterNames), len(statuses))
	}
	byName := make(map[string]adapterStatusView)
	for _, s := range statuses {
		byName[s.Adapter] = s
		if !s.PlanOnly {
			t.Fatalf("expected every adapter to be plan-only, got %+v", s)
		}
	}
	if !byName["steam"].DestinationConfigured {
		t.Fatal("expected the steam destination to be configured from the active config root")
	}
	if !byName["steam"].InputReady {
		t.Fatal("expected steam input readiness from the saved profile draft")
	}
	if byName["playnite"].DestinationConfigured {
		t.Fatal("expected playnite to be unconfigured")
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

// TestReviewHistoryEndpoint covers issue #5.
func TestReviewHistoryEndpoint(t *testing.T) {
	handler, token, paths := newTestHandlerWithOptions(t, Options{})
	if _, err := workspace.SaveProfileDraft(paths, "", exampleReviewProfile("historytest")); err != nil {
		t.Fatal(err)
	}
	rec := mutate(t, handler, token, "POST", "/api/review/plans/export", map[string]any{
		"profileId": "historytest", "adapter": "steam",
	})
	if rec.Code != 200 {
		t.Fatalf("plan export: status = %d: %s", rec.Code, rec.Body.String())
	}

	rec = fetch(t, handler, "/api/review/history")
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var entries []review.HistoryEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Type != "plan" || !entries[0].Verified {
		t.Fatalf("unexpected history: %+v", entries)
	}
	if strings.Contains(rec.Body.String(), paths.Root) {
		t.Fatal("history response must never leak the workspace root path")
	}
}
