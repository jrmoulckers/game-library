package dashboard

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/publishing"
	"github.com/jrmoulckers/game-library/internal/topology"
	"github.com/jrmoulckers/game-library/internal/workspace"
)

func publishingCall(t *testing.T, handler http.Handler, token, endpoint string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := newRequest(http.MethodPost, endpoint)
	req.Body = httpBody(string(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+testHost)
	req.Header.Set(csrfHeader, token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestPublishingAPIExactApprovalAndRecovery(t *testing.T) {
	handler, token, paths := newTestHandlerWithOptions(t, Options{})
	root, destination := t.TempDir(), t.TempDir()
	grid := filepath.Join(root, "artwork", "custom", "grid")
	if err := os.MkdirAll(grid, 0o700); err != nil {
		t.Fatal(err)
	}
	var artwork bytes.Buffer
	if err := png.Encode(&artwork, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"123.png", "123p.png"} {
		if err := os.WriteFile(filepath.Join(grid, name), artwork.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(destination, "123.png"), []byte("predecessor"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := model.Config{Version: 1, Roots: []model.Root{{ID: "catalog", Kind: "decky-catalog", Path: root}},
		Policy: model.PolicyFile{Version: 1, Default: "tracked-external"}}
	if err := workspace.WriteActiveConfig(paths.Config, "", cfg); err != nil {
		t.Fatal(err)
	}
	doc := topology.Default()
	doc.Profiles[0].Artwork = "custom"
	if err := topology.Save(paths.Topology, doc); err != nil {
		t.Fatal(err)
	}
	targets, err := publishing.LoadTargets(paths, doc)
	if err != nil {
		t.Fatal(err)
	}
	targets.Targets[0].Root = destination
	if err := workspace.WriteLocalJSON(publishing.TargetPath(paths), targets); err != nil {
		t.Fatal(err)
	}
	request := publishingRequest{Profile: "steam/standard", Target: targets.Targets[0].ID}
	rec := publishingCall(t, handler, "", "/api/publishing/preview", request)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("preview bypassed CSRF: %d", rec.Code)
	}
	rec = publishingCall(t, handler, token, "/api/publishing/preview", request)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", rec.Code, rec.Body.String())
	}
	var view struct {
		Plan publishing.Plan `json:"plan"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Plan.Different != 1 || view.Plan.Missing != 1 {
		t.Fatalf("parity = %+v", view.Plan)
	}
	rec = publishingCall(t, handler, token, "/api/publishing/publish", request)
	if rec.Code != http.StatusConflict {
		t.Fatalf("unapproved publish: %d", rec.Code)
	}
	request.Approval = view.Plan.Digest
	rec = publishingCall(t, handler, token, "/api/publishing/stage", request)
	if rec.Code != http.StatusOK {
		t.Fatalf("stage: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(destination, "123p.png")); !os.IsNotExist(err) {
		t.Fatal("stage mutated frontend")
	}
	for range 2 {
		rec = publishingCall(t, handler, token, "/api/publishing/publish", request)
		if rec.Code != http.StatusOK {
			t.Fatalf("publish/idempotent repeat: %d %s", rec.Code, rec.Body.String())
		}
	}
	request.Operation = view.Plan.Digest
	rec = publishingCall(t, handler, token, "/api/publishing/rollback-preview", request)
	if rec.Code != http.StatusOK {
		t.Fatalf("rollback preview: %d %s", rec.Code, rec.Body.String())
	}
	var rollback publishing.Rollback
	if err := json.Unmarshal(rec.Body.Bytes(), &rollback); err != nil {
		t.Fatal(err)
	}
	request.Approval = rollback.Plan.Digest
	rec = publishingCall(t, handler, token, "/api/publishing/rollback", request)
	if rec.Code != http.StatusOK {
		t.Fatalf("rollback: %d %s", rec.Code, rec.Body.String())
	}
	data, err := os.ReadFile(filepath.Join(destination, "123.png"))
	if err != nil || string(data) != "predecessor" {
		t.Fatal("API rollback did not restore predecessor")
	}
	if _, err := os.Stat(filepath.Join(destination, "123p.png")); err != nil {
		t.Fatal("API rollback deleted added artwork")
	}
	request.Profile = "playnite/standard"
	rec = publishingCall(t, handler, token, "/api/publishing/preview", request)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "platforms must match") {
		t.Fatalf("cross-platform export accepted: %d %s", rec.Code, rec.Body.String())
	}
}
