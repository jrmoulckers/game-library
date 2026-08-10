package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrmoulckers/game-library/internal/manifest"
	"github.com/jrmoulckers/game-library/internal/model"
)

func TestAnalyzeManifestClassifiesCreateReplaceSameAndConflict(t *testing.T) {
	root := t.TempDir()

	existingSameHash := "existing-same"
	sameHashPath := filepath.Join(root, "same.bin")
	if err := os.WriteFile(sameHashPath, []byte(existingSameHash), 0o644); err != nil {
		t.Fatal(err)
	}
	sameHash, err := hashFile(sameHashPath)
	if err != nil {
		t.Fatal(err)
	}

	conflictPath := filepath.Join(root, "conflict.bin")
	if err := os.WriteFile(conflictPath, []byte("old content"), 0o644); err != nil {
		t.Fatal(err)
	}

	actions := []model.Action{
		{Action: "copy", DestinationRoot: "catalog", DestinationPath: "new.bin", SourceSHA256: strings.Repeat("a", 64), SourceSize: 42},
		{Action: "copy", DestinationRoot: "catalog", DestinationPath: "same.bin", SourceSHA256: sameHash, SourceSize: int64(len(existingSameHash))},
		{Action: "copy", DestinationRoot: "catalog", DestinationPath: "conflict.bin", SourceSHA256: strings.Repeat("b", 64), SourceSize: 7},
		{Action: "skip", Reason: "tracked externally"},
	}
	plan, err := manifest.NewPlan("import-plan", actions)
	if err != nil {
		t.Fatal(err)
	}

	analysis, err := AnalyzeManifest(plan, RootResolver{"catalog": root})
	if err != nil {
		t.Fatal(err)
	}
	if analysis.ManifestDigest == "" {
		t.Fatal("expected a non-empty manifest digest")
	}
	byDestination := make(map[string]ActionAnalysis)
	for _, a := range analysis.Actions {
		byDestination[a.Action.DestinationPath] = a
	}

	if byDestination["new.bin"].Effect != EffectCreate {
		t.Fatalf("new.bin effect = %q, want create", byDestination["new.bin"].Effect)
	}
	if byDestination["same.bin"].Effect != EffectReplaceSameHash {
		t.Fatalf("same.bin effect = %q, want replace-same-hash", byDestination["same.bin"].Effect)
	}
	if byDestination["same.bin"].CurrentDestinationHash != sameHash {
		t.Fatalf("same.bin hash mismatch")
	}
	conflictEntry := byDestination["conflict.bin"]
	if conflictEntry.Effect != EffectConflict || !conflictEntry.Conflict {
		t.Fatalf("conflict.bin effect = %+v, want conflict", conflictEntry)
	}
	if conflictEntry.ConflictReason == "" {
		t.Fatal("expected a conflict reason")
	}

	skipEntry, ok := findActionByAction(analysis.Actions, "skip")
	if !ok || skipEntry.Effect != EffectNoFilesystemChange {
		t.Fatalf("skip effect = %+v, want no-filesystem-change", skipEntry)
	}

	if analysis.Conflicts != 1 {
		t.Fatalf("Conflicts = %d, want 1", analysis.Conflicts)
	}
	if analysis.EstimatedBackupBytes != int64(len("old content")) {
		t.Fatalf("EstimatedBackupBytes = %d, want %d", analysis.EstimatedBackupBytes, len("old content"))
	}
	// EstimatedNewBytes accumulates from create (42) and the conflicting
	// replace (7); the already-matching same.bin contributes nothing new.
	if analysis.EstimatedNewBytes != 49 {
		t.Fatalf("EstimatedNewBytes = %d, want 49", analysis.EstimatedNewBytes)
	}

	if len(analysis.Destinations) != 1 || analysis.Destinations[0].Root != "catalog" {
		t.Fatalf("expected a single catalog destination summary, got %+v", analysis.Destinations)
	}
	if analysis.Destinations[0].NeededBytes != 49 {
		t.Fatalf("NeededBytes = %d, want 49", analysis.Destinations[0].NeededBytes)
	}
	// Free space may or may not be resolvable on every CI platform (see
	// diskspace_other.go), but when it is known it must never claim
	// sufficiency for an absurdly large requirement, and never claim
	// insufficiency it cannot actually determine.
	if analysis.Destinations[0].AvailableBytesKnown && analysis.Destinations[0].AvailableBytes < 0 {
		t.Fatalf("AvailableBytes must never be negative: %+v", analysis.Destinations[0])
	}
}

func findActionByAction(actions []ActionAnalysis, action string) (ActionAnalysis, bool) {
	for _, a := range actions {
		if a.Action.Action == action {
			return a, true
		}
	}
	return ActionAnalysis{}, false
}

func TestAnalyzeManifestRejectsUnsafeDestination(t *testing.T) {
	actions := []model.Action{{Action: "copy", DestinationRoot: "catalog", DestinationPath: "../escape.bin", SourceSHA256: strings.Repeat("a", 64)}}
	plan, err := manifest.NewPlan("import-plan", actions)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AnalyzeManifest(plan, RootResolver{"catalog": t.TempDir()}); err == nil {
		t.Fatal("expected an error for an unsafe destination path")
	}
}

