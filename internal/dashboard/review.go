package dashboard

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/jrmoulckers/game-library/internal/config"
	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/organizer"
	"github.com/jrmoulckers/game-library/internal/review"
	"github.com/jrmoulckers/game-library/internal/source"
	"github.com/jrmoulckers/game-library/internal/workspace"
)

// adapterNames lists every frontend export adapter this repository
// supports, in a fixed, deterministic order. It is the single source of
// truth for both the adapter-status endpoint and destination-root
// resolution: every other reference to "the list of adapters" in this
// package must derive from it, not repeat it.
var adapterNames = []string{"steam", "playnite", "decky", "esde", "romm"}

// requireActiveConfig loads and validates the active configuration,
// writing a sanitized error response and returning ok=false when it is
// absent or unreadable. Every review endpoint needs a configured set of
// roots (or, when InventoryReport is set, at least the policy) to operate
// against, so this is the shared entry point for all of them.
func (h *handlers) requireActiveConfig(w http.ResponseWriter) (model.Config, bool) {
	cfg, found, err := workspace.LoadActiveConfig(h.opts.Workspace.Config)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to read the active configuration")
		return model.Config{}, false
	}
	if !found {
		writeJSONError(w, http.StatusBadRequest, "config_missing", "save an active configuration before using the review surface")
		return model.Config{}, false
	}
	return cfg, true
}

// loadConfigLeniently loads the active configuration if one exists, and
// otherwise returns a zero-value model.Config with ok=true: unlike
// requireActiveConfig, an absent configuration is not itself an error for
// endpoints (adapter status, manifest analysis) that can still usefully
// answer "nothing configured yet" rather than refusing to respond at all.
func (h *handlers) loadConfigLeniently(w http.ResponseWriter) (model.Config, bool) {
	cfg, found, err := workspace.LoadActiveConfig(h.opts.Workspace.Config)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to read the active configuration")
		return model.Config{}, false
	}
	if !found {
		return model.Config{}, true
	}
	return cfg, true
}

// loadReviewSnapshot returns the process-lifetime cached review snapshot,
// loading it on first use. See snapshotcache.go: this never rescans the
// configured roots on every request the way calling review.LoadSnapshot
// directly would.
func (h *handlers) loadReviewSnapshot(w http.ResponseWriter, cfg model.Config) (review.Snapshot, bool) {
	snapshot, err := h.snapshots.getOrLoad(cfg, h.opts.InventoryReport)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to load the inventory snapshot")
		return review.Snapshot{}, false
	}
	return snapshot, true
}

// loadAllProfileDrafts loads every currently saved profile draft, used for
// theme-membership filtering. It is not an error for there to be none yet.
func (h *handlers) loadAllProfileDrafts(w http.ResponseWriter) ([]model.Profile, bool) {
	ids, err := workspace.ListProfileDraftIDs(h.opts.Workspace)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to list profile drafts")
		return nil, false
	}

	profiles := make([]model.Profile, 0, len(ids))
	for _, id := range ids {
		draft, found, err := workspace.LoadProfileDraft(h.opts.Workspace, id)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to read a profile draft")
			return nil, false
		}
		if found {
			profiles = append(profiles, draft.Profile)
		}
	}
	return profiles, true
}

