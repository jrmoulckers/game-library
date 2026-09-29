package dashboard

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/jrmoulckers/game-library/internal/manifest"
	"github.com/jrmoulckers/game-library/internal/publishing"
	"github.com/jrmoulckers/game-library/internal/topology"
	"github.com/jrmoulckers/game-library/internal/workspace"
)

func (h *handlers) publishingConfig() (topology.Document, publishing.Targets, error) {
	doc, _, err := topology.Load(h.opts.Workspace.Topology)
	if err != nil {
		return doc, publishing.Targets{}, err
	}
	targets, err := publishing.LoadTargets(h.opts.Workspace, doc)
	return doc, targets, err
}

func (h *handlers) getPublishingTargets(w http.ResponseWriter, r *http.Request) {
	h.stateMu.RLock()
	defer h.stateMu.RUnlock()
	_, targets, err := h.publishingConfig()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "targets_unavailable", "Could not read local frontend targets.")
		return
	}
	digest, err := manifest.Digest(targets)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "targets_unavailable", "Could not digest frontend targets.")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		publishing.Targets
		Digest string `json:"digest"`
	}{targets, digest})
}

func (h *handlers) putPublishingTargets(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BaseDigest string             `json:"baseDigest"`
		Targets    publishing.Targets `json:"targets"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeDecodeError(w, err)
		return
	}
	h.stateMu.Lock()
	defer h.stateMu.Unlock()
	doc, current, err := h.publishingConfig()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "targets_unavailable", "Could not read local frontend targets.")
		return
	}
	digest, err := manifest.Digest(current)
	if err != nil || body.BaseDigest != digest {
		writeJSONError(w, http.StatusConflict, "targets_changed", "Reload frontend targets before saving.")
		return
	}
	if err := publishing.ValidateTargets(body.Targets, doc); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_targets", err.Error())
		return
	}
	if err := workspace.WriteLocalJSON(publishing.TargetPath(h.opts.Workspace), body.Targets); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "targets_write_failed", "Could not save host-local target configuration.")
		return
	}
	writeJSON(w, http.StatusOK, body.Targets)
}

type publishingRequest struct {
	Profile   string `json:"profile"`
	Target    string `json:"target"`
	Approval  string `json:"approval,omitempty"`
	Operation string `json:"operation,omitempty"`
}

func (h *handlers) publishingSelection(body publishingRequest) (publishing.Source, publishing.Target, error) {
	doc, targets, err := h.publishingConfig()
	if err != nil {
		return publishing.Source{}, publishing.Target{}, fmt.Errorf("local frontend configuration unavailable")
	}
	var target publishing.Target
	for _, candidate := range targets.Targets {
		if candidate.ID == body.Target {
			target = candidate
		}
	}
	if target.ID == "" {
		return publishing.Source{}, target, fmt.Errorf("unknown frontend target")
	}
	cfg, found, err := workspace.LoadActiveConfig(h.opts.Workspace.Config)
	if err != nil || !found {
		return publishing.Source{}, target, fmt.Errorf("active source configuration unavailable")
	}
	for _, declared := range doc.Profiles {
		if declared.Key() == body.Profile {
			if target.Platform != declared.Platform {
				return publishing.Source{}, target, fmt.Errorf("profile and frontend platforms must match")
			}
			source, err := publishing.FromArtwork(cfg, declared)
			return source, target, err
		}
	}
	return publishing.Source{}, target, fmt.Errorf("unknown platform profile")
}

func (h *handlers) publishingPreview(w http.ResponseWriter, r *http.Request) {
	h.publishingAction(w, r, "preview")
}

func (h *handlers) publishingStage(w http.ResponseWriter, r *http.Request) {
	h.publishingAction(w, r, "stage")
}

func (h *handlers) publishingPublish(w http.ResponseWriter, r *http.Request) {
	h.publishingAction(w, r, "publish")
}

func (h *handlers) publishingAction(w http.ResponseWriter, r *http.Request, action string) {
	var body publishingRequest
	if err := decodeJSON(r, &body); err != nil {
		writeDecodeError(w, err)
		return
	}
	h.stateMu.RLock()
	defer h.stateMu.RUnlock()
	source, target, err := h.publishingSelection(body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "export_unavailable", workspace.SanitizeFSError(err).Error())
		return
	}
	switch action {
	case "preview":
		plan, err := publishing.Build(source, target)
		if err != nil {
			writeJSONError(w, http.StatusConflict, "preview_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Plan   publishing.Plan   `json:"plan"`
			Target publishing.Target `json:"target"`
		}{plan, target})
	case "stage":
		path, err := h.publisher.Stage(source, target, body.Approval)
		if err != nil {
			writeJSONError(w, http.StatusConflict, "stage_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"path": path})
	case "publish":
		receipt, err := h.publisher.Publish(source, target, body.Approval)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":   map[string]string{"code": "publish_stopped", "message": err.Error()},
				"receipt": receipt,
			})
			return
		}
		writeJSON(w, http.StatusOK, receipt)
	}
}

// Parity is computed from current bytes, never from old publication receipts,
// intended device applicability, metadata readiness or synced sidecar labels.
func (h *handlers) publishingParity(w http.ResponseWriter, r *http.Request) {
	h.stateMu.RLock()
	defer h.stateMu.RUnlock()
	doc, targets, err := h.publishingConfig()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "targets_unavailable", "Could not read local frontend targets.")
		return
	}
	profileKey := r.URL.Query().Get("profile")
	result := []publishing.Plan{}
	var source publishing.Source
	var sourceErr error
	sourceLoaded := false
	for _, target := range targets.Targets {
		if !strings.HasPrefix(profileKey, target.Platform+"/") {
			continue
		}
		if target.Adapter != "steam" {
			result = append(result, publishing.Plan{Profile: profileKey, Target: target.ID,
				State: "unsupported", Message: "No local publishing adapter is enabled for this frontend.", Files: []publishing.File{}})
			continue
		}
		if !sourceLoaded {
			source, _, sourceErr = h.publishingSelection(publishingRequest{Profile: profileKey, Target: target.ID})
			sourceLoaded = true
		}
		if sourceErr != nil {
			state := "unknown"
			if target.Adapter != "steam" {
				state = "unsupported"
			} else if target.Root == "" {
				state = "unobserved"
			}
			result = append(result, publishing.Plan{Profile: profileKey, Target: target.ID,
				State: state, Message: sourceErr.Error(), Files: []publishing.File{}})
			continue
		}
		plan, err := publishing.Build(source, target)
		if err != nil {
			result = append(result, publishing.Plan{Profile: profileKey, Target: target.ID,
				State: "unknown", Message: err.Error(), Files: []publishing.File{}})
			continue
		}
		result = append(result, plan)
	}
	writeJSON(w, http.StatusOK, struct {
		Plans   []publishing.Plan   `json:"plans"`
		Targets []publishing.Target `json:"targets"`
		Devices []topology.Device   `json:"devices"`
	}{result, targets.Targets, doc.Devices})
}

func (h *handlers) publishingHistory(w http.ResponseWriter, r *http.Request) {
	receipts, err := h.publisher.History()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "history_unavailable", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, receipts)
}

func (h *handlers) publishingRollbackPreview(w http.ResponseWriter, r *http.Request) {
	h.publishingRestore(w, r, false)
}

func (h *handlers) publishingRollback(w http.ResponseWriter, r *http.Request) {
	h.publishingRestore(w, r, true)
}

func (h *handlers) publishingRestore(w http.ResponseWriter, r *http.Request, apply bool) {
	var body publishingRequest
	if err := decodeJSON(r, &body); err != nil {
		writeDecodeError(w, err)
		return
	}
	h.stateMu.RLock()
	defer h.stateMu.RUnlock()
	_, targets, err := h.publishingConfig()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "targets_unavailable", "Could not read local frontend targets.")
		return
	}
	for _, target := range targets.Targets {
		if target.ID != body.Target {
			continue
		}
		if apply {
			receipt, err := h.publisher.Rollback(target, body.Operation, body.Approval)
			if err != nil {
				writeJSONError(w, http.StatusConflict, "rollback_stopped", err.Error())
				return
			}
			writeJSON(w, http.StatusOK, receipt)
		} else {
			preview, err := h.publisher.PreviewRollback(target, body.Operation)
			if err != nil {
				writeJSONError(w, http.StatusConflict, "rollback_unavailable", err.Error())
				return
			}
			writeJSON(w, http.StatusOK, struct {
				publishing.Rollback
				Target publishing.Target `json:"target"`
			}{preview, target})
		}
		return
	}
	writeJSONError(w, http.StatusBadRequest, "unknown_target", "Unknown frontend target.")
}
