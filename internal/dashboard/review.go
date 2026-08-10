package dashboard

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jrmoulckers/game-library/internal/config"
	"github.com/jrmoulckers/game-library/internal/manifest"
	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/organizer"
	"github.com/jrmoulckers/game-library/internal/profile"
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
	snapshot, ok := h.loadOrganizerSnapshot(w, cfg)
	if !ok {
		return
	}
	profiles, ok := h.loadAllProfileDrafts(w)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, organizer.Build(snapshot, profiles))
}

func (h *handlers) organizerGame(w http.ResponseWriter, r *http.Request) {
	h.stateMu.RLock()
	defer h.stateMu.RUnlock()
	cfg, ok := h.requireActiveConfig(w)
	if !ok {
		return
	}
	snapshot, ok := h.loadOrganizerSnapshot(w, cfg)
	if !ok {
		return
	}

	profiles, ok := h.loadAllProfileDrafts(w)
	if !ok {
		return
	}
	game, found := organizer.FindGame(organizer.Build(snapshot, profiles), r.PathValue("id"))
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
	writeJSON(w, http.StatusOK, map[string]any{
		"sources":       source.Detect(source.Environment{GOOS: runtime.GOOS, HomeDir: home}, cfg.Roots),
		"caseSensitive": runtime.GOOS != "windows",
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

func (h *handlers) reviewOverview(w http.ResponseWriter, r *http.Request) {
	cfg, ok := h.requireActiveConfig(w)
	if !ok {
		return
	}
	snapshot, ok := h.loadReviewSnapshot(w, cfg)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, review.BuildOverview(snapshot, review.Clock()))
}

// refreshView is the response shape for POST /api/review/refresh: explicit
// status/progress-compatible metadata about the in-memory snapshot,
// without ever persisting the underlying private inventory to disk.
type refreshView struct {
	// Status is "completed" when this request performed a fresh load, or
	// "in-progress" when another refresh was already running and this
	// request did not start a second one.
	Status           string `json:"status"`
	Source           string `json:"source,omitempty"`
	ScannedAt        string `json:"scannedAt,omitempty"`
	RootCount        int    `json:"rootCount,omitempty"`
	ObservationCount int    `json:"observationCount,omitempty"`
}

// reviewRefresh refreshes the in-memory review snapshot cache on demand.
// It permits only one in-flight refresh at a time (see
// snapshotCache.tryBeginRefresh): a concurrent second request observes
// status "in-progress" and returns immediately rather than triggering a
// duplicate scan. The refreshed snapshot is only ever cached in memory; it
// is never written to disk by this endpoint.
func (h *handlers) reviewRefresh(w http.ResponseWriter, r *http.Request) {
	cfg, ok := h.requireActiveConfig(w)
	if !ok {
		return
	}
	if !h.snapshots.tryBeginRefresh() {
		writeJSON(w, http.StatusConflict, refreshView{Status: "in-progress"})
		return
	}
	defer h.snapshots.endRefresh()

	snapshot, err := h.snapshots.reload(cfg, h.opts.InventoryReport)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to refresh the inventory snapshot")
		return
	}
	writeJSON(w, http.StatusOK, refreshView{
		Status:           "completed",
		Source:           string(snapshot.Source),
		ScannedAt:        formatScanTime(snapshot),
		RootCount:        len(snapshot.Inventory.Roots),
		ObservationCount: len(snapshot.Inventory.Observations),
	})
}

func formatScanTime(snapshot review.Snapshot) string {
	if snapshot.ScannedAt.IsZero() {
		return ""
	}
	return snapshot.ScannedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
}

func (h *handlers) reviewObservations(w http.ResponseWriter, r *http.Request) {
	cfg, ok := h.requireActiveConfig(w)
	if !ok {
		return
	}
	snapshot, ok := h.loadReviewSnapshot(w, cfg)
	if !ok {
		return
	}
	profiles, ok := h.loadAllProfileDrafts(w)
	if !ok {
		return
	}

	query := r.URL.Query()
	filter := review.ObservationFilter{
		Source:        query.Get("source"),
		System:        query.Get("system"),
		Identity:      query.Get("identity"),
		Role:          query.Get("role"),
		Dimensions:    query.Get("dimensions"),
		PolicyOutcome: query.Get("policyOutcome"),
		Theme:         query.Get("theme"),
		Validation:    query.Get("validation"),
	}
	page, _ := strconv.Atoi(query.Get("page"))
	pageSize, _ := strconv.Atoi(query.Get("pageSize"))

	writeJSON(w, http.StatusOK, review.ListObservations(snapshot, cfg.Policy, profiles, filter, page, pageSize))
}

// isPreviewableImage is the allowlist check for the inline thumbnail
// endpoint (reviewMedia): only image/* MIME types are ever rendered
// inline, even though review.ResolveMedia itself is a general-purpose
// resolver that can identify PDFs, videos, and other content (issue #7).
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
	w.Header().Set("Cache-Control", "no-store")
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

func (h *handlers) reviewIdentity(w http.ResponseWriter, r *http.Request) {
	cfg, ok := h.requireActiveConfig(w)
	if !ok {
		return
	}
	snapshot, ok := h.loadReviewSnapshot(w, cfg)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, review.BuildIdentityView(snapshot))
}

