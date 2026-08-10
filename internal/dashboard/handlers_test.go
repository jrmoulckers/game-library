package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
)

func mutate(t *testing.T, handler http.Handler, token, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := newRequest(method, path)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+testHost)
	req.Header.Set(csrfHeader, token)
	req.Body = httpBody(string(data))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func fetch(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newRequest("GET", path))
	return rec
}

func TestHealthAndBootstrap(t *testing.T) {
	handler, token := newTestHandler(t)

	rec := fetch(t, handler, "/healthz")
	if rec.Code != 200 {
		t.Fatalf("healthz status = %d", rec.Code)
	}

	rec = fetch(t, handler, "/api/bootstrap")
	if rec.Code != 200 {
		t.Fatalf("bootstrap status = %d", rec.Code)
	}
	var boot bootstrapView
	if err := json.Unmarshal(rec.Body.Bytes(), &boot); err != nil {
		t.Fatal(err)
	}
	if boot.CSRFToken != token {
		t.Fatalf("bootstrap token %q != issued token %q", boot.CSRFToken, token)
	}
	if boot.ToolVersion != model.ToolVersion {
		t.Fatalf("bootstrap tool version %q != %q", boot.ToolVersion, model.ToolVersion)
	}
}

func TestGetConfigBeforeAnyWriteReportsAbsence(t *testing.T) {
	handler, _ := newTestHandler(t)
	rec := fetch(t, handler, "/api/config")
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	var view configView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Exists {
		t.Fatal("expected exists=false before any config write")
	}
}

func exampleConfig() model.Config {
	return model.Config{
		Version: model.SchemaVersion,
		Roots: []model.Root{
			{ID: "source", Kind: "generic", Path: "D:\\GamingProfiles"},
		},
		Policy: model.PolicyFile{Version: model.SchemaVersion, Default: "tracked-external"},
	}
}

func TestPutThenGetConfigRoundTrips(t *testing.T) {
	handler, token := newTestHandler(t)
	cfg := exampleConfig()

	rec := mutate(t, handler, token, "PUT", "/api/config", configRequest{BaseDigest: "", Config: cfg})
	if rec.Code != 200 {
		t.Fatalf("PUT config status = %d: %s", rec.Code, rec.Body.String())
	}

	rec = fetch(t, handler, "/api/config")
	var view configView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !view.Exists || view.Config == nil {
		t.Fatal("expected the config to exist after a PUT")
	}
	if view.Config.Roots[0].ID != "source" {
		t.Fatalf("unexpected roots: %+v", view.Config.Roots)
	}
}

func TestPutConfigRejectsInvalidConfig(t *testing.T) {
	handler, token := newTestHandler(t)
	invalid := model.Config{Version: model.SchemaVersion} // no roots
	rec := mutate(t, handler, token, "PUT", "/api/config", configRequest{Config: invalid})
	if rec.Code != 400 {
		t.Fatalf("expected 400 for an invalid config, got %d", rec.Code)
	}
}

