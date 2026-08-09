package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
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