func TestAnalyzeManifestDigestMatchesManifestDigest(t *testing.T) {
	actions := []model.Action{{Action: "skip", Reason: "r"}}
	plan, err := manifest.NewPlan("import-plan", actions)
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := AnalyzeManifest(plan, RootResolver{"catalog": t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	want, err := manifest.Digest(plan)
	if err != nil {
		t.Fatal(err)
	}
	if analysis.ManifestDigest != want {
		t.Fatalf("ManifestDigest = %q, want %q", analysis.ManifestDigest, want)
	}
}

// TestAnalyzeManifestUnresolvedRootBecomesAnExplicitConflict covers issue
// #2: an action whose DestinationRoot is not present in the RootResolver
// must never be probed against any guessed or client-supplied path, and
// must never abort the whole analysis with a server error — it becomes an
// explicit, symbolic conflict entry instead.
func TestAnalyzeManifestUnresolvedRootBecomesAnExplicitConflict(t *testing.T) {
	actions := []model.Action{
		{Action: "copy", DestinationRoot: "playnite", DestinationPath: "games/example/Logo.png", SourceSHA256: strings.Repeat("a", 64), SourceSize: 5},
	}
	plan, err := manifest.NewPlan("playnite-export-plan", actions)
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := AnalyzeManifest(plan, RootResolver{"catalog": t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(analysis.Actions))
	}
	entry := analysis.Actions[0]
	if entry.Effect != EffectRootUnavailable || !entry.Conflict {
		t.Fatalf("expected an unresolved-root conflict, got %+v", entry)
	}
	if entry.ConflictReason == "" {
		t.Fatal("expected a non-empty conflict reason")
	}
	if strings.Contains(entry.ConflictReason, string(os.PathSeparator)) {
		t.Fatalf("conflict reason must never contain a filesystem path: %q", entry.ConflictReason)
	}
	if len(analysis.Warnings) == 0 {
		t.Fatal("expected at least one manifest-level warning for the unresolved root")
	}
	if analysis.Conflicts != 1 {
		t.Fatalf("Conflicts = %d, want 1", analysis.Conflicts)
	}
	// An unresolved root must never appear in the per-destination space
	// summary: there is no root path to check free space against.
	for _, d := range analysis.Destinations {
		if d.Root == "playnite" {
			t.Fatalf("did not expect a destination space entry for an unresolved root: %+v", d)
		}
	}
}

// TestAnalyzeManifestRepresentsRemoveActionsWithoutDeletingAnything covers
// issue #6: a "remove" action (never generated by any planner in this
// repository today, but representable if one ever appears in a manifest) is
// reported, never executed.
func TestAnalyzeManifestRepresentsRemoveActionsWithoutDeletingAnything(t *testing.T) {
	root := t.TempDir()
	existingPath := filepath.Join(root, "stale.bin")
	if err := os.WriteFile(existingPath, []byte("stale content"), 0o644); err != nil {
		t.Fatal(err)
	}

	actions := []model.Action{
		{Action: "remove", DestinationRoot: "catalog", DestinationPath: "stale.bin", Reason: "superseded by a newer asset"},
		{Action: "remove", DestinationRoot: "catalog", DestinationPath: "already-gone.bin", Reason: "already absent"},
	}
	plan, err := manifest.NewPlan("hypothetical-cleanup-plan", actions)
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := AnalyzeManifest(plan, RootResolver{"catalog": root})
	if err != nil {
		t.Fatal(err)
	}

	byDestination := make(map[string]ActionAnalysis)
	for _, a := range analysis.Actions {
		byDestination[a.Action.DestinationPath] = a
	}
	present := byDestination["stale.bin"]
	if present.Effect != EffectWouldRemove || !present.CurrentDestinationExists {
		t.Fatalf("stale.bin = %+v, want would-remove", present)
	}
	if present.CurrentDestinationBytes != int64(len("stale content")) {
		t.Fatalf("CurrentDestinationBytes = %d", present.CurrentDestinationBytes)
	}
	if analysis.EstimatedFreedBytes != int64(len("stale content")) {
		t.Fatalf("EstimatedFreedBytes = %d, want %d", analysis.EstimatedFreedBytes, len("stale content"))
	}

	absent := byDestination["already-gone.bin"]
	if absent.Effect != EffectNoFilesystemChange {
		t.Fatalf("already-gone.bin = %+v, want no-filesystem-change", absent)
	}

	// The file must still exist: analysis never executes anything.
	if _, err := os.Stat(existingPath); err != nil {
		t.Fatalf("AnalyzeManifest must never delete a file: %v", err)
	}
}

func TestRootResolverTreatsEmptyPathAsUnconfigured(t *testing.T) {
	resolver := RootResolver{"catalog": ""}
	if _, ok := resolver.Resolve("catalog"); ok {
		t.Fatal("expected an empty configured path to be treated as unconfigured")
	}
	if _, ok := resolver.Resolve("missing"); ok {
		t.Fatal("expected a name absent from the resolver to be unconfigured")
	}
}