func TestPutConfigRequiresEmptyBaseDigestWhenAbsent(t *testing.T) {
	handler, token := newTestHandler(t)
	rec := mutate(t, handler, token, "PUT", "/api/config", configRequest{BaseDigest: "stale", Config: exampleConfig()})
	if rec.Code != 409 {
		t.Fatalf("expected 409 for a non-empty base digest with no existing config, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPutConfigRejectsStaleBaseDigest(t *testing.T) {
	handler, token := newTestHandler(t)
	cfg := exampleConfig()

	rec := mutate(t, handler, token, "PUT", "/api/config", configRequest{Config: cfg})
	if rec.Code != 200 {
		t.Fatalf("initial PUT config status = %d: %s", rec.Code, rec.Body.String())
	}

	rec = mutate(t, handler, token, "PUT", "/api/config", configRequest{BaseDigest: "not-the-current-digest", Config: cfg})
	if rec.Code != 409 {
		t.Fatalf("expected 409 for a stale base digest, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPutConfigAcceptsCurrentBaseDigest(t *testing.T) {
	handler, token := newTestHandler(t)
	cfg := exampleConfig()

	rec := mutate(t, handler, token, "PUT", "/api/config", configRequest{Config: cfg})
	if rec.Code != 200 {
		t.Fatalf("initial PUT config status = %d: %s", rec.Code, rec.Body.String())
	}
	var view configView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}

	cfg.Roots = append(cfg.Roots, model.Root{ID: "second", Kind: "generic", Path: "E:\\More"})
	rec = mutate(t, handler, token, "PUT", "/api/config", configRequest{BaseDigest: view.Digest, Config: cfg})
	if rec.Code != 200 {
		t.Fatalf("expected 200 for a correct base digest, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetConfigIsSideEffectFree(t *testing.T) {
	handler, _ := newTestHandler(t)
	before := fetch(t, handler, "/api/config").Body.String()
	fetch(t, handler, "/api/config")
	fetch(t, handler, "/api/config")
	after := fetch(t, handler, "/api/config").Body.String()
	if before != after {
		t.Fatalf("GET /api/config must be side-effect-free; got %q then %q", before, after)
	}
}

func examplePolicyFile() model.PolicyFile {
	return model.PolicyFile{
		Version: model.SchemaVersion,
		Default: "tracked-external",
		Rules: []model.PolicyRule{
			{Source: "canonical-catalog", Mode: "managed"},
		},
	}
}

type policyDraftRequestBody struct {
	BaseDigest string           `json:"baseDigest"`
	Policy     model.PolicyFile `json:"policy"`
}

func TestPolicyDraftRoundTripAndConflict(t *testing.T) {
	handler, token := newTestHandler(t)

	rec := fetch(t, handler, "/api/drafts/policy")
	var view policyDraftView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Exists {
		t.Fatal("expected no policy draft before any save")
	}

	rec = mutate(t, handler, token, "PUT", "/api/drafts/policy", policyDraftRequestBody{
		BaseDigest: "", Policy: examplePolicyFile(),
	})
	if rec.Code != 200 {
		t.Fatalf("PUT policy draft status = %d: %s", rec.Code, rec.Body.String())
	}

	// Stale digest must conflict.
	rec = mutate(t, handler, token, "PUT", "/api/drafts/policy", policyDraftRequestBody{
		BaseDigest: "stale", Policy: examplePolicyFile(),
	})
	if rec.Code != 409 {
		t.Fatalf("expected 409 for a stale base digest, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPolicyDraftRejectsInvalidPolicy(t *testing.T) {
	handler, token := newTestHandler(t)
	invalid := model.PolicyFile{Version: model.SchemaVersion, Default: "not-a-mode"}
	rec := mutate(t, handler, token, "PUT", "/api/drafts/policy", policyDraftRequestBody{Policy: invalid})
	if rec.Code != 400 {
		t.Fatalf("expected 400 for an invalid policy, got %d", rec.Code)
	}
}

type profileDraftRequestBody struct {
	BaseDigest string        `json:"baseDigest"`
	Profile    model.Profile `json:"profile"`
}

func exampleProfile(id string) model.Profile {
	return model.Profile{
		Version: model.SchemaVersion,
		ID:      id,
		Name:    "Example profile",
		Games: []model.ProfileGame{
			{
				ID: "steam:123",
				Assets: map[string]model.AssetSelection{
					"grid": {SHA256: strings.Repeat("a", 64), Extension: "png"},
				},
			},
		},
	}
}

func TestProfileDraftRoundTripAndConflict(t *testing.T) {
	handler, token := newTestHandler(t)

	rec := fetch(t, handler, "/api/drafts/profiles/example")
	var view profileDraftView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Exists {
		t.Fatal("expected no profile draft before any save")
	}

	rec = mutate(t, handler, token, "PUT", "/api/drafts/profiles/example", profileDraftRequestBody{
		BaseDigest: "", Profile: exampleProfile("example"),
	})
	if rec.Code != 200 {
		t.Fatalf("PUT profile draft status = %d: %s", rec.Code, rec.Body.String())
	}

	rec = mutate(t, handler, token, "PUT", "/api/drafts/profiles/example", profileDraftRequestBody{
		BaseDigest: "stale", Profile: exampleProfile("example"),
	})
	if rec.Code != 409 {
		t.Fatalf("expected 409 for a stale base digest, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestProfileDraftRejectsMismatchedID(t *testing.T) {
	handler, token := newTestHandler(t)
	rec := mutate(t, handler, token, "PUT", "/api/drafts/profiles/example", profileDraftRequestBody{
		Profile: exampleProfile("different"),
	})
	if rec.Code != 400 {
		t.Fatalf("expected 400 for a mismatched profile id, got %d", rec.Code)
	}
}

func TestProfileDraftRejectsUnsafeID(t *testing.T) {
	handler, _ := newTestHandler(t)
	rec := fetch(t, handler, "/api/drafts/profiles/con")
	if rec.Code != 400 {
		t.Fatalf("expected 400 for a reserved device-name profile id, got %d", rec.Code)
	}
}

func TestGetDraftsAreSideEffectFree(t *testing.T) {
	handler, _ := newTestHandler(t)
	before := fetch(t, handler, "/api/drafts/policy").Body.String()
	fetch(t, handler, "/api/drafts/policy")
	after := fetch(t, handler, "/api/drafts/policy").Body.String()
	if before != after {
		t.Fatalf("GET /api/drafts/policy must be side-effect-free")
	}
}

func TestIndexAndStaticAssetServe(t *testing.T) {
	handler, _ := newTestHandler(t)
	rec := fetch(t, handler, "/")
	if rec.Code != 200 {
		t.Fatalf("index status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "gamelib artwork organizer") {
		t.Fatalf("index body missing expected shell content: %s", rec.Body.String())
	}

	rec = fetch(t, handler, "/static/app.css")
	if rec.Code != 200 {
		t.Fatalf("static asset status = %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("unexpected content type: %s", rec.Header().Get("Content-Type"))
	}
}
