package review

import (
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
)

func dup(rootID, rootKind, relPath, hash string) model.Observation {
	return model.Observation{RootID: rootID, RootKind: rootKind, RelativePath: relPath, SHA256: hash, Size: 5}
}

func TestBuildDuplicateViewIgnoresUniqueContent(t *testing.T) {
	snapshot := Snapshot{Inventory: model.Inventory{Observations: []model.Observation{
		dup("a", "generic", "one.png", sha256Like(0)),
		dup("b", "generic", "two.png", sha256Like(1)),
	}}}
	if groups := BuildDuplicateView(snapshot); len(groups) != 0 {
		t.Fatalf("expected no duplicate groups, got %+v", groups)
	}
}

func TestBuildDuplicateViewClassifiesCanonicalOpportunity(t *testing.T) {
	snapshot := Snapshot{Inventory: model.Inventory{Observations: []model.Observation{
		dup("a", "generic", "one.png", sha256Like(0)),
		dup("b", "generic", "one-copy.png", sha256Like(0)),
	}}}
	groups := BuildDuplicateView(snapshot)
	if len(groups) != 1 {
		t.Fatalf("expected 1 duplicate group, got %d", len(groups))
	}
	if groups[0].Classification != ClassCanonicalOpportunity {
		t.Fatalf("classification = %q, want %q", groups[0].Classification, ClassCanonicalOpportunity)
	}
	if groups[0].Reason == "" {
		t.Fatal("expected a non-empty reason")
	}
	if len(groups[0].Occurrences) != 2 {
		t.Fatalf("expected 2 occurrences, got %d", len(groups[0].Occurrences))
	}
}

func TestBuildDuplicateViewClassifiesExpectedGeneratedCopy(t *testing.T) {
	snapshot := Snapshot{Inventory: model.Inventory{Observations: []model.Observation{
		dup("source", "steam-grid", "123.png", sha256Like(0)),
		dup("catalog", "decky-catalog", "artwork/deck-default/grid/123.png", sha256Like(0)),
	}}}
	groups := BuildDuplicateView(snapshot)
	if len(groups) != 1 {
		t.Fatalf("expected 1 duplicate group, got %d", len(groups))
	}
	if groups[0].Classification != ClassExpectedCopy {
		t.Fatalf("classification = %q, want %q", groups[0].Classification, ClassExpectedCopy)
	}
}

func TestBuildDuplicateViewClassifiesGeneratedGeneratedAsReview(t *testing.T) {
	snapshot := Snapshot{Inventory: model.Inventory{Observations: []model.Observation{
		dup("catalog-a", "decky-catalog", "artwork/a/grid/123.png", sha256Like(0)),
		dup("catalog-b", "decky-catalog", "artwork/b/grid/123.png", sha256Like(0)),
	}}}
	groups := BuildDuplicateView(snapshot)
	if len(groups) != 1 {
		t.Fatalf("expected 1 duplicate group, got %d", len(groups))
	}
	if groups[0].Classification != ClassReview {
		t.Fatalf("classification = %q, want %q", groups[0].Classification, ClassReview)
	}
}

func TestBuildDuplicateViewIsDeterministicallyOrdered(t *testing.T) {
	snapshot := Snapshot{Inventory: model.Inventory{Observations: []model.Observation{
		dup("b", "generic", "z.png", sha256Like(1)),
		dup("a", "generic", "y.png", sha256Like(1)),
		dup("a", "generic", "x.png", sha256Like(0)),
		dup("a", "generic", "w.png", sha256Like(0)),
	}}}
	first := BuildDuplicateView(snapshot)
	second := BuildDuplicateView(snapshot)
	if len(first) != len(second) {
		t.Fatalf("non-deterministic group count")
	}
	for i := range first {
		if first[i].SHA256 != second[i].SHA256 {
			t.Fatalf("non-deterministic group ordering at index %d", i)
		}
		for j := range first[i].Occurrences {
			if first[i].Occurrences[j] != second[i].Occurrences[j] {
				t.Fatalf("non-deterministic occurrence ordering in group %d", i)
			}
		}
	}
}