func (h *handlers) reviewDuplicates(w http.ResponseWriter, r *http.Request) {
	cfg, ok := h.requireActiveConfig(w)
	if !ok {
		return
	}
	snapshot, ok := h.loadReviewSnapshot(w, cfg)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, review.BuildDuplicateView(snapshot))
}

func (h *handlers) reviewPolicyImpact(w http.ResponseWriter, r *http.Request) {
	cfg, ok := h.requireActiveConfig(w)
	if !ok {
		return
	}
	snapshot, ok := h.loadReviewSnapshot(w, cfg)
	if !ok {
		return
	}
	view, err := review.BuildPolicyImpact(snapshot, cfg.Policy)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_policy", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// policyImpactPreviewRequest carries an arbitrary (not-yet-saved) policy
// body — typically the policy draft currently being edited — so a
// reviewer can see its impact before ever saving it, let alone promoting
// it anywhere.
type policyImpactPreviewRequest struct {
	Policy model.PolicyFile `json:"policy"`
}

// reviewPolicyImpactPreview previews review.BuildPolicyImpact for a
// caller-supplied policy body against the current snapshot, complementing
// reviewPolicyImpact (which always uses the active configuration's saved
// policy). It never writes anything: this is a read-only "what would this
// draft do" preview, and never implies the previewed policy has been
// promoted or saved anywhere.
func (h *handlers) reviewPolicyImpactPreview(w http.ResponseWriter, r *http.Request) {
	var body policyImpactPreviewRequest
	if err := decodeJSON(r, &body); err != nil {
		writeDecodeError(w, err)
		return
	}
	cfg, ok := h.requireActiveConfig(w)
	if !ok {
		return
	}
	snapshot, ok := h.loadReviewSnapshot(w, cfg)
	if !ok {
		return
	}
	view, err := review.BuildPolicyImpact(snapshot, body.Policy)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_policy", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, view)
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
func (h *handlers) reviewThemes(w http.ResponseWriter, r *http.Request) {
	catalogRoot, ok := h.catalogRootOrError(w)
	if !ok {
		return
	}
	ids, err := review.ListSafeThemeIDs(catalogRoot)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to list catalog themes")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Themes []string `json:"themes"`
	}{ids})
}

