package dashboard

import (
	"sync"
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
)

func TestSnapshotCacheLazyLoadsOnce(t *testing.T) {
	cache := &snapshotCache{}
	cfg := model.Config{Version: model.SchemaVersion, Policy: model.PolicyFile{Version: model.SchemaVersion, Default: "tracked-external"}}

	first, err := cache.getOrLoad(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if !cache.hasValue {
		t.Fatal("expected hasValue to be true after the first load")
	}
	second, err := cache.getOrLoad(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if first.ScannedAt != second.ScannedAt {
		t.Fatal("expected the second getOrLoad to return the exact same cached snapshot, not reload")
	}
}

func TestSnapshotCacheInvalidateForcesReload(t *testing.T) {
	cache := &snapshotCache{}
	cfg := model.Config{Version: model.SchemaVersion, Policy: model.PolicyFile{Version: model.SchemaVersion, Default: "tracked-external"}}

	if _, err := cache.getOrLoad(cfg, ""); err != nil {
		t.Fatal(err)
	}
	cache.invalidate()
	if cache.hasValue {
		t.Fatal("expected hasValue to be false after invalidate")
	}
	if _, err := cache.getOrLoad(cfg, ""); err != nil {
		t.Fatal(err)
	}
	if !cache.hasValue {
		t.Fatal("expected getOrLoad to reload and cache again after invalidate")
	}
}

// TestSnapshotCachePermitsOnlyOneInFlightRefresh covers issue #4's "permits
// one in-flight refresh" requirement directly at the cache level: many
// concurrent callers attempting tryBeginRefresh must see exactly one
// success.
func TestSnapshotCachePermitsOnlyOneInFlightRefresh(t *testing.T) {
	cache := &snapshotCache{}
	const attempts = 50

	var wg sync.WaitGroup
	var mu sync.Mutex
	successes := 0
	wg.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func() {
			defer wg.Done()
			if cache.tryBeginRefresh() {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if successes != 1 {
		t.Fatalf("expected exactly 1 successful tryBeginRefresh out of %d concurrent attempts, got %d", attempts, successes)
	}

	// After endRefresh, a subsequent attempt must succeed again.
	cache.endRefresh()
	if !cache.tryBeginRefresh() {
		t.Fatal("expected tryBeginRefresh to succeed again after endRefresh")
	}
}
