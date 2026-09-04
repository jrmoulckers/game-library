package dashboard

import (
	"net/http"

	"github.com/jrmoulckers/game-library/internal/coverage"
	"github.com/jrmoulckers/game-library/internal/organizer"
	"github.com/jrmoulckers/game-library/internal/topology"
)

type topologyView struct {
	topology.Document
	// Saved reports whether this is the owner's stored document or the
	// starting default. The default is fully usable, so this only tells
	// the UI whether edits have ever been made.
	Saved bool `json:"saved"`
}

func (h *handlers) getTopology(w http.ResponseWriter, r *http.Request) {
	h.stateMu.RLock()
	defer h.stateMu.RUnlock()
	doc, saved, err := topology.Load(h.opts.Workspace.Topology)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to read the topology")
		return
	}
	writeJSON(w, http.StatusOK, topologyView{Document: doc, Saved: saved})
}

func (h *handlers) putTopology(w http.ResponseWriter, r *http.Request) {
	var doc topology.Document
	if err := decodeJSON(r, &doc); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := topology.Validate(doc); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_topology", err.Error())
		return
	}
	h.stateMu.Lock()
	defer h.stateMu.Unlock()
	if err := topology.Save(h.opts.Workspace.Topology, doc); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to save the topology")
		return
	}
	writeJSON(w, http.StatusOK, topologyView{Document: doc, Saved: true})
}

// coverageReport answers both directions at once: which profiles hold
// media for each game, and which games each profile covers. It is built
// from the same scan the library view uses, so it never re-reads disk.
func (h *handlers) coverageReport(w http.ResponseWriter, r *http.Request) {
	h.stateMu.RLock()
	defer h.stateMu.RUnlock()
	cfg, ok := h.requireActiveConfig(w)
	if !ok {
		return
	}
	doc, _, err := topology.Load(h.opts.Workspace.Topology)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to read the topology")
		return
	}
	h.metadata.start(cfg.Roots, false)
	snapshot, ok := h.loadOrganizerSnapshot(w, cfg)
	if !ok {
		return
	}
	profiles, ok := h.loadAllProfileDrafts(w)
	if !ok {
		return
	}
	titles, metadataStatus := h.metadata.current()
	catalog := organizer.BuildWithMetadata(snapshot, profiles, titles)
	report := coverage.Build(catalog, doc)
	writeJSON(w, http.StatusOK, struct {
		coverage.Report
		MetadataStatus string `json:"metadataStatus"`
	}{Report: report, MetadataStatus: metadataStatus})
}
