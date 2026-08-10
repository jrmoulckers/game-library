package workspace

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/jrmoulckers/game-library/internal/manifest"
	"github.com/jrmoulckers/game-library/internal/model"
)

func exampleConfig() model.Config {
	return model.Config{
		Version: model.SchemaVersion,
		Roots: []model.Root{
			{ID: "source", Kind: "generic", Path: filepath.Join("D:", "GamingProfiles")},
		},
		Policy: model.PolicyFile{Version: model.SchemaVersion, Default: "tracked-external"},
	}
}

func TestLoadActiveConfigReportsAbsence(t *testing.T) {
	dir := t.TempDir()
	_, found, err := LoadActiveConfig(filepath.Join(dir, "active.json"))
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expected found=false before any config has been written")
	}
}

func TestWriteAndLoadActiveConfigRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config", "active.json")
	cfg := exampleConfig()
	if err := WriteActiveConfig(path, "", cfg); err != nil {
		t.Fatal(err)
	}
	loaded, found, err := LoadActiveConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected found=true after writing a config")
	}
	if loaded.Roots[0].ID != cfg.Roots[0].ID {
		t.Fatalf("round trip mismatch: %+v", loaded)
	}
}

func TestWriteActiveConfigRejectsInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "active.json")
	invalid := model.Config{Version: model.SchemaVersion} // no roots
	if err := WriteActiveConfig(path, "", invalid); err == nil {
		t.Fatal("expected an error for a config with no roots")
	}
	if _, found, _ := LoadActiveConfig(path); found {
		t.Fatal("an invalid config write must not create a file")
	}
}

func TestWriteActiveConfigOverwritesAtomically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "active.json")
	cfg := exampleConfig()
	if err := WriteActiveConfig(path, "", cfg); err != nil {
		t.Fatal(err)
	}
	digest, err := manifest.Digest(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Roots = append(cfg.Roots, model.Root{ID: "second", Kind: "generic", Path: filepath.Join("E:", "More")})
	if err := WriteActiveConfig(path, digest, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, _, err := LoadActiveConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Roots) != 2 {
		t.Fatalf("expected 2 roots after overwrite, got %d", len(loaded.Roots))
	}
}

func TestWriteActiveConfigRequiresEmptyBaseDigestWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "active.json")
	if err := WriteActiveConfig(path, "stale", exampleConfig()); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict for a non-empty base digest with no existing config, got %v", err)
	}
	if _, found, _ := LoadActiveConfig(path); found {
		t.Fatal("a rejected write must not create a file")
	}
}

func TestWriteActiveConfigRejectsStaleBaseDigest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "active.json")
	cfg := exampleConfig()
	if err := WriteActiveConfig(path, "", cfg); err != nil {
		t.Fatal(err)
	}
	if err := WriteActiveConfig(path, "not-the-current-digest", cfg); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict for a stale base digest, got %v", err)
	}
	// The on-disk config must remain untouched after a rejected write.
	loaded, _, err := LoadActiveConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Roots) != 1 {
		t.Fatalf("a rejected stale write must not modify the stored config: %+v", loaded)
	}
}
