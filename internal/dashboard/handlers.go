package dashboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"sync"

	"github.com/jrmoulckers/game-library/internal/config"
	"github.com/jrmoulckers/game-library/internal/manifest"
	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/policy"
	"github.com/jrmoulckers/game-library/internal/profile"
	"github.com/jrmoulckers/game-library/internal/workspace"
)

type handlers struct {
	opts      Options
	csrfToken string
	snapshots *snapshotCache
	scans     *scanManager
	metadata  *metadataCache
	stateMu   sync.RWMutex
}

func (h *handlers) mux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", h.index)
	mux.HandleFunc("GET /static/app.css", h.staticCSS)
	mux.HandleFunc("GET /static/js/{name}", h.staticJS)
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("GET /api/bootstrap", h.bootstrap)
	mux.HandleFunc("GET /api/organizer", h.organizerCatalog)
	mux.HandleFunc("GET /api/organizer/games/{id}", h.organizerGame)
	mux.HandleFunc("POST /api/organizer/scan", h.startOrganizerScan)
	mux.HandleFunc("GET /api/organizer/scan", h.organizerScanStatus)
	mux.HandleFunc("GET /api/sources/detect", h.detectSources)
	mux.HandleFunc("GET /api/config", h.getConfig)
	mux.HandleFunc("PUT /api/config", h.putConfig)
	mux.HandleFunc("GET /api/drafts/policy", h.getPolicyDraft)
	mux.HandleFunc("PUT /api/drafts/policy", h.putPolicyDraft)
	mux.HandleFunc("GET /api/drafts/profiles/{id}", h.getProfileDraft)
	mux.HandleFunc("PUT /api/drafts/profiles/{id}", h.putProfileDraft)
	mux.HandleFunc("GET /api/review/overview", h.reviewOverview)
	mux.HandleFunc("POST /api/review/refresh", h.reviewRefresh)
	mux.HandleFunc("GET /api/review/observations", h.reviewObservations)
	mux.HandleFunc("GET /api/review/media/{id}", h.reviewMedia)
	mux.HandleFunc("GET /api/review/media/{id}/download", h.reviewMediaDownload)
	mux.HandleFunc("GET /api/review/identity", h.reviewIdentity)
	mux.HandleFunc("GET /api/review/duplicates", h.reviewDuplicates)
	mux.HandleFunc("GET /api/review/policy-impact", h.reviewPolicyImpact)
	mux.HandleFunc("POST /api/review/policy-impact-preview", h.reviewPolicyImpactPreview)
	mux.HandleFunc("GET /api/review/profiles", h.reviewProfileDrafts)
	mux.HandleFunc("GET /api/review/themes", h.reviewThemes)
	mux.HandleFunc("GET /api/review/adapters", h.reviewAdapterStatus)
	mux.HandleFunc("GET /api/review/history", h.reviewHistory)
	mux.HandleFunc("POST /api/config/validate-roots", h.validateSetupRoots)
	mux.HandleFunc("GET /api/review/profiles/{id}/resolve", h.reviewProfileResolve)
	mux.HandleFunc("GET /api/review/profiles/{id}/export/{adapter}", h.reviewExportPreview)
	mux.HandleFunc("POST /api/review/plans/import", h.reviewPlanImport)
	mux.HandleFunc("POST /api/review/plans/bundle", h.reviewPlanBundle)
	mux.HandleFunc("POST /api/review/plans/export", h.reviewPlanExport)
	mux.HandleFunc("POST /api/review/manifest-analysis", h.reviewManifestAnalysis)
	mux.HandleFunc("POST /api/review/gates/a", h.reviewGateA)
	mux.HandleFunc("POST /api/review/gates/b", h.reviewGateB)
	mux.HandleFunc("POST /api/review/gates/c", h.reviewGateC)
	return mux
}

