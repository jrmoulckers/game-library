package publishing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrmoulckers/game-library/internal/manifest"
	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/topology"
	"github.com/jrmoulckers/game-library/internal/workspace"
)

func write(t *testing.T, path, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T) (*Engine, Source, Target) {
	t.Helper()
	root := t.TempDir()
	write(t, filepath.Join(root, "123.png"), "new grid")
	write(t, filepath.Join(root, "123p.png"), "new portrait")
	actions := []model.Action{}
	for _, name := range []string{"123.png", "123p.png"} {
		hash, err := hashFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		actions = append(actions, model.Action{Action: "copy", SourceRoot: "catalog", SourcePath: name,
			SourceSHA256: hash, DestinationRoot: "steam", DestinationPath: name})
	}
	plan, err := manifest.NewPlan("steam-export-plan", actions)
	if err != nil {
		t.Fatal(err)
	}
	target := Target{ID: "pc-steam", Name: "PC Steam", Platform: "steam", Device: "pc", Adapter: "steam", Root: t.TempDir()}
	return &Engine{Paths: workspace.NewPaths(t.TempDir())}, Source{
		Key: "steam/standard", Plan: plan, Roots: map[string]string{"catalog": root},
	}, target
}

func preview(t *testing.T, source Source, target Target) Plan {
	t.Helper()
	plan, err := Build(source, target)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestPublishBackupIdempotenceAndNoDeleteRollback(t *testing.T) {
	engine, source, target := fixture(t)
	write(t, filepath.Join(target.Root, "123.png"), "old grid")
	write(t, filepath.Join(target.Root, "999.png"), "unrelated")
	plan := preview(t, source, target)
	if plan.Different != 1 || plan.Missing != 1 || plan.Unknown != 0 {
		t.Fatalf("parity = %+v", plan)
	}
	if again := preview(t, source, target); again.Digest != plan.Digest {
		t.Fatal("preview is not deterministic")
	}
	if _, err := engine.Publish(source, target, ""); err == nil {
		t.Fatal("unapproved publish allowed")
	}
	receipt, err := engine.Publish(source, target, plan.Digest)
	if err != nil || receipt.State != "complete" {
		t.Fatalf("publish: %+v %v", receipt, err)
	}
	backup := filepath.Join(engine.Paths.Root, "publishing", plan.Digest, "backup", "123.png")
	if data, err := os.ReadFile(backup); err != nil || string(data) != "old grid" {
		t.Fatalf("backup: %q %v", data, err)
	}
	if _, err := engine.Publish(source, target, plan.Digest); err != nil {
		t.Fatalf("idempotent repeat: %v", err)
	}
	current := preview(t, source, target)
	if current.Matching != 2 {
		t.Fatalf("published bytes = %+v", current)
	}
	restore, err := engine.PreviewRollback(target, plan.Digest)
	if err != nil || restore.Retained != 1 || len(restore.Plan.Files) != 1 {
		t.Fatalf("restore: %+v %v", restore, err)
	}
	if _, err := engine.Rollback(target, plan.Digest, "wrong"); err == nil {
		t.Fatal("unapproved rollback allowed")
	}
	if _, err := engine.Rollback(target, plan.Digest, restore.Plan.Digest); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(target.Root, "123.png")); string(data) != "old grid" {
		t.Fatalf("predecessor not restored: %q", data)
	}
	for _, name := range []string{"123p.png", "999.png"} {
		if _, err := os.Stat(filepath.Join(target.Root, name)); err != nil {
			t.Fatalf("rollback deleted %s", name)
		}
	}
	again, err := engine.PreviewRollback(target, plan.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Rollback(target, plan.Digest, again.Plan.Digest); err != nil {
		t.Fatalf("idempotent rollback: %v", err)
	}
}

func TestPublishRejectsSourceTargetAndConfigurationDrift(t *testing.T) {
	for _, drift := range []string{"source", "target", "root", "profile", "platform"} {
		t.Run(drift, func(t *testing.T) {
			engine, source, target := fixture(t)
			plan := preview(t, source, target)
			switch drift {
			case "source":
				write(t, filepath.Join(source.Roots["catalog"], "123.png"), "source drift")
			case "target":
				write(t, filepath.Join(target.Root, "123.png"), "target drift")
			case "root":
				target.Root = t.TempDir()
			case "profile":
				source.Key = "steam/minimalist"
			case "platform":
				target.Platform = "playnite"
			}
			if _, err := engine.Publish(source, target, plan.Digest); err == nil {
				t.Fatal("stale approval allowed")
			}
			if _, err := os.Stat(filepath.Join(target.Root, "123p.png")); !os.IsNotExist(err) {
				t.Fatal("stale approval wrote artwork")
			}
		})
	}
}

func TestParityStatesAndCopyOnlyStaging(t *testing.T) {
	engine, source, target := fixture(t)
	plan := preview(t, source, target)
	path, err := engine.Stage(source, target, plan.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Stage(source, target, plan.Digest); err != nil {
		t.Fatal(err)
	}
	staged := filepath.Join(engine.Paths.Root, filepath.FromSlash(path), "files", "123.png")
	write(t, staged, "edited export")
	if _, err := engine.Stage(source, target, plan.Digest); err == nil {
		t.Fatal("staging drift overwritten")
	}
	if data, _ := os.ReadFile(filepath.Join(source.Roots["catalog"], "123.png")); string(data) != "new grid" {
		t.Fatal("staging copy aliased source")
	}
	for _, state := range []string{"unobserved", "offline", "unsupported"} {
		t.Run(state, func(t *testing.T) {
			candidate := target
			switch state {
			case "unobserved":
				candidate.Root = ""
			case "offline":
				candidate.Root = filepath.Join(t.TempDir(), "not-mounted")
			case "unsupported":
				candidate.Adapter = "unknown-frontend"
			}
			plan := preview(t, source, candidate)
			if plan.State != state || plan.Unknown != 2 || plan.Matching != 0 {
				t.Fatalf("state = %+v", plan)
			}
			if _, err := engine.Publish(source, candidate, plan.Digest); err == nil {
				t.Fatal("unobserved/unsupported publish allowed")
			}
		})
	}
}

func TestCollisionsContainmentAndAtomicFailure(t *testing.T) {
	t.Run("alternate extension", func(t *testing.T) {
		_, source, target := fixture(t)
		write(t, filepath.Join(target.Root, "123.jpg"), "conflicting grid")
		if _, err := Build(source, target); err == nil {
			t.Fatal("alternate extension collision accepted")
		}
	})
	t.Run("export collision", func(t *testing.T) {
		_, source, target := fixture(t)
		extra := source.Plan.Actions[0]
		extra.DestinationPath = "123.PNG"
		source.Plan.Actions = append(source.Plan.Actions, extra)
		if _, err := Build(source, target); err == nil {
			t.Fatal("case collision accepted")
		}
	})
	for _, path := range []string{"../escape.png", "a/../escape.png", "a\\escape.png", "C:/escape.png", "a.png:stream", "NUL.png", "a./b.png"} {
		t.Run(path, func(t *testing.T) {
			if _, err := safePath(t.TempDir(), path, true); err == nil {
				t.Fatal("unsafe path accepted")
			}
		})
	}
	t.Run("overlapping roots", func(t *testing.T) {
		engine, source, target := fixture(t)
		target.Root = source.Roots["catalog"]
		if _, err := engine.Stage(source, target, preview(t, source, target).Digest); err == nil {
			t.Fatal("canonical tree accepted as destination")
		}
	})
	t.Run("copy hash failure", func(t *testing.T) {
		_, source, target := fixture(t)
		destination := filepath.Join(target.Root, "123.png")
		write(t, destination, "old bytes")
		if err := copyAtomic(filepath.Join(source.Roots["catalog"], "123.png"), destination, strings.Repeat("0", 64)); err == nil {
			t.Fatal("hash failure ignored")
		}
		data, _ := os.ReadFile(destination)
		if string(data) != "old bytes" {
			t.Fatal("atomic failure changed destination")
		}
		entries, _ := os.ReadDir(target.Root)
		if len(entries) != 1 {
			t.Fatal("atomic failure left temporary files")
		}
	})
}

func TestPartialPublicationAndRollbackDrift(t *testing.T) {
	engine, source, target := fixture(t)
	write(t, filepath.Join(target.Root, "123.png"), "old grid")
	plan := preview(t, source, target)
	if _, err := engine.Stage(source, target, plan.Digest); err != nil {
		t.Fatal(err)
	}
	// A backup path obstruction must stop before any replacement.
	write(t, filepath.Join(engine.Paths.Root, "publishing", plan.Digest, "backup"), "obstruction")
	receipt, err := engine.Publish(source, target, plan.Digest)
	if err == nil || receipt.State != "in-progress" {
		t.Fatalf("backup failure not surfaced: %+v %v", receipt, err)
	}
	if data, _ := os.ReadFile(filepath.Join(target.Root, "123.png")); string(data) != "old grid" {
		t.Fatal("replacement occurred without backup")
	}
	history, err := engine.History()
	if err != nil || len(history) != 1 {
		t.Fatalf("failed operation not recoverable: %v", err)
	}

	engine, source, target = fixture(t)
	write(t, filepath.Join(target.Root, "123.png"), "old grid")
	plan = preview(t, source, target)
	if _, err := engine.Publish(source, target, plan.Digest); err != nil {
		t.Fatal(err)
	}
	restore, err := engine.PreviewRollback(target, plan.Digest)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(target.Root, "123.png"), "post-publish edit")
	if _, err := engine.Rollback(target, plan.Digest, restore.Plan.Digest); err == nil {
		t.Fatal("rollback overwrote subsequent edit")
	}
	if _, err := engine.Publish(source, target, plan.Digest); err == nil {
		t.Fatal("idempotent retry overwrote drift")
	}
}

func TestLinksRejected(t *testing.T) {
	engine, source, target := fixture(t)
	link := filepath.Join(target.Root, "123.png")
	if err := os.Symlink(filepath.Join(source.Roots["catalog"], "123.png"), link); err != nil {
		t.Skip("host does not permit symlink fixtures")
	}
	plan := preview(t, source, target)
	if plan.State != "unknown" {
		t.Fatalf("linked target state = %s", plan.State)
	}
	if _, err := engine.Publish(source, target, plan.Digest); err == nil {
		t.Fatal("linked target published")
	}
}

func TestTargetValidationSeparatesApplicability(t *testing.T) {
	targets := Targets{Version: 1, Targets: []Target{{ID: "pc-steam", Name: "PC", Device: "pc", Platform: "steam", Adapter: "steam"}}}
	if err := ValidateTargets(targets, topology.Default()); err != nil {
		t.Fatal(err)
	}
	targets.Targets[0].Platform = "playnite"
	if err := ValidateTargets(targets, topology.Default()); err == nil {
		t.Fatal("Steam adapter on Playnite accepted")
	}
}

func TestInterruptedPreparedReceiptCanRestoreAfterRestart(t *testing.T) {
	engine, source, target := fixture(t)
	write(t, filepath.Join(target.Root, "123.png"), "old grid")
	plan := preview(t, source, target)
	if _, err := engine.Stage(source, target, plan.Digest); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(engine.Paths.Root, "publishing", plan.Digest, "backup", "123.png"), "old grid")
	receipt := Receipt{Plan: plan, Prepared: []string{"123.png"}, State: "in-progress"}
	if err := engine.saveReceipt(receipt); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(target.Root, "123.png"), "new grid")
	restarted := &Engine{Paths: engine.Paths}
	restore, err := restarted.PreviewRollback(target, plan.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Rollback(target, plan.Digest, restore.Plan.Digest); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(target.Root, "123.png")); string(data) != "old grid" {
		t.Fatal("interrupted prepared operation not restored")
	}
}

