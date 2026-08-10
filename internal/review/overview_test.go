package review

import (
	"testing"
	"time"

	"github.com/jrmoulckers/game-library/internal/model"
)

func TestBuildOverviewSummarizesRootsAndIssues(t *testing.T) {
	snapshot := Snapshot{
		Source:    SourceScan,
		ScannedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Inventory: model.Inventory{
			Privacy: "private",
			Roots: []model.RootSummary{
				{ID: "a", Kind: "generic", FileCount: 3, TotalBytes: 300, MediaCount: 2, ImageCount: 2},
				{ID: "b", Kind: "esde-media", FileCount: 1, TotalBytes: 10},
			},
			Issues: []model.ValidationIssue{
				{RootID: "a", RelativePath: "x.png", Code: "role-media-mismatch", Message: "bad"},
				{RootID: "a", RelativePath: "y.png", Code: "media-type-mismatch", Message: "bad"},
				{RootID: "b", RelativePath: "z.png", Code: "symlink-rejected", Message: "bad"},
			},
			DuplicateSummary: model.DuplicateSummary{Groups: 1, Copies: 2},
		},
	}
	now := time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC)
	overview := BuildOverview(snapshot, now)

	if overview.ScanAgeSeconds != 7200 {
		t.Fatalf("ScanAgeSeconds = %d, want 7200", overview.ScanAgeSeconds)
	}
	if overview.TotalIssues != 3 {
		t.Fatalf("TotalIssues = %d, want 3", overview.TotalIssues)
	}
	if len(overview.Roots) != 2 {
		t.Fatalf("expected 2 root summaries, got %d", len(overview.Roots))
	}
	byID := map[string]RootOverview{}
	for _, r := range overview.Roots {
		byID[r.RootID] = r
	}
	if byID["a"].IssueCount != 2 {
		t.Fatalf("root a issue count = %d, want 2", byID["a"].IssueCount)
	}
	if byID["b"].IssueCount != 1 {
		t.Fatalf("root b issue count = %d, want 1", byID["b"].IssueCount)
	}
	if overview.DuplicateSummary.Groups != 1 {
		t.Fatalf("expected duplicate summary to pass through unchanged")
	}
}

func TestBuildOverviewHandlesNeverScannedSnapshot(t *testing.T) {
	overview := BuildOverview(Snapshot{}, time.Now())
	if overview.ScannedAt != "" {
		t.Fatalf("expected empty ScannedAt for a zero-value snapshot, got %q", overview.ScannedAt)
	}
	if overview.ScanAgeSeconds != 0 {
		t.Fatalf("expected 0 scan age for a never-scanned snapshot, got %d", overview.ScanAgeSeconds)
	}
}