func (h *handlers) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type bootstrapView struct {
	ToolVersion   string `json:"toolVersion"`
	SchemaVersion int    `json:"schemaVersion"`
	CSRFToken     string `json:"csrfToken"`
	CSRFHeader    string `json:"csrfHeader"`
	// WorkspaceRoot and ConfigPath are the host-local directory and file
	// this process was started with (via `gamelib serve --workspace`/
	// `--config`, or their platform-local defaults). They are the user's
	// own machine paths for the dashboard's own local-only workspace —
	// never a canonical library, artwork, or homelab path — and are
	// surfaced only so the first-run setup screen can tell the user
	// exactly which file "Save configuration" writes.
	WorkspaceRoot string `json:"workspaceRoot"`
	ConfigPath    string `json:"configPath"`
}

func (h *handlers) bootstrap(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, bootstrapView{
		ToolVersion:   model.ToolVersion,
		SchemaVersion: model.SchemaVersion,
		CSRFToken:     h.csrfToken,
		CSRFHeader:    csrfHeader,
		WorkspaceRoot: h.opts.Workspace.Root,
		ConfigPath:    h.opts.Workspace.Config,
	})
}

type configView struct {
	Exists bool          `json:"exists"`
	Config *model.Config `json:"config,omitempty"`
	Digest string        `json:"digest,omitempty"`
	// PolicyDigest is manifest.Digest(cfg.Policy) alone — distinct from
	// Digest (the whole active configuration) — so a dashboard client can
	// cite the exact active-policy digest a Gate B review references
	// (GateBReview.PolicyDigest) without recomputing it itself.
	PolicyDigest string `json:"policyDigest,omitempty"`
}

// configRequest wraps a PUT /api/config body with a baseDigest optimistic-
// concurrency precondition, matching the same pattern policy/profile
// drafts already use: baseDigest must equal the digest of whatever active
// configuration currently exists (or "" when none does), or the write is
// rejected with 409 rather than silently overwriting a concurrent editor's
// change.
type configRequest struct {
	BaseDigest string       `json:"baseDigest"`
	Config     model.Config `json:"config"`
}

func (h *handlers) getConfig(w http.ResponseWriter, r *http.Request) {
	cfg, found, err := workspace.LoadActiveConfig(h.opts.Workspace.Config)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to read the active configuration")
		return
	}
	view := configView{Exists: found}
	if found {
		view.Config = &cfg
		if digest, err := manifest.Digest(cfg); err == nil {
			view.Digest = digest
		}
		if policyDigest, err := manifest.Digest(cfg.Policy); err == nil {
			view.PolicyDigest = policyDigest
		}
	}
	writeJSON(w, http.StatusOK, view)
}

func (h *handlers) putConfig(w http.ResponseWriter, r *http.Request) {
	var body configRequest
	if err := decodeJSON(r, &body); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := config.Validate(body.Config); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_config", err.Error())
		return
	}
	h.stateMu.Lock()
	defer h.stateMu.Unlock()
	if err := workspace.WriteActiveConfig(h.opts.Workspace.Config, body.BaseDigest, body.Config); err != nil {
		if errors.Is(err, workspace.ErrConflict) {
			writeJSONError(w, http.StatusConflict, "config_conflict", "base digest is stale; reload the active configuration and retry")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to save the active configuration")
		return
	}
	// The configured roots may have just changed: any cached review
	// snapshot was computed against the previous configuration and must
	// never be served again.
	h.scans.invalidate()
	h.metadata.invalidate()
	if err := h.snapshots.invalidateFor(body.Config); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to update the inventory snapshot state")
		return
	}
	digest, err := manifest.Digest(body.Config)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to digest the saved configuration")
		return
	}
	writeJSON(w, http.StatusOK, configView{Exists: true, Config: &body.Config, Digest: digest})
}

type policyDraftView struct {
	Exists bool                           `json:"exists"`
	Draft  *workspace.PolicyDraftEnvelope `json:"draft,omitempty"`
}