func (h *handlers) organizerCatalog(w http.ResponseWriter, r *http.Request) {
	h.stateMu.RLock()
	defer h.stateMu.RUnlock()
	cfg, ok := h.requireActiveConfig(w)
	if !ok {
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
	view := organizer.BuildWithMetadata(snapshot, profiles, titles)
	writeJSON(w, http.StatusOK, struct {
		organizer.Catalog
		MetadataStatus string `json:"metadataStatus"`
	}{Catalog: view, MetadataStatus: metadataStatus})
}

func (h *handlers) organizerGame(w http.ResponseWriter, r *http.Request) {
	h.stateMu.RLock()
	defer h.stateMu.RUnlock()
	cfg, ok := h.requireActiveConfig(w)
	if !ok {
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
	titles, _ := h.metadata.current()
	game, found := organizer.FindGame(organizer.BuildWithMetadata(snapshot, profiles, titles), r.PathValue("id"))
	if !found {
		status := h.scans.snapshot().Status
		if status == "scanning" || status == "cancelling" || status == "idle" {
			writeJSONError(w, http.StatusServiceUnavailable, "scan_starting", "the artwork scan has not reached this game yet")
			return
		}
		writeJSONError(w, http.StatusNotFound, "not_found", "game not found")
		return
	}
	writeJSON(w, http.StatusOK, game)
}

func (h *handlers) loadOrganizerSnapshot(w http.ResponseWriter, cfg model.Config) (review.Snapshot, bool) {
	if h.opts.InventoryReport != "" {
		return h.loadReviewSnapshot(w, cfg)
	}
	snapshot, found, err := h.snapshots.current()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to load the inventory snapshot")
		return review.Snapshot{}, false
	}
	if !found {
		started := h.scans.start(cfg, h.snapshots)
		snapshot, found, err = h.snapshots.current()
		if !found && !started {
			status := h.scans.snapshot().Status
			if status == "scanning" || status == "cancelling" || status == "idle" {
				return review.NewSnapshot(cfg, model.Inventory{
					Version: model.SchemaVersion, ToolVersion: model.ToolVersion,
					CreatedAt: time.Now().UTC().Format(time.RFC3339), Privacy: "private",
				}, review.SourceScan, time.Now().UTC()), true
			}
		}
	}
	if err != nil || !found {
		writeJSONError(w, http.StatusServiceUnavailable, "scan_starting", "the artwork scan is starting")
		return review.Snapshot{}, false
	}
	return snapshot, true
}

func (h *handlers) detectSources(w http.ResponseWriter, r *http.Request) {
	cfg, ok := h.loadConfigLeniently(w)
	if !ok {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to locate the local home directory")
		return
	}
	detected := source.Detect(source.Environment{GOOS: runtime.GOOS, HomeDir: home}, cfg.Roots)
	h.metadata.start(cfg.Roots, false)
	metadataCatalog, metadataStatus := h.metadata.current()
	writeJSON(w, http.StatusOK, map[string]any{
		"sources":             detected,
		"supported":           source.SupportedStates(detected, cfg.Roots),
		"caseSensitive":       runtime.GOOS != "windows",
		"metadataStatus":      metadataStatus,
		"metadataDiagnostics": metadataCatalog.Diagnostics,
	})
}

// artifactRef converts an absolute artifact path (as returned by
// review/workspace artifact-writing functions) into the workspace-relative
// symbolic reference that is safe to return in an API response. It never
// hands the caller anything containing the workspace root or any other
// absolute filesystem detail (issue #1).
func (h *handlers) artifactRef(w http.ResponseWriter, absPath string) (string, bool) {
	ref, err := workspace.RelativeArtifactName(h.opts.Workspace, absPath)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to compute an artifact reference")
		return "", false
	}
	return ref, true
}

func isPreviewableImage(mime string) bool {
	return strings.HasPrefix(mime, "image/")
}

func (h *handlers) reviewMedia(w http.ResponseWriter, r *http.Request) {
	cfg, ok := h.requireActiveConfig(w)
	if !ok {
		return
	}
	snapshot, ok := h.loadReviewSnapshot(w, cfg)
	if !ok {
		return
	}
	resolution, err := review.ResolveMedia(snapshot, r.PathValue("id"))
	if err != nil {
		switch {
		case errors.Is(err, review.ErrMediaNotFound):
			writeJSONError(w, http.StatusNotFound, "media_not_found", "no media matches that id")
		default:
			writeJSONError(w, http.StatusForbidden, "media_unsafe", "this media cannot be served safely")
		}
		return
	}
	if !isPreviewableImage(resolution.MIME) {
		writeJSONError(w, http.StatusForbidden, "media_unsafe", "this media type cannot be previewed inline")
		return
	}
	file, err := os.Open(resolution.Path)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "media_not_found", "no media matches that id")
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", resolution.MIME)
	// Responses carry a content-hash ETag, so caching cannot serve
	// stale bytes; no-store previously forced a full re-download of
	// every artwork asset on each render.
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if resolution.SHA256 != "" {
		w.Header().Set("ETag", `"`+resolution.SHA256+`"`)
	}
	http.ServeContent(w, r, "", review.Clock(), file)
}

// downloadableMIME allowlists the exact set of MIME types the separate
// media download endpoint will ever serve, mapped to a fixed, safe file
// extension used only to build a Content-Disposition filename — never
// derived from, or exposing, any filesystem path.
var downloadableMIME = map[string]string{
	"image/png":        "png",
	"image/jpeg":       "jpg",
	"image/gif":        "gif",
	"image/webp":       "webp",
	"image/x-icon":     "ico",
	"application/pdf":  "pdf",
	"video/mp4":        "mp4",
	"video/webm":       "webm",
	"video/x-matroska": "mkv",
}

// reviewMediaDownload is the separate download path referenced by issue
// #7: unlike the inline thumbnail endpoint (reviewMedia, images only), it
// serves any allowlisted media type but always as an attachment
// (Content-Disposition: attachment), never inline, so a browser never
// renders a non-image response directly.
func (h *handlers) reviewMediaDownload(w http.ResponseWriter, r *http.Request) {
	cfg, ok := h.requireActiveConfig(w)
	if !ok {
		return
	}
	snapshot, ok := h.loadReviewSnapshot(w, cfg)
	if !ok {
		return
	}
	id := r.PathValue("id")
	resolution, err := review.ResolveMedia(snapshot, id)
	if err != nil {
		switch {
		case errors.Is(err, review.ErrMediaNotFound):
			writeJSONError(w, http.StatusNotFound, "media_not_found", "no media matches that id")
		default:
			writeJSONError(w, http.StatusForbidden, "media_unsafe", "this media cannot be served safely")
		}
		return
	}
	ext, allowed := downloadableMIME[resolution.MIME]
	if !allowed {
		writeJSONError(w, http.StatusForbidden, "media_unsafe", "this media type cannot be downloaded")
		return
	}
	file, err := os.Open(resolution.Path)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "media_not_found", "no media matches that id")
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", resolution.MIME)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", id+"."+ext))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if resolution.SHA256 != "" {
		w.Header().Set("ETag", `"`+resolution.SHA256+`"`)
	}
	http.ServeContent(w, r, "", review.Clock(), file)
}

