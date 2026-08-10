package workspace

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDefaultRootWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only default root layout")
	}
	t.Setenv("LOCALAPPDATA", `C:\Users\tester\AppData\Local`)
	root, err := DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(`C:\Users\tester\AppData\Local`, "gamelib")
	if root != want {
		t.Fatalf("root = %q, want %q", root, want)
	}
}

func TestDefaultRootWindowsFallsBackToAppData(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only default root layout")
	}
	t.Setenv("LOCALAPPDATA", "")
	t.Setenv("APPDATA", `C:\Users\tester\AppData\Roaming`)
	root, err := DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(`C:\Users\tester\AppData\Roaming`, "gamelib")
	if root != want {
		t.Fatalf("root = %q, want %q", root, want)
	}
}

func TestDefaultRootLinuxUsesXDGConfigHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX-only default root layout")
	}
	t.Setenv("XDG_CONFIG_HOME", "/home/tester/.config")
	root, err := DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/home/tester/.config", "gamelib")
	if root != want {
		t.Fatalf("root = %q, want %q", root, want)
	}
}

func TestDefaultRootLinuxFallsBackToHomeConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX-only default root layout")
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	root, err := DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".config", "gamelib")
	if root != want {
		t.Fatalf("root = %q, want %q", root, want)
	}
}

func TestNewPathsLayout(t *testing.T) {
	paths := NewPaths(filepath.Join("root"))
	if paths.Config != filepath.Join("root", "config", "active.json") {
		t.Fatalf("unexpected config path: %q", paths.Config)
	}
	if paths.Drafts != filepath.Join("root", "drafts") {
		t.Fatalf("unexpected drafts dir: %q", paths.Drafts)
	}
	if paths.Artifacts != filepath.Join("root", "artifacts") {
		t.Fatalf("unexpected artifacts dir: %q", paths.Artifacts)
	}
}

func TestDefaultUsesDefaultRoot(t *testing.T) {
	paths, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if paths.Root == "" {
		t.Fatal("expected a non-empty default root")
	}
	if _, err := os.Stat(paths.Root); err == nil {
		t.Skip("default root already exists on this machine; skip create assertion")
	}
}