// destinationRootResolver builds the server-owned RootResolver manifest
// analysis and adapter status use (issue #2): "catalog" always resolves
// exclusively to h.opts.CatalogRoot (never a config-supplied root of the
// same id/kind, and never anything client-supplied), and every other
// symbolic name resolves only to a configured root whose ID or Kind
// exactly matches that name, preferring an ID match. A name absent from
// the result is, by construction, not configured.
func (h *handlers) destinationRootResolver(cfg model.Config) review.RootResolver {
	resolver := review.RootResolver{}
	if h.opts.CatalogRoot != "" {
		resolver["catalog"] = h.opts.CatalogRoot
	}
	for _, root := range cfg.Roots {
		if root.ID == "catalog" {
			continue
		}
		if _, exists := resolver[root.ID]; !exists {
			resolver[root.ID] = root.Path
		}
	}
	for _, root := range cfg.Roots {
		if root.Kind == "catalog" {
			continue
		}
		if _, exists := resolver[root.Kind]; !exists {
			resolver[root.Kind] = root.Path
		}
	}
	return resolver
}

// adapterStatusView reports one adapter's plan-only capability and
// server-side readiness — never a filesystem path (issue #5).
type adapterStatusView struct {
	Adapter               string `json:"adapter"`
	PlanOnly              bool   `json:"planOnly"`
	DestinationConfigured bool   `json:"destinationConfigured"`
	InputReady            bool   `json:"inputReady"`
}

// reviewAdapterStatus reports, for every supported frontend adapter,
// whether a destination root is currently configured for it and whether at
// least one saved profile draft has assets that adapter's export plan
// would actually use. Every adapter in this repository is plan-only: there
// is no publish/apply endpoint for any of them.
func (h *handlers) reviewAdapterStatus(w http.ResponseWriter, r *http.Request) {
	cfg, ok := h.loadConfigLeniently(w)
	if !ok {
		return
	}
	profiles, ok := h.loadAllProfileDrafts(w)
	if !ok {
		return
	}
	resolver := h.destinationRootResolver(cfg)

	statuses := make([]adapterStatusView, 0, len(adapterNames))
	for _, adapter := range adapterNames {
		_, destinationConfigured := resolver.Resolve(adapter)
		inputReady := false
		for _, draft := range profiles {
			if _, err := profile.BuildExportPlan(adapter, draft); err == nil {
				inputReady = true
				break
			}
		}
		statuses = append(statuses, adapterStatusView{
			Adapter:               adapter,
			PlanOnly:              true,
			DestinationConfigured: destinationConfigured,
			InputReady:            inputReady,
		})
	}
	writeJSON(w, http.StatusOK, statuses)
}

