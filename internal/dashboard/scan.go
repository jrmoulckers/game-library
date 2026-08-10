package dashboard

import (
	"net/http"
	"sync"
	"time"

	"github.com/jrmoulckers/game-library/internal/inventory"
	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/review"
)

type scanRootStatus struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	ItemCount int    `json:"itemCount,omitempty"`
	Message   string `json:"message,omitempty"`
}

type scanStatus struct {
	Status    string           `json:"status"`
	Completed int              `json:"completed"`
	Total     int              `json:"total"`
	Roots     []scanRootStatus `json:"roots"`
}

type scanManager struct {
	mu         sync.RWMutex
	running    bool
	generation uint64
	status     scanStatus
}

func (m *scanManager) start(cfg model.Config, cache *snapshotCache) bool {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return false
	}
	if !cache.tryBeginRefresh() {
		m.mu.Unlock()
		return false
	}
	m.generation++
	generation := m.generation
	m.running = true
	m.status = scanStatus{Status: "scanning", Total: len(cfg.Roots)}
	for _, root := range cfg.Roots {
		m.status.Roots = append(m.status.Roots, scanRootStatus{ID: root.ID, Status: "queued"})
	}
	m.mu.Unlock()
	revision := cache.currentRevision()
	cache.storeIfRevision(review.NewSnapshot(cfg, model.Inventory{
		Version: model.SchemaVersion, ToolVersion: model.ToolVersion,
		CreatedAt: time.Now().UTC().Format(time.RFC3339), Privacy: "private",
	}, review.SourceScan, time.Now().UTC()), revision)

	go m.run(cfg, cache, generation, revision)
	return true
}

func (m *scanManager) run(cfg model.Config, cache *snapshotCache, generation, revision uint64) {
	defer cache.endRefresh()
	defer m.finish(generation)
	combined := model.Inventory{
		Version: model.SchemaVersion, ToolVersion: model.ToolVersion,
		CreatedAt: time.Now().UTC().Format(time.RFC3339), Privacy: "private",
	}
	for index, root := range cfg.Roots {
		if !m.isCurrent(generation) {
			return
		}
		m.updateRoot(index, "scanning", 0, "")
		scanned, err := inventory.Scan([]model.Root{root})
		if err != nil {
			m.updateRoot(index, "error", 0, "This source could not be scanned. Check that it is available and readable.")
		} else {
			combined.Roots = append(combined.Roots, scanned.Roots...)
			combined.Observations = append(combined.Observations, scanned.Observations...)
			combined.Issues = append(combined.Issues, scanned.Issues...)
			m.updateRoot(index, "complete", len(scanned.Observations), "")
		}
		combined.DuplicateSummary = inventory.SummarizeDuplicates(combined.Observations)
		if m.isCurrent(generation) {
			cache.storeIfRevision(review.NewSnapshot(cfg, combined, review.SourceScan, time.Now().UTC()), revision)
		}
	}
}

func (m *scanManager) finish(generation uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = false
	if m.generation == generation {
		m.status.Status = "complete"
	} else {
		m.status.Status = "idle"
	}
}

func (m *scanManager) isCurrent(generation uint64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.generation == generation
}

func (m *scanManager) invalidate() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.generation++
	m.status.Status = "cancelling"
}

func (m *scanManager) updateRoot(index int, status string, count int, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status.Roots[index].Status = status
	m.status.Roots[index].ItemCount = count
	m.status.Roots[index].Message = message
	completed := 0
	for _, root := range m.status.Roots {
		if root.Status == "complete" || root.Status == "error" {
			completed++
		}
	}
	m.status.Completed = completed
}

func (m *scanManager) snapshot() scanStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := m.status
	result.Roots = append([]scanRootStatus(nil), m.status.Roots...)
	if result.Status == "" {
		result.Status = "idle"
	}
	return result
}

func (h *handlers) startOrganizerScan(w http.ResponseWriter, r *http.Request) {
	h.stateMu.RLock()
	defer h.stateMu.RUnlock()
	cfg, ok := h.requireActiveConfig(w)
	if !ok {
		return
	}
	if h.opts.InventoryReport != "" {
		writeJSONError(w, http.StatusConflict, "report_source", "this dashboard is using an existing inventory report and cannot rescan its sources")
		return
	}
	if !h.scans.start(cfg, h.snapshots) {
		writeJSON(w, http.StatusConflict, h.scans.snapshot())
		return
	}
	writeJSON(w, http.StatusAccepted, h.scans.snapshot())
}

func (h *handlers) organizerScanStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.scans.snapshot())
}
