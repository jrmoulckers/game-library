package review

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListSafeThemeIDsEnumeratesOnlyDirectoriesWithATheme(t *testing.T) {
	catalog := t.TempDir()
	themesDir := filepath.Join(catalog, "library", "themes")
	mustMkdirAll(t, filepath.Join(themesDir, "retro-neon"))
	mustWriteFile(t, filepath.Join(themesDir, "retro-neon", "theme.json"), `{"id":"retro-neon"}`)
	mustMkdirAll(t, filepath.Join(themesDir, "incomplete"))
	// "incomplete" has no theme.json and must not be listed.
	mustMkdirAll(t, filepath.Join(themesDir, "..unsafe"))

	ids, err := ListSafeThemeIDs(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "retro-neon" {
		t.Fatalf("ids = %+v, want [retro-neon]", ids)
	}
}

func TestListSafeThemeIDsReturnsEmptyForMissingThemesDirectory(t *testing.T) {
	ids, err := ListSafeThemeIDs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected no themes, got %+v", ids)
	}
}

func TestListSafeThemeIDsRequiresACatalogRoot(t *testing.T) {
	if _, err := ListSafeThemeIDs(""); err == nil {
		t.Fatal("expected an error for an empty catalog root")
	}
}

func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}