// reviewHistory lists every immutable local plan/gate-review artifact by
// symbolic reference, with digest/integrity status and no absolute paths
// (issue #5).
func (h *handlers) reviewHistory(w http.ResponseWriter, r *http.Request) {
	entries, err := review.ListHistory(h.opts.Workspace)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to list local artifact history")
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

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

// validateSetupRoots validates candidate setup roots (existence,
// directory-ness, readability) and flags case-colliding IDs, all without
// writing anything and without ever echoing a filesystem path back in the
// response (issue #5).
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

func (h *handlers) reviewProfileResolve(w http.ResponseWriter, r *http.Request) {
	catalogRoot, ok := h.catalogRootOrError(w)
	if !ok {
		return
	}
	profileDraft, ok := h.loadProfileDraftOrError(w, r.PathValue("id"))
	if !ok {
		return
	}
	resolution, err := review.PreviewProfileResolve(profileDraft, catalogRoot)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_profile", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resolution)
}

func (h *handlers) reviewExportPreview(w http.ResponseWriter, r *http.Request) {
	profileDraft, ok := h.loadProfileDraftOrError(w, r.PathValue("id"))
	if !ok {
		return
	}
	preview, err := review.PreviewExportPlan(profileDraft, r.PathValue("adapter"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_export_preview", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

// planPersistedView never carries a filesystem path: Artifact is the
// workspace-relative symbolic reference to the persisted plan artifact
// (see handlers.artifactRef), e.g. "plans/import-plan/import-abc123.json"
// (issue #1).
type planPersistedView struct {
	Plan    model.Manifest `json:"plan"`
	Created bool           `json:"created"`
	// Digest is manifest.Digest(Plan): the same content digest
	// review.AnalyzeManifest computes as ManifestAnalysis.ManifestDigest
	// for this exact plan, and the same value a Gate B review can cite as
	// GateBReview.ExportPlanDigest, so a client never needs to recompute
	// it itself.
	Digest   string `json:"digest"`
	Artifact string `json:"artifact"`
}

// planDigestOrError computes planPersistedView.Digest, writing a
// sanitized 500 and returning ok=false if the plan cannot be digested
// (which would indicate an internal encoding bug, never a client input
// problem, since plan was just built and validated by this package).
func (h *handlers) planDigestOrError(w http.ResponseWriter, plan model.Manifest) (string, bool) {
	digest, err := manifest.Digest(plan)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to digest the generated plan")
		return "", false
	}
	return digest, true
}

func (h *handlers) reviewPlanImport(w http.ResponseWriter, r *http.Request) {
	cfg, ok := h.requireActiveConfig(w)
	if !ok {
		return
	}
	snapshot, ok := h.loadReviewSnapshot(w, cfg)
	if !ok {
		return
	}
	if snapshot.Inventory.Privacy != "private" || len(snapshot.Inventory.Observations) == 0 {
		writeJSONError(w, http.StatusBadRequest, "private_inventory_required", "import planning requires a private inventory with observations")
		return
	}
	plan, created, artifactPath, err := review.BuildAndPersistImportPlan(h.opts.Workspace, snapshot.Inventory, cfg.Policy)
	if err != nil {
		if errors.Is(err, workspace.ErrConflict) {
			writeJSONError(w, http.StatusConflict, "plan_conflict", "an artifact with this plan's id already exists with different content")
			return
		}
		writeJSONError(w, http.StatusBadRequest, "invalid_plan", err.Error())
		return
	}
	artifact, ok := h.artifactRef(w, artifactPath)
	if !ok {
		return
	}
	digest, ok := h.planDigestOrError(w, plan)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, planPersistedView{Plan: plan, Created: created, Digest: digest, Artifact: artifact})
}

type bundlePlanRequest struct {
	ProfileID string `json:"profileId"`
}

func (h *handlers) reviewPlanBundle(w http.ResponseWriter, r *http.Request) {
	catalogRoot, ok := h.catalogRootOrError(w)
	if !ok {
		return
	}
	var body bundlePlanRequest
	if err := decodeJSON(r, &body); err != nil {
		writeDecodeError(w, err)
		return
	}
	profileDraft, ok := h.loadProfileDraftOrError(w, body.ProfileID)
	if !ok {
		return
	}
	plan, resolution, created, artifactPath, err := review.BuildAndPersistBundlePlan(h.opts.Workspace, profileDraft, catalogRoot)
	if err != nil {
		if errors.Is(err, workspace.ErrConflict) {
			writeJSONError(w, http.StatusConflict, "plan_conflict", "an artifact with this plan's id already exists with different content")
			return
		}
		writeJSONError(w, http.StatusBadRequest, "invalid_plan", err.Error())
		return
	}
	artifact, ok := h.artifactRef(w, artifactPath)
	if !ok {
		return
	}
	digest, ok := h.planDigestOrError(w, plan)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, struct {
		planPersistedView
		Resolution model.ProfileResolution `json:"resolution"`
	}{planPersistedView{Plan: plan, Created: created, Digest: digest, Artifact: artifact}, resolution})
}

type exportPlanRequest struct {
	ProfileID string `json:"profileId"`
	Adapter   string `json:"adapter"`
}

func (h *handlers) reviewPlanExport(w http.ResponseWriter, r *http.Request) {
	var body exportPlanRequest
	if err := decodeJSON(r, &body); err != nil {
		writeDecodeError(w, err)
		return
	}
	profileDraft, ok := h.loadProfileDraftOrError(w, body.ProfileID)
	if !ok {
		return
	}
	plan, created, artifactPath, err := review.BuildAndPersistExportPlan(h.opts.Workspace, profileDraft, body.Adapter)
	if err != nil {
		if errors.Is(err, workspace.ErrConflict) {
			writeJSONError(w, http.StatusConflict, "plan_conflict", "an artifact with this plan's id already exists with different content")
			return
		}
		writeJSONError(w, http.StatusBadRequest, "invalid_plan", err.Error())
		return
	}
	artifact, ok := h.artifactRef(w, artifactPath)
	if !ok {
		return
	}
	digest, ok := h.planDigestOrError(w, plan)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, planPersistedView{Plan: plan, Created: created, Digest: digest, Artifact: artifact})
}

