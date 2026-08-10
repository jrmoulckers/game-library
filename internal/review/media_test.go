package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
)

func snapshotWithSingleFile(t *testing.T, rootID, relPath, content string) (Snapshot, string) {
	t.Helper()
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, filepath.FromSlash(relPath)), content)
	observation := model.Observation{RootID: rootID, RelativePath: relPath, Size: int64(len(content)), SHA256: sha256Like(0)}
	snapshot := Snapshot{
		Inventory: model.Inventory{Observations: []model.Observation{observation}},
		Roots:     map[string]model.Root{rootID: {ID: rootID, Kind: "generic", Path: root}},
		index:     map[string]model.Observation{ObservationID(rootID, relPath): observation},
	}
	return snapshot, root
}

func TestResolveMediaServesARegularFile(t *testing.T) {
	snapshot, _ := snapshotWithSingleFile(t, "source", "grid/123.png", pngContent)
	resolution, err := ResolveMedia(snapshot, ObservationID("source", "grid/123.png"))
	if err != nil {
		t.Fatal(err)
	}
	if resolution.MIME != "image/png" {
		t.Fatalf("MIME = %q, want image/png", resolution.MIME)
	}
	if resolution.Size != int64(len(pngContent)) {
		t.Fatalf("Size = %d, want %d", resolution.Size, len(pngContent))
	}
}

func TestResolveMediaRejectsUnknownID(t *testing.T) {
	snapshot, _ := snapshotWithSingleFile(t, "source", "grid/123.png", pngContent)
	if _, err := ResolveMedia(snapshot, "not-a-real-id"); err == nil {
		t.Fatal("expected an error for an unknown observation id")
	}
}

func TestResolveMediaNeverAcceptsARawPath(t *testing.T) {
	snapshot, root := snapshotWithSingleFile(t, "source", "grid/123.png", pngContent)
	// The literal relative path (not the derived ID) must not resolve to
	// anything: media is addressed only by ObservationID.
	if _, err := ResolveMedia(snapshot, "grid/123.png"); err == nil {
		t.Fatal("expected a raw relative path to be rejected as an unknown id")
	}
	if _, err := ResolveMedia(snapshot, filepath.Join(root, "grid", "123.png")); err == nil {
		t.Fatal("expected a raw absolute path to be rejected as an unknown id")
	}
}

func TestResolveMediaRejectsTraversalInjectedIntoTheIndex(t *testing.T) {
	// Defense-in-depth: even if something upstream ever produced an
	// observation whose RelativePath contains a traversal segment, the
	// containment check inside ResolveMedia (reusing
	// workspace.Contain) must still refuse to serve outside the root.
	root := t.TempDir()

	observation := model.Observation{RootID: "source", RelativePath: "../secret.txt", SHA256: sha256Like(0)}
	id := ObservationID("source", "../secret.txt")
	snapshot := Snapshot{
		Inventory: model.Inventory{Observations: []model.Observation{observation}},
		Roots:     map[string]model.Root{"source": {ID: "source", Kind: "generic", Path: filepath.Join(root, "sub")}},
		index:     map[string]model.Observation{id: observation},
	}
	if _, err := ResolveMedia(snapshot, id); err == nil {
		t.Fatal("expected a traversal relative path to be rejected")
	}
}

func TestResolveMediaRejectsMissingRoot(t *testing.T) {
	observation := model.Observation{RootID: "gone", RelativePath: "a.png", SHA256: sha256Like(0)}
	snapshot := Snapshot{
		Inventory: model.Inventory{Observations: []model.Observation{observation}},
		Roots:     map[string]model.Root{},
		index:     map[string]model.Observation{ObservationID("gone", "a.png"): observation},
	}
	if _, err := ResolveMedia(snapshot, ObservationID("gone", "a.png")); err == nil {
		t.Fatal("expected an error when the observation's root is no longer configured")
	}
}

func TestResolveMediaRejectsMissingFile(t *testing.T) {
	root := t.TempDir()
	observation := model.Observation{RootID: "source", RelativePath: "missing.png", SHA256: sha256Like(0)}
	snapshot := Snapshot{
		Inventory: model.Inventory{Observations: []model.Observation{observation}},
		Roots:     map[string]model.Root{"source": {ID: "source", Kind: "generic", Path: root}},
		index:     map[string]model.Observation{ObservationID("source", "missing.png"): observation},
	}
	if _, err := ResolveMedia(snapshot, ObservationID("source", "missing.png")); err == nil {
		t.Fatal("expected an error for a file that no longer exists")
	}
}

