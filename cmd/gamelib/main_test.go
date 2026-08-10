package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/report"
	"github.com/jrmoulckers/game-library/internal/workspace"
)

func TestInventoryScanSanitizesPaths(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "private-name.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "config.json")
	outputPath := filepath.Join(t.TempDir(), "report.json")
	cfg := model.Config{
		Version: 1,
		Roots:   []model.Root{{ID: "source", Kind: "generic", Path: root}},
		Policy:  model.PolicyFile{Version: 1, Default: "tracked-external"},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{
		"inventory", "scan", "--config", configPath, "--output", outputPath, "--privacy", "sanitized",
	}); err != nil {
		t.Fatal(err)
	}
	report, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(report)
	if strings.Contains(text, root) || strings.Contains(text, "private-name.txt") {
		t.Fatalf("sanitized report leaked a source path: %s", text)
	}
}

func TestServeRejectsNonLoopbackListenWithoutBlocking(t *testing.T) {
	// This must fail fast during flag/address validation, before any
	// listener is bound or Serve is called, so the test never blocks.
	if err := run([]string{"serve", "--listen", "0.0.0.0:8787"}); err == nil {
		t.Fatal("expected gamelib serve to reject a wildcard --listen address")
	}
}

func TestServeRejectsHostnameListenWithoutBlocking(t *testing.T) {
	if err := run([]string{"serve", "--listen", "localhost:8787"}); err == nil {
		t.Fatal("expected gamelib serve to reject a hostname --listen address")
	}
}

func TestNewServeServerBindsWithExplicitWorkspaceAndConfig(t *testing.T) {
	workspaceRoot := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "custom-active.json")

	srv, err := newServeServer([]string{
		"--listen", "127.0.0.1:0",
		"--workspace", workspaceRoot,
		"--config", configPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	done := make(chan error, 1)
	go func() { done <- srv.Serve() }()
	defer func() {
		srv.Close()
		<-done
	}()

	resp, err := http.Get("http://" + srv.Addr() + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestNewServeServerUsesPlatformDefaultWorkspaceWhenUnset(t *testing.T) {
	srv, err := newServeServer([]string{"--listen", "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	srv.Close()
}

func TestNewServeServerWiresInventoryReportAndCatalogFlags(t *testing.T) {
	workspaceRoot := t.TempDir()
	reportPath := filepath.Join(t.TempDir(), "inventory.json")
	catalogRoot := t.TempDir()

	reportInventory := model.Inventory{
		Version: model.SchemaVersion, Privacy: "private", CreatedAt: "2026-01-01T00:00:00Z",
		Observations: []model.Observation{{RootID: "source", RelativePath: "a.png", SHA256: strings.Repeat("a", 64)}},
	}
	if err := report.WriteJSON(reportPath, reportInventory); err != nil {
		t.Fatal(err)
	}

	// The dashboard's review surface requires an active configuration even
	// when loading from a report (the report supplies observations; the
	// active config still supplies policy and root identity), so write
	// one directly via the workspace package the same way the dashboard
	// would.
	paths := workspace.NewPaths(workspaceRoot)
	cfg := model.Config{
		Version: model.SchemaVersion,
		Roots:   []model.Root{{ID: "source", Kind: "generic", Path: t.TempDir()}},
		Policy:  model.PolicyFile{Version: model.SchemaVersion, Default: "tracked-external"},
	}
	if err := workspace.WriteActiveConfig(paths.Config, "", cfg); err != nil {
		t.Fatal(err)
	}

	srv, err := newServeServer([]string{
		"--listen", "127.0.0.1:0",
		"--workspace", workspaceRoot,
		"--inventory-report", reportPath,
		"--catalog", catalogRoot,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	done := make(chan error, 1)
	go func() { done <- srv.Serve() }()
	defer func() {
		srv.Close()
		<-done
	}()

	resp, err := http.Get("http://" + srv.Addr() + "/api/review/overview")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var overview struct {
		Source string `json:"source"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&overview); err != nil {
		t.Fatal(err)
	}
	if overview.Source != "report" {
		t.Fatalf("expected the dashboard to load from the --inventory-report file, got source=%q", overview.Source)
	}
}