func TestWorkspaceLockExcludesAnotherEngine(t *testing.T) {
	engine, source, target := fixture(t)
	unlock, err := engine.lock()
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	other := &Engine{Paths: engine.Paths}
	if _, err := other.Publish(source, target, preview(t, source, target).Digest); err == nil {
		t.Fatal("another process/engine could use a locked workspace")
	}
}

func TestStagedManifestDriftFailsClosed(t *testing.T) {
	engine, source, target := fixture(t)
	plan := preview(t, source, target)
	path, err := engine.Stage(source, target, plan.Digest)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(engine.Paths.Root, filepath.FromSlash(path), "manifest.json"), "{}")
	if _, err := engine.Stage(source, target, plan.Digest); err == nil {
		t.Fatal("tampered staged manifest was silently replaced")
	}
	if _, err := engine.Publish(source, target, plan.Digest); err == nil {
		t.Fatal("tampered staged manifest was published")
	}
	if _, err := os.Stat(filepath.Join(target.Root, "123.png")); !os.IsNotExist(err) {
		t.Fatal("staging drift changed frontend")
	}
}

func TestMatchingSubsetDoesNotHideBlockedDriftOrWriteArtwork(t *testing.T) {
	engine, source, target := fixture(t)
	write(t, filepath.Join(target.Root, "123.png"), "new grid")
	write(t, filepath.Join(target.Root, "123p.png"), "different unsupported bytes")
	source.Plan.Actions[1].Action = "blocked"
	source.Plan.Actions[1].Reason = "unsupported fixture"
	var err error
	source.Plan, err = manifest.NewPlan("steam-export-plan", source.Plan.Actions)
	if err != nil {
		t.Fatal(err)
	}
	plan := preview(t, source, target)
	if plan.Matching != 1 || plan.Different != 1 || plan.Blocked != 1 ||
		plan.Publishable.Files != 1 || plan.Publishable.Matching != 1 || plan.Publishable.Different != 0 {
		t.Fatalf("all-file and publishable counters were conflated: %+v", plan)
	}
	receipt, err := engine.Publish(source, target, plan.Digest)
	if err != nil || len(receipt.Prepared) != 0 {
		t.Fatalf("matching subset copied artwork: %+v %v", receipt, err)
	}
	if data, _ := os.ReadFile(filepath.Join(target.Root, "123p.png")); string(data) != "different unsupported bytes" {
		t.Fatal("blocked drift was repaired")
	}
}

func TestFilesystemRootCannotBecomeAFrontendTarget(t *testing.T) {
	engine, source, target := fixture(t)
	target.Root = filepath.VolumeName(target.Root) + string(filepath.Separator)
	if !overlaps(target.Root, engine.Paths.Root) || !overlaps(target.Root, source.Roots["catalog"]) {
		t.Fatal("volume root was treated as separate from its descendants")
	}
	if err := engine.validateRoots(source, target); err == nil {
		t.Fatal("filesystem root accepted as a frontend")
	}
}
