package review

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/report"
)

func exampleConfig(rootPath string) model.Config {
	return model.Config{
		Version: model.SchemaVersion,
		Roots:   []model.Root{{ID: "source", Kind: "generic", Path: rootPath}},
		Policy:  model.PolicyFile{Version: model.SchemaVersion, Default: "tracked-external"},
	}
}

func TestLoadSnapshotScansConfiguredRoots(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "123.png"), pngContent)

	Clock = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	defer func() { Clock = time.Now }()

	snapshot, err := LoadSnapshot(exampleConfig(root), "")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Source != SourceScan {
		t.Fatalf("source = %q, want %q", snapshot.Source, SourceScan)
	}
	if len(snapshot.Inventory.Observations) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(snapshot.Inventory.Observations))
	}
	if snapshot.ScannedAt.IsZero() {
		t.Fatal("expected a non-zero ScannedAt for a fresh scan")
	}
}

func TestLoadSnapshotLoadsFromPrivateReport(t *testing.T) {
	reportPath := filepath.Join(t.TempDir(), "inventory.json")
	inv := model.Inventory{
		Version: model.SchemaVersion, Privacy: "private", CreatedAt: "2026-02-03T04:05:06Z",
		Observations: []model.Observation{{RootID: "source", RelativePath: "123.png", SHA256: strings.Repeat("a", 64)}},
	}
	if err := report.WriteJSON(reportPath, inv); err != nil {
		t.Fatal(err)
	}

	snapshot, err := LoadSnapshot(exampleConfig(t.TempDir()), reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Source != SourceReport {
		t.Fatalf("source = %q, want %q", snapshot.Source, SourceReport)
	}
	if snapshot.ScannedAt.Format(time.RFC3339) != "2026-02-03T04:05:06Z" {
		t.Fatalf("ScannedAt = %v, want the report's CreatedAt", snapshot.ScannedAt)
	}
	if len(snapshot.Inventory.Observations) != 1 {
		t.Fatalf("expected 1 observation from the report, got %d", len(snapshot.Inventory.Observations))
	}
}

func TestLoadSnapshotFromReportNeverTouchesConfiguredRoots(t *testing.T) {
	reportPath := filepath.Join(t.TempDir(), "inventory.json")
	inv := model.Inventory{Version: model.SchemaVersion, Privacy: "sanitized", CreatedAt: "2026-01-01T00:00:00Z"}
	if err := report.WriteJSON(reportPath, inv); err != nil {
		t.Fatal(err)
	}
	// A configured root that does not exist on disk at all must not cause
	// an error when loading from a report: the report path never scans.
	missingRoot := filepath.Join(t.TempDir(), "does-not-exist")
	if _, err := LoadSnapshot(exampleConfig(missingRoot), reportPath); err != nil {
		t.Fatal(err)
	}
}

func TestObservationIDIsStableAndDistinguishesInputs(t *testing.T) {
	a := ObservationID("source", "a.png")
	b := ObservationID("source", "a.png")
	if a != b {
		t.Fatal("expected the same rootID+relativePath to produce the same ID")
	}
	c := ObservationID("source", "b.png")
	if a == c {
		t.Fatal("expected different relative paths to produce different IDs")
	}
	d := ObservationID("other", "a.png")
	if a == d {
		t.Fatal("expected different root IDs to produce different IDs")
	}
	if len(a) != 64 {
		t.Fatalf("expected a 64-character hex digest, got %d characters", len(a))
	}
}

func TestFindObservationReportsAbsence(t *testing.T) {
	snapshot := Snapshot{index: map[string]model.Observation{}}
	if _, ok := snapshot.FindObservation("nonexistent"); ok {
		t.Fatal("expected FindObservation to report false for an unknown id")
	}
}

func TestScanAgeNeverNegative(t *testing.T) {
	snapshot := Snapshot{ScannedAt: time.Now().Add(1 * time.Hour)}
	if age := snapshot.ScanAge(time.Now()); age != 0 {
		t.Fatalf("expected a future ScannedAt to clamp to 0, got %v", age)
	}
}

func TestScanAgeZeroWhenNeverScanned(t *testing.T) {
	snapshot := Snapshot{}
	if age := snapshot.ScanAge(time.Now()); age != 0 {
		t.Fatalf("expected zero ScannedAt to report age 0, got %v", age)
	}
}
