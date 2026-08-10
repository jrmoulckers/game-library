package dashboard

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/review"
)

func TestScanManagerPublishesPartialAndCompleteSnapshot(t *testing.T) {
	first := filepath.Join(t.TempDir(), "first")
	second := filepath.Join(t.TempDir(), "second")
	for _, item := range []struct {
		root string
		name string
	}{{first, "123.png"}, {second, "456p.png"}} {
		if err := os.MkdirAll(item.root, 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(item.root, item.name), []byte(reviewPNG), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cfg := model.Config{Roots: []model.Root{
		{ID: "one", Kind: "steam-grid", Path: first},
		{ID: "two", Kind: "steam-grid", Path: second},
	}}
	manager := &scanManager{}
	cache := &snapshotCache{}
	if !manager.start(cfg, cache) {
		t.Fatal("expected scan to start")
	}
	if manager.start(cfg, cache) {
		t.Fatal("expected concurrent scan to be rejected")
	}
	deadline := time.Now().Add(5 * time.Second)
	for manager.snapshot().Status != "complete" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	status := manager.snapshot()
	if status.Status != "complete" || status.Completed != 2 {
		t.Fatalf("status = %+v", status)
	}
	snapshot, err := cache.getOrLoad(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Inventory.Observations) != 2 {
		t.Fatalf("observations = %d", len(snapshot.Inventory.Observations))
	}
}

func TestInvalidatedScanCannotReplaceCurrentSnapshot(t *testing.T) {
	cache := &snapshotCache{}
	current := review.NewSnapshot(model.Config{}, model.Inventory{
		Observations: []model.Observation{{RootID: "current", RelativePath: "art.png"}},
	}, review.SourceScan, time.Now())
	cache.store(current)
	manager := &scanManager{running: true, generation: 2}
	manager.run(model.Config{Roots: []model.Root{{ID: "old", Path: t.TempDir()}}}, cache, 1, cache.currentRevision())
	got, found, err := cache.current()
	if err != nil || !found {
		t.Fatalf("current snapshot: found=%v err=%v", found, err)
	}
	if len(got.Inventory.Observations) != 1 || got.Inventory.Observations[0].RootID != "current" {
		t.Fatalf("invalidated scan replaced snapshot: %+v", got.Inventory)
	}
	if status := manager.snapshot().Status; status != "idle" {
		t.Fatalf("invalidated scan status = %q, want idle for replacement", status)
	}
}