// manifestAnalysisRequest intentionally has no destinationRoot field: every
// destination root is resolved exclusively through server-owned configured
// symbolic roots (see destinationRootResolver), never a client-controlled
// path (issue #2). decodeJSON rejects unknown fields, so a client that
// still sends "destinationRoot" gets a clear 400 rather than the field
// being silently ignored.
type manifestAnalysisRequest struct {
	Manifest model.Manifest `json:"manifest"`
}

func (h *handlers) reviewManifestAnalysis(w http.ResponseWriter, r *http.Request) {
	var body manifestAnalysisRequest
	if err := decodeJSON(r, &body); err != nil {
		writeDecodeError(w, err)
		return
	}
	cfg, ok := h.loadConfigLeniently(w)
	if !ok {
		return
	}
	resolver := h.destinationRootResolver(cfg)
	analysis, err := review.AnalyzeManifest(body.Manifest, resolver)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_manifest", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, analysis)
}

func (h *handlers) reviewGateA(w http.ResponseWriter, r *http.Request) {
	var body review.GateAReview
	if err := decodeJSON(r, &body); err != nil {
		writeDecodeError(w, err)
		return
	}
	result, created, artifactPath, err := review.CreateGateAReview(h.opts.Workspace, body, review.Clock)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_gate_review", err.Error())
		return
	}
	artifact, ok := h.artifactRef(w, artifactPath)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Review   review.GateAReview `json:"review"`
		Created  bool               `json:"created"`
		Artifact string             `json:"artifact"`
	}{result, created, artifact})
}

func (h *handlers) reviewGateB(w http.ResponseWriter, r *http.Request) {
	var body review.GateBReview
	if err := decodeJSON(r, &body); err != nil {
		writeDecodeError(w, err)
		return
	}
	result, created, artifactPath, err := review.CreateGateBReview(h.opts.Workspace, body, review.Clock)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_gate_review", err.Error())
		return
	}
	artifact, ok := h.artifactRef(w, artifactPath)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Review   review.GateBReview `json:"review"`
		Created  bool               `json:"created"`
		Artifact string             `json:"artifact"`
	}{result, created, artifact})
}

func (h *handlers) reviewGateC(w http.ResponseWriter, r *http.Request) {
	var body review.GateCReview
	if err := decodeJSON(r, &body); err != nil {
		writeDecodeError(w, err)
		return
	}
	// Executable is always forced to false inside CreateGateCReview,
	// regardless of whatever the request body set it to: this repository
	// has no apply/publish/delete/prune/rollback capability, and Gate C
	// never authorizes execution.
	result, created, artifactPath, err := review.CreateGateCReview(h.opts.Workspace, body, review.Clock)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_gate_review", err.Error())
		return
	}
	artifact, ok := h.artifactRef(w, artifactPath)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Review   review.GateCReview `json:"review"`
		Created  bool               `json:"created"`
		Artifact string             `json:"artifact"`
	}{result, created, artifact})
}