func TestResolveMediaRejectsSymlinkLeaf(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "real.png")
	mustWriteFile(t, target, pngContent)
	link := filepath.Join(root, "link.png")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink creation is not permitted in this environment: %v", err)
	}
	observation := model.Observation{RootID: "source", RelativePath: "link.png", SHA256: sha256Like(0)}
	snapshot := Snapshot{
		Inventory: model.Inventory{Observations: []model.Observation{observation}},
		Roots:     map[string]model.Root{"source": {ID: "source", Kind: "generic", Path: root}},
		index:     map[string]model.Observation{ObservationID("source", "link.png"): observation},
	}
	if _, err := ResolveMedia(snapshot, ObservationID("source", "link.png")); err == nil {
		t.Fatal("expected a symlink leaf to be rejected even though it was not a symlink at scan time")
	}
}

func TestResolveMediaRejectsSymlinkAncestorDirectory(t *testing.T) {
	root := t.TempDir()
	realSubdir := filepath.Join(t.TempDir(), "real-sub")
	if err := os.MkdirAll(realSubdir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(realSubdir, "a.png"), pngContent)
	// "grid" inside root becomes a symlink pointing to a directory that was
	// never part of the scanned root, simulating a substitution that
	// happened after the original inventory scan.
	if err := os.Symlink(realSubdir, filepath.Join(root, "grid")); err != nil {
		t.Skipf("symlink creation is not permitted in this environment: %v", err)
	}
	observation := model.Observation{RootID: "source", RelativePath: "grid/a.png", SHA256: sha256Like(0)}
	snapshot := Snapshot{
		Inventory: model.Inventory{Observations: []model.Observation{observation}},
		Roots:     map[string]model.Root{"source": {ID: "source", Kind: "generic", Path: root}},
		index:     map[string]model.Observation{ObservationID("source", "grid/a.png"): observation},
	}
	if _, err := ResolveMedia(snapshot, ObservationID("source", "grid/a.png")); err == nil {
		t.Fatal("expected a symlinked ancestor directory to be rejected")
	}
}

func TestResolveMediaRejectsOversizedFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "big.png")
	content := "\x89PNG\r\n\x1a\n" + strings.Repeat("x", 1024)
	mustWriteFile(t, path, content)
	observation := model.Observation{RootID: "source", RelativePath: "big.png", SHA256: sha256Like(0)}
	snapshot := Snapshot{
		Inventory: model.Inventory{Observations: []model.Observation{observation}},
		Roots:     map[string]model.Root{"source": {ID: "source", Kind: "generic", Path: root}},
		index:     map[string]model.Observation{ObservationID("source", "big.png"): observation},
	}

	setMaxMediaServeBytesForTest(t, 10)

	if _, err := ResolveMedia(snapshot, ObservationID("source", "big.png")); err == nil {
		t.Fatal("expected an oversized file to be rejected")
	}
}

func TestResolveMediaSniffsMIMEFromContentNotExtension(t *testing.T) {
	root := t.TempDir()
	// Named ".png" but actually PDF content: the resolver must trust the
	// sniffed content type, not the extension.
	path := filepath.Join(root, "not-really.png")
	mustWriteFile(t, path, "%PDF-1.4 fake pdf body")
	observation := model.Observation{RootID: "source", RelativePath: "not-really.png", SHA256: sha256Like(0)}
	snapshot := Snapshot{
		Inventory: model.Inventory{Observations: []model.Observation{observation}},
		Roots:     map[string]model.Root{"source": {ID: "source", Kind: "generic", Path: root}},
		index:     map[string]model.Observation{ObservationID("source", "not-really.png"): observation},
	}
	resolution, err := ResolveMedia(snapshot, ObservationID("source", "not-really.png"))
	if err != nil {
		t.Fatal(err)
	}
	if resolution.MIME != "application/pdf" {
		t.Fatalf("MIME = %q, want application/pdf (sniffed from content)", resolution.MIME)
	}
}
