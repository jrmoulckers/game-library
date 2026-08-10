package dashboard

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jrmoulckers/game-library/internal/manifest"
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

func TestReviewOverviewRequiresActiveConfig(t *testing.T) {
	handler, _ := newTestHandler(t)
	rec := fetch(t, handler, "/api/review/overview")
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestReviewOverviewScansConfiguredRoot(t *testing.T) {
	handler, _, paths := newTestHandlerWithOptions(t, Options{})
	cfg, _ := reviewSourceConfig(t)
	writeActiveConfigForTest(t, paths, cfg)

	rec := fetch(t, handler, "/api/review/overview")
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	var overview review.Overview
	if err := json.Unmarshal(rec.Body.Bytes(), &overview); err != nil {
		t.Fatal(err)
	}

	if len(overview.Roots) != 1 || overview.Roots[0].FileCount != 1 {
		t.Fatalf("unexpected overview: %+v", overview)
	}
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

func TestReviewObservationsFiltersByRole(t *testing.T) {
	handler, _, paths := newTestHandlerWithOptions(t, Options{})
	cfg, _ := reviewSourceConfig(t)
	writeActiveConfigForTest(t, paths, cfg)

	rec := fetch(t, handler, "/api/review/observations?role=grid")
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var page review.ObservationPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 {
		t.Fatalf("Total = %d, want 1", page.Total)
	}

	rec = fetch(t, handler, "/api/review/observations?role=nonexistent")
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 0 {
		t.Fatalf("expected 0 results for an unmatched role filter, got %d", page.Total)
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

func TestReviewIdentityAttachesObservationIDs(t *testing.T) {
	handler, _, paths := newTestHandlerWithOptions(t, Options{})
	cfg, _ := reviewSourceConfig(t)
	writeActiveConfigForTest(t, paths, cfg)

	rec := fetch(t, handler, "/api/review/identity")
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var view review.IdentityView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Proposals) != 1 {
		t.Fatalf("expected 1 identity proposal, got %d", len(view.Proposals))
	}
}

func TestReviewDuplicatesReturnsEmptyForUniqueContent(t *testing.T) {
	handler, _, paths := newTestHandlerWithOptions(t, Options{})
	cfg, _ := reviewSourceConfig(t)
	writeActiveConfigForTest(t, paths, cfg)

	rec := fetch(t, handler, "/api/review/duplicates")
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var groups []review.DuplicateGroupView
	if err := json.Unmarshal(rec.Body.Bytes(), &groups); err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("expected no duplicate groups, got %+v", groups)
	}
}

func TestReviewPolicyImpactUsesActiveConfigPolicy(t *testing.T) {
	handler, _, paths := newTestHandlerWithOptions(t, Options{})
	cfg, _ := reviewSourceConfig(t)
	cfg.Policy.Rules = []model.PolicyRule{{Source: "source", Mode: "managed"}}
	writeActiveConfigForTest(t, paths, cfg)

	rec := fetch(t, handler, "/api/review/policy-impact")
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var view review.PolicyImpactView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Counts["managed"] != 1 {
		t.Fatalf("expected 1 managed observation, got %+v", view.Counts)
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

func TestReviewProfileResolveRequiresCatalogRoot(t *testing.T) {
	handler, token, paths := newTestHandlerWithOptions(t, Options{})
	if _, err := workspace.SaveProfileDraft(paths, "", exampleReviewProfile("example")); err != nil {
		t.Fatal(err)
	}
	_ = token
	rec := fetch(t, handler, "/api/review/profiles/example/resolve")
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 without a configured catalog root", rec.Code)
	}
}

func TestReviewProfileResolveWithCatalogRoot(t *testing.T) {
	catalogRoot := t.TempDir()
	handler, _, paths := newTestHandlerWithOptions(t, Options{CatalogRoot: catalogRoot})
	if _, err := workspace.SaveProfileDraft(paths, "", exampleReviewProfile("example")); err != nil {
		t.Fatal(err)
	}
	rec := fetch(t, handler, "/api/review/profiles/example/resolve")
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var resolution model.ProfileResolution
	if err := json.Unmarshal(rec.Body.Bytes(), &resolution); err != nil {
		t.Fatal(err)
	}
	if resolution.Complete {
		t.Fatal("expected an incomplete resolution: the catalog root has no managed assets")
	}
}

func TestReviewProfileResolveUnknownProfile404s(t *testing.T) {
	handler, _, _ := newTestHandlerWithOptions(t, Options{CatalogRoot: t.TempDir()})
	rec := fetch(t, handler, "/api/review/profiles/nonexistent/resolve")
	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestReviewExportPreviewDeckyValidatesInvariants(t *testing.T) {
	handler, _, paths := newTestHandlerWithOptions(t, Options{})
	if _, err := workspace.SaveProfileDraft(paths, "", exampleReviewProfile("example")); err != nil {
		t.Fatal(err)
	}
	rec := fetch(t, handler, "/api/review/profiles/example/export/decky")
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var preview review.ExportPreview
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.DeckyProfile == nil || preview.DeckyProfile.Version != 1 {
		t.Fatalf("expected a validated Decky v1 profile preview, got %+v", preview)
	}
}

func TestReviewPlanImportPersistsArtifactAndIsIdempotent(t *testing.T) {
	handler, token, paths := newTestHandlerWithOptions(t, Options{})
	cfg, _ := reviewSourceConfig(t)
	cfg.Policy.Default = "managed"
	writeActiveConfigForTest(t, paths, cfg)

	rec := mutate(t, handler, token, "POST", "/api/review/plans/import", map[string]any{})
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var first planPersistedView
	if err := json.Unmarshal(rec.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if !first.Created {
		t.Fatal("expected the first import plan persist to create a new artifact")
	}

	rec = mutate(t, handler, token, "POST", "/api/review/plans/import", map[string]any{})
	var second planPersistedView
	if err := json.Unmarshal(rec.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if second.Created {
		t.Fatal("expected the second identical import plan persist to be idempotent")
	}
	if second.Plan.OperationID != first.Plan.OperationID {
		t.Fatal("expected the same deterministic operation id")
	}
}

func TestReviewPlanExportPersistsArtifact(t *testing.T) {
	handler, token, paths := newTestHandlerWithOptions(t, Options{})
	if _, err := workspace.SaveProfileDraft(paths, "", exampleReviewProfile("example")); err != nil {
		t.Fatal(err)
	}
	rec := mutate(t, handler, token, "POST", "/api/review/plans/export", map[string]any{
		"profileId": "example", "adapter": "steam",
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var view planPersistedView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !view.Created || view.Artifact == "" {
		t.Fatalf("expected a newly created artifact: %+v", view)
	}
}

func TestReviewPlanBundleRequiresCatalogRoot(t *testing.T) {
	handler, token, paths := newTestHandlerWithOptions(t, Options{})
	if _, err := workspace.SaveProfileDraft(paths, "", exampleReviewProfile("example")); err != nil {
		t.Fatal(err)
	}
	rec := mutate(t, handler, token, "POST", "/api/review/plans/bundle", map[string]any{"profileId": "example"})
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 without a configured catalog root", rec.Code)
	}
}

func TestReviewManifestAnalysisComputesDigestAndEffects(t *testing.T) {
	catalogRoot := t.TempDir()
	handler, token, _ := newTestHandlerWithOptions(t, Options{CatalogRoot: catalogRoot})

	body := map[string]any{
		"manifest": model.Manifest{
			Version: model.SchemaVersion, Kind: "import-plan", OperationID: "import-test",
			Actions: []model.Action{{Action: "copy", DestinationRoot: "catalog", DestinationPath: "new.bin", SourceSHA256: strings.Repeat("a", 64), SourceSize: 5}},
		},
	}
	rec := mutate(t, handler, token, "POST", "/api/review/manifest-analysis", body)
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var analysis review.ManifestAnalysis
	if err := json.Unmarshal(rec.Body.Bytes(), &analysis); err != nil {
		t.Fatal(err)
	}
	if analysis.ManifestDigest == "" {
		t.Fatal("expected a non-empty manifest digest")
	}
	if len(analysis.Actions) != 1 || analysis.Actions[0].Effect != review.EffectCreate {
		t.Fatalf("unexpected analysis: %+v", analysis)
	}
}

// TestReviewManifestAnalysisRejectsClientSuppliedDestinationRoot covers
// issue #2: the manifest-analysis request body no longer accepts a
// destinationRoot field at all — sending one must fail cleanly (unknown
// field), never be silently accepted and used to probe an arbitrary local
// path.
func TestReviewManifestAnalysisRejectsClientSuppliedDestinationRoot(t *testing.T) {
	handler, token, _ := newTestHandlerWithOptions(t, Options{CatalogRoot: t.TempDir()})
	rec := mutate(t, handler, token, "POST", "/api/review/manifest-analysis", map[string]any{
		"manifest":        model.Manifest{Version: model.SchemaVersion, Kind: "import-plan", OperationID: "x"},
		"destinationRoot": "C:\\Windows\\System32",
	})
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 for a client-supplied destinationRoot", rec.Code)
	}
}

func TestDestinationRootResolverReservesCatalog(t *testing.T) {
	handlers := &handlers{opts: Options{}}
	cfg := model.Config{
		Roots: []model.Root{
			{ID: "catalog", Kind: "generic", Path: t.TempDir()},
			{ID: "other", Kind: "catalog", Path: t.TempDir()},
		},
	}
	if _, ok := handlers.destinationRootResolver(cfg)["catalog"]; ok {
		t.Fatal("catalog destination must not resolve from configured scan roots")
	}

	canonical := t.TempDir()
	handlers.opts.CatalogRoot = canonical
	if got := handlers.destinationRootResolver(cfg)["catalog"]; got != canonical {
		t.Fatalf("catalog destination = %q, want explicit catalog root %q", got, canonical)
	}
}

// TestReviewManifestAnalysisResolvesAdapterRootFromActiveConfig covers
// issue #2: an adapter destination root is resolved only from the active
// configuration's own roots (matched by id or kind), never guessed.
func TestReviewManifestAnalysisResolvesAdapterRootFromActiveConfig(t *testing.T) {
	handler, token, paths := newTestHandlerWithOptions(t, Options{})
	steamRoot := t.TempDir()
	writeActiveConfigForTest(t, paths, model.Config{
		Version: model.SchemaVersion,
		Roots:   []model.Root{{ID: "steam", Kind: "generic", Path: steamRoot}},
		Policy:  model.PolicyFile{Version: model.SchemaVersion, Default: "tracked-external"},
	})

	body := map[string]any{
		"manifest": model.Manifest{
			Version: model.SchemaVersion, Kind: "steam-export-plan", OperationID: "steam-test",
			Actions: []model.Action{{Action: "copy", DestinationRoot: "steam", DestinationPath: "123.png", SourceSHA256: strings.Repeat("a", 64), SourceSize: 5}},
		},
	}
	rec := mutate(t, handler, token, "POST", "/api/review/manifest-analysis", body)
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var analysis review.ManifestAnalysis
	if err := json.Unmarshal(rec.Body.Bytes(), &analysis); err != nil {
		t.Fatal(err)
	}
	if len(analysis.Actions) != 1 || analysis.Actions[0].Effect != review.EffectCreate {
		t.Fatalf("expected the steam root to resolve from the active config: %+v", analysis)
	}
}

// TestReviewManifestAnalysisUnconfiguredAdapterRootBecomesConflict covers
// issue #2's explicit-conflict-not-server-error requirement at the API
// layer.
func TestReviewManifestAnalysisUnconfiguredAdapterRootBecomesConflict(t *testing.T) {
	handler, token, _ := newTestHandlerWithOptions(t, Options{})
	body := map[string]any{
		"manifest": model.Manifest{
			Version: model.SchemaVersion, Kind: "playnite-export-plan", OperationID: "playnite-test",
			Actions: []model.Action{{Action: "copy", DestinationRoot: "playnite", DestinationPath: "games/x/Logo.png", SourceSHA256: strings.Repeat("a", 64), SourceSize: 5}},
		},
	}
	rec := mutate(t, handler, token, "POST", "/api/review/manifest-analysis", body)
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var analysis review.ManifestAnalysis
	if err := json.Unmarshal(rec.Body.Bytes(), &analysis); err != nil {
		t.Fatal(err)
	}
	if len(analysis.Actions) != 1 || analysis.Actions[0].Effect != review.EffectRootUnavailable {
		t.Fatalf("expected an unresolved-root conflict, got %+v", analysis)
	}
}

func TestReviewGatesEnforceSequencingAndNonExecutableGateC(t *testing.T) {
	handler, token, paths := newTestHandlerWithOptions(t, Options{})

	// Gate B without a Gate A reference is rejected.
	rec := mutate(t, handler, token, "POST", "/api/review/gates/b", map[string]any{"policyDigest": "p"})
	if rec.Code != 400 {
		t.Fatalf("gate B without gate A: status = %d, want 400", rec.Code)
	}

	rec = mutate(t, handler, token, "POST", "/api/review/gates/a", map[string]any{"inventoryDigest": "inv", "identityDigest": "id"})
	if rec.Code != 200 {
		t.Fatalf("gate A: status = %d: %s", rec.Code, rec.Body.String())
	}
	var gateA struct {
		Review   review.GateAReview `json:"review"`
		Artifact string             `json:"artifact"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &gateA); err != nil {
		t.Fatal(err)
	}
	if gateA.Artifact == "" || strings.Contains(gateA.Artifact, paths.Root) {
		t.Fatalf("gate A artifact reference must be a non-empty relative reference, got %q", gateA.Artifact)
	}

	rec = mutate(t, handler, token, "POST", "/api/review/gates/b", map[string]any{
		"gateAId": gateA.Review.ID, "policyDigest": "policy-digest",
	})
	if rec.Code != 200 {
		t.Fatalf("gate B: status = %d: %s", rec.Code, rec.Body.String())
	}
	var gateB struct {
		Review review.GateBReview `json:"review"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &gateB); err != nil {
		t.Fatal(err)
	}

	// Gate C without a Gate B reference is rejected.
	rec = mutate(t, handler, token, "POST", "/api/review/gates/c", map[string]any{
		"analysis": review.ManifestAnalysis{ManifestDigest: "m"},
	})
	if rec.Code != 400 {
		t.Fatalf("gate C without gate B: status = %d, want 400", rec.Code)
	}

	// Gate C's analysis must be tied to a manifest this workspace actually
	// persisted (issue #3): persist one via the plan-export endpoint and
	// use its own analysis digest.
	if _, err := workspace.SaveProfileDraft(paths, "", exampleReviewProfile("gatectest")); err != nil {
		t.Fatal(err)
	}
	planRec := mutate(t, handler, token, "POST", "/api/review/plans/export", map[string]any{
		"profileId": "gatectest", "adapter": "steam",
	})
	if planRec.Code != 200 {
		t.Fatalf("plan export: status = %d: %s", planRec.Code, planRec.Body.String())
	}
	var planView planPersistedView
	if err := json.Unmarshal(planRec.Body.Bytes(), &planView); err != nil {
		t.Fatal(err)
	}
	analysis := review.ManifestAnalysis{ManifestDigest: mustDigest(t, planView.Plan)}

	// Gate C, even asking for executable:true, must always come back false.
	rec = mutate(t, handler, token, "POST", "/api/review/gates/c", map[string]any{
		"gateBId":    gateB.Review.ID,
		"analysis":   analysis,
		"executable": true,
	})
	if rec.Code != 200 {
		t.Fatalf("gate C: status = %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"executable":true`) {
		t.Fatalf("gate C response must never report executable:true: %s", rec.Body.String())
	}
	var gateC struct {
		Review   review.GateCReview `json:"review"`
		Artifact string             `json:"artifact"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &gateC); err != nil {
		t.Fatal(err)
	}
	if gateC.Review.Executable {
		t.Fatal("expected Gate C to always be non-executable")
	}
	if gateC.Artifact == "" || strings.Contains(gateC.Artifact, paths.Root) {
		t.Fatalf("gate C artifact reference must be a non-empty relative reference, got %q", gateC.Artifact)
	}
}

func mustDigest(t *testing.T, plan model.Manifest) string {
	t.Helper()
	digest, err := manifest.Digest(plan)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

// TestGateSequencingRejectsForgedAndNonexistentGateReferencesAtTheAPI
// covers issue #3 at the HTTP layer: a syntactically well-shaped but never
// persisted prior-gate reference, and a reference belonging to the wrong
// gate letter, must both be rejected.
func TestGateSequencingRejectsForgedAndNonexistentGateReferencesAtTheAPI(t *testing.T) {
	handler, token, _ := newTestHandlerWithOptions(t, Options{})

	rec := mutate(t, handler, token, "POST", "/api/review/gates/b", map[string]any{
		"gateAId": "gate-a-" + strings.Repeat("0", 32), "policyDigest": "p",
	})
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 for a nonexistent gate A reference", rec.Code)
	}

	rec = mutate(t, handler, token, "POST", "/api/review/gates/a", map[string]any{"inventoryDigest": "inv"})
	if rec.Code != 200 {
		t.Fatalf("gate A: status = %d: %s", rec.Code, rec.Body.String())
	}
	var gateA struct {
		Review review.GateAReview `json:"review"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &gateA); err != nil {
		t.Fatal(err)
	}

	// A real Gate A id used where a Gate B id is required for Gate C.
	rec = mutate(t, handler, token, "POST", "/api/review/gates/c", map[string]any{
		"gateBId":  gateA.Review.ID,
		"analysis": review.ManifestAnalysis{ManifestDigest: "m"},
	})
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 when a gate A id is supplied where a gate B id is required", rec.Code)
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
