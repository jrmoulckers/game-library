package dashboard

import (
	"sync"

	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/review"
)

// snapshotCache holds a single lazily-loaded, concurrency-safe in-memory
// review.Snapshot per running dashboard process. It exists so a GET review
// endpoint never re-scans the configured roots on every request (which
// review.LoadSnapshot would otherwise do unconditionally): the snapshot is
// loaded once, on first use, and then only refreshed when the active
// configuration changes (see invalidate, called from PUT /api/config) or
// when an operator explicitly asks via POST /api/review/refresh.
//
// Nothing here is ever written to disk: this is an in-memory cache only,
// matching ADR-0007 and this task's requirement to never persist a private
// inventory automatically.
type snapshotCache struct {
	mu         sync.Mutex
	hasValue   bool
	value      review.Snapshot
	err        error
	refreshing bool
}

// getOrLoad returns the cached snapshot, loading it first if this is the
// first call since process start or the last invalidate/refresh.
func (c *snapshotCache) getOrLoad(cfg model.Config, reportPath string) (review.Snapshot, error) {
	c.mu.Lock()
	if c.hasValue {
		value, err := c.value, c.err
		c.mu.Unlock()
		return value, err
	}
	c.mu.Unlock()
	return c.reload(cfg, reportPath)
}

// invalidate clears the cached snapshot (and any cached load error)
// without loading a new one. The next getOrLoad call will load fresh. This
// is called after a successful PUT /api/config so a changed set of roots
// is never served from a snapshot computed against the previous
// configuration.
func (c *snapshotCache) invalidate() {
	c.mu.Lock()
	c.hasValue = false
	c.value = review.Snapshot{}
	c.err = nil
	c.mu.Unlock()
}

// tryBeginRefresh attempts to claim the single in-flight refresh slot,
// returning false immediately (without blocking) if a refresh is already
// running. This is how POST /api/review/refresh permits only one in-flight
// refresh: a concurrent second request observes ok=false and returns
// immediately instead of starting a duplicate scan.
func (c *snapshotCache) tryBeginRefresh() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.refreshing {
		return false
	}
	c.refreshing = true
	return true
}

// endRefresh releases the in-flight refresh slot claimed by
// tryBeginRefresh. Callers must always call this (typically via defer)
// once tryBeginRefresh has returned true, whether or not the refresh
// itself succeeded.
func (c *snapshotCache) endRefresh() {
	c.mu.Lock()
	c.refreshing = false
	c.mu.Unlock()
}

// reload always performs a fresh review.LoadSnapshot call and stores the
// result (value or error) as the new cached value, regardless of whatever
// was cached before.
func (c *snapshotCache) reload(cfg model.Config, reportPath string) (review.Snapshot, error) {
	snapshot, err := review.LoadSnapshot(cfg, reportPath)
	c.mu.Lock()
	c.hasValue = true
	c.value = snapshot
	c.err = err
	c.mu.Unlock()
	return snapshot, err
}