func (h *handlers) getPolicyDraft(w http.ResponseWriter, r *http.Request) {
	draft, found, err := workspace.LoadPolicyDraft(h.opts.Workspace)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to read the policy draft")
		return
	}
	view := policyDraftView{Exists: found}
	if found {
		view.Draft = &draft
	}
	writeJSON(w, http.StatusOK, view)
}

type policyDraftRequest struct {
	BaseDigest string           `json:"baseDigest"`
	Policy     model.PolicyFile `json:"policy"`
}

func (h *handlers) putPolicyDraft(w http.ResponseWriter, r *http.Request) {
	var body policyDraftRequest
	if err := decodeJSON(r, &body); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := policy.Validate(body.Policy); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_policy", err.Error())
		return
	}
	envelope, err := workspace.SavePolicyDraft(h.opts.Workspace, body.BaseDigest, body.Policy)
	if err != nil {
		if errors.Is(err, workspace.ErrConflict) {
			writeJSONError(w, http.StatusConflict, "draft_conflict", "base digest is stale; reload the draft and retry")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to save the policy draft")
		return
	}
	writeJSON(w, http.StatusOK, envelope)
}

type profileDraftView struct {
	Exists bool                            `json:"exists"`
	Draft  *workspace.ProfileDraftEnvelope `json:"draft,omitempty"`
}

func (h *handlers) getProfileDraft(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	draft, found, err := workspace.LoadProfileDraft(h.opts.Workspace, id)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_profile_id", err.Error())
		return
	}
	view := profileDraftView{Exists: found}
	if found {
		view.Draft = &draft
	}
	writeJSON(w, http.StatusOK, view)
}

type profileDraftRequest struct {
	BaseDigest string        `json:"baseDigest"`
	Profile    model.Profile `json:"profile"`
}

func (h *handlers) putProfileDraft(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body profileDraftRequest
	if err := decodeJSON(r, &body); err != nil {
		writeDecodeError(w, err)
		return
	}
	if body.Profile.ID != id {
		writeJSONError(w, http.StatusBadRequest, "profile_id_mismatch", "profile id in the request body must match the URL")
		return
	}
	if err := profile.Validate(body.Profile); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_profile", err.Error())
		return
	}
	envelope, err := workspace.SaveProfileDraft(h.opts.Workspace, body.BaseDigest, body.Profile)
	if err != nil {
		if errors.Is(err, workspace.ErrConflict) {
			writeJSONError(w, http.StatusConflict, "draft_conflict", "base digest is stale; reload the draft and retry")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to save the profile draft")
		return
	}
	writeJSON(w, http.StatusOK, envelope)
}

// decodeErrors distinguish media-type and payload-size failures from
// generic malformed-JSON failures so handlers can map them to the right
// HTTP status without inspecting error strings.
var (
	errUnsupportedMediaType = errors.New("unsupported media type")
	errBodyTooLarge         = errors.New("request body too large")
	errMalformedJSON        = errors.New("malformed JSON body")
)

func decodeJSON(r *http.Request, dst any) error {
	ct := r.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(ct)
	if err != nil || mediaType != "application/json" {
		return errUnsupportedMediaType
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return errBodyTooLarge
		}
		return fmt.Errorf("%w: %v", errMalformedJSON, err)
	}
	// dec.More() only reports whether another element follows within the
	// array/object currently being parsed; after a single top-level Decode
	// it does not reliably surface trailing content following that value
	// (for example a second concatenated JSON value). Attempting a second
	// Decode and requiring it to fail with exactly io.EOF is the documented
	// way to detect trailing data after a single JSON value.
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("%w: trailing data after JSON value", errMalformedJSON)
		}
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return errBodyTooLarge
		}
		return fmt.Errorf("%w: trailing data after JSON value: %v", errMalformedJSON, err)
	}
	return nil
}

func writeDecodeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errUnsupportedMediaType):
		writeJSONError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "request body must be application/json")
	case errors.Is(err, errBodyTooLarge):
		writeJSONError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "request body exceeds the size limit")
	default:
		writeJSONError(w, http.StatusBadRequest, "malformed_json", "request body is not valid JSON")
	}
}