func (h *handlers) catalogRootOrError(w http.ResponseWriter) (string, bool) {
	if h.opts.CatalogRoot == "" {
		writeJSONError(w, http.StatusBadRequest, "catalog_root_missing", "configure a catalog root before using this endpoint")
		return "", false
	}
	return h.opts.CatalogRoot, true
}

func (h *handlers) loadProfileDraftOrError(w http.ResponseWriter, id string) (model.Profile, bool) {
	draft, found, err := workspace.LoadProfileDraft(h.opts.Workspace, id)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_profile_id", err.Error())
		return model.Profile{}, false
	}
	if !found {
		writeJSONError(w, http.StatusNotFound, "profile_draft_not_found", "no profile draft matches that id")
		return model.Profile{}, false
	}
	return draft.Profile, true
}

// profileDraftSummary is a read-only enumeration entry for a saved profile
// draft: only its symbolic id/name/theme, never a filesystem path (issue
// #5).
type profileDraftSummary struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Theme string `json:"theme,omitempty"`
}

// reviewProfileDrafts lists every currently saved profile draft. This is a
// read-only enumeration the dashboard UI needs (issue #5) that previously
// had no endpoint at all.
func (h *handlers) reviewProfileDrafts(w http.ResponseWriter, r *http.Request) {
	profiles, ok := h.loadAllProfileDrafts(w)
	if !ok {
		return
	}
	summaries := make([]profileDraftSummary, 0, len(profiles))
	for _, p := range profiles {
		summaries = append(summaries, profileDraftSummary{ID: p.ID, Name: p.Name, Theme: p.Theme})
	}
	writeJSON(w, http.StatusOK, summaries)
}

// reviewThemes lists the safe theme IDs currently present under the
// configured catalog root's canonical library/themes layout (issue #5): a
// read-only enumeration, never a raw path.
// rootValidationResult reports one candidate root's filesystem readiness
// without ever echoing its path back to the client.
type rootValidationResult struct {
	ID       string   `json:"id"`
	Exists   bool     `json:"exists"`
	IsDir    bool     `json:"isDir"`
	Readable bool     `json:"readable"`
	Issues   []string `json:"issues,omitempty"`
}

type validateRootsRequest struct {
	Roots []model.Root `json:"roots"`
}

type validateRootsResponse struct {
	Results        []rootValidationResult `json:"results"`
	CaseCollisions [][]string             `json:"caseCollisions,omitempty"`
}

func (h *handlers) validateSetupRoots(w http.ResponseWriter, r *http.Request) {
	var body validateRootsRequest
	if err := decodeJSON(r, &body); err != nil {
		writeDecodeError(w, err)
		return
	}

	results := make([]rootValidationResult, 0, len(body.Roots))
	byLowerID := make(map[string][]string)
	for _, root := range body.Roots {
		result := rootValidationResult{ID: root.ID}
		if !config.IsSafeID(root.ID) {
			result.Issues = append(result.Issues, "id is not path-safe")
		}
		// Case-collision detection is independent of path-safety: two IDs
		// that only differ by case would still collide against each
		// other on a case-insensitive filesystem/ID index even if one of
		// them is separately flagged as not path-safe.
		byLowerID[strings.ToLower(root.ID)] = append(byLowerID[strings.ToLower(root.ID)], root.ID)

		info, statErr := os.Stat(root.Path)
		switch {
		case statErr == nil:
			result.Exists = true
			result.IsDir = info.IsDir()
			if result.IsDir {
				if f, openErr := os.Open(root.Path); openErr == nil {
					_, _ = f.Readdirnames(1)
					f.Close()
					result.Readable = true
				} else {
					result.Issues = append(result.Issues, "directory exists but is not readable")
				}
			} else {
				result.Issues = append(result.Issues, "path exists but is not a directory")
			}
		case os.IsNotExist(statErr):
			result.Issues = append(result.Issues, "path does not exist")
		default:
			result.Issues = append(result.Issues, "path could not be inspected")
		}

		results = append(results, result)
	}

	var collisions [][]string
	keys := make([]string, 0, len(byLowerID))
	for k := range byLowerID {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if len(byLowerID[k]) > 1 {
			ids := append([]string(nil), byLowerID[k]...)
			sort.Strings(ids)
			collisions = append(collisions, ids)
		}
	}

	writeJSON(w, http.StatusOK, validateRootsResponse{Results: results, CaseCollisions: collisions})
}
