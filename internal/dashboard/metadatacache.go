package dashboard

import (
	"sync"

	"github.com/jrmoulckers/game-library/internal/metadata"
	"github.com/jrmoulckers/game-library/internal/model"
)

type metadataCache struct {
	mu          sync.RWMutex
	generation  uint64
	running     bool
	ready       bool
	catalog     metadata.Catalog
	workers     chan struct{}
	fingerprint string
}

func (c *metadataCache) start(roots []model.Root, force bool) {
	fingerprint := metadata.Fingerprint(roots)
	c.mu.Lock()
	if c.running || (c.ready && c.fingerprint == fingerprint) {
		c.mu.Unlock()
		return
	}
	if c.ready && c.fingerprint != fingerprint {
		c.catalog = metadata.NewCatalog()
		c.ready = false
	}
	if c.workers == nil {
		c.workers = make(chan struct{}, 2)
	}
	select {
	case c.workers <- struct{}{}:
	default:
		c.mu.Unlock()
		return
	}
	c.generation++
	generation := c.generation
	c.running = true
	c.mu.Unlock()

	rootsCopy := append([]model.Root(nil), roots...)
	go func() {
		defer func() { <-c.workers }()
		catalog := metadata.ResolveAll(rootsCopy)
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.generation != generation {
			return
		}
		c.catalog = catalog
		c.fingerprint = fingerprint
		c.running = false
		c.ready = true
	}()
}

func (c *metadataCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	c.running = false
	c.ready = false
	c.catalog = metadata.NewCatalog()
	c.fingerprint = ""
}

func (c *metadataCache) current() (metadata.Catalog, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	status := "ready"
	if c.running {
		status = "loading"
	} else if !c.ready {
		status = "idle"
	}
	return c.catalog, status
}
