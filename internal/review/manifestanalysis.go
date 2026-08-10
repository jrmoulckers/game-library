package review

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/jrmoulckers/game-library/internal/manifest"
	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/workspace"
)

// ActionEffect describes what would happen at a given destination if a
// manifest's action were ever executed (this package never executes one).
type ActionEffect string

const (
	// EffectCreate: the destination does not exist yet.
	EffectCreate ActionEffect = "create"
	// EffectReplaceSameHash: the destination already exists and already
	// has the exact content the action expects — an idempotent no-op.
	EffectReplaceSameHash ActionEffect = "replace-same-hash"
	// EffectConflict: the destination exists with content that differs
	// from what the action expects.
	EffectConflict ActionEffect = "conflict"
	// EffectNoFilesystemChange: the action has no destination to check
	// (for example "skip" or "blocked").
	EffectNoFilesystemChange ActionEffect = "no-filesystem-change"
	// EffectRootUnavailable: the action's DestinationRoot does not
	// resolve through the server-owned RootResolver supplied to
	// AnalyzeManifest. This is never a client-supplied filesystem path —
	// it is always an explicit, symbolic "this root is not configured"
	// outcome, reported as a conflict rather than a guess or a server
	// error.
	EffectRootUnavailable ActionEffect = "root-unavailable"
	// EffectWouldRemove: a "remove" action whose destination currently
	// exists. AnalyzeManifest only ever reports this; it never deletes
	// anything, and no planner in this repository currently generates
	// "remove" actions.
	EffectWouldRemove ActionEffect = "would-remove"
)

// ActionAnalysis is a single action's exact, read-only analysis against a
// destination root: nothing here writes to that root.
type ActionAnalysis struct {
	Action                   model.Action `json:"action"`
	Effect                   ActionEffect `json:"effect"`
	CurrentDestinationExists bool         `json:"currentDestinationExists"`
	CurrentDestinationHash   string       `json:"currentDestinationHash,omitempty"`
	CurrentDestinationBytes  int64        `json:"currentDestinationBytes,omitempty"`
	SourceBytes              int64        `json:"sourceBytes,omitempty"`
	Conflict                 bool         `json:"conflict"`
	ConflictReason           string       `json:"conflictReason,omitempty"`
}

// DestinationSpace summarizes, for one symbolic destination root
// referenced by the manifest, the total bytes AnalyzeManifest estimates
// that root will need versus the free space currently observed for it.
// AvailableBytesKnown is false whenever free space could not be determined
// (unsupported platform, or the root does not resolve at all), in which
// case Sufficient carries no meaning and must not be treated as "false
// means insufficient" by a caller.
type DestinationSpace struct {
	Root                string `json:"root"`
	Resolvable          bool   `json:"resolvable"`
	NeededBytes         int64  `json:"neededBytes"`
	AvailableBytes      int64  `json:"availableBytes,omitempty"`
	AvailableBytesKnown bool   `json:"availableBytesKnown"`
	Sufficient          bool   `json:"sufficient"`
}

// ManifestAnalysis is the full Gate C-grade analysis of a manifest: every
// action's exact effect, an aggregate conflict count, estimated
// backup/space needs, per-destination-root free-space sufficiency, plus the
// manifest's own content digest so the analysis can always be tied back to
// the exact plan it describes.
type ManifestAnalysis struct {
	ManifestDigest       string             `json:"manifestDigest"`
	Actions              []ActionAnalysis   `json:"actions"`
	Conflicts            int                `json:"conflicts"`
	EstimatedBackupBytes int64              `json:"estimatedBackupBytes"`
	EstimatedNewBytes    int64              `json:"estimatedNewBytes"`
	EstimatedFreedBytes  int64              `json:"estimatedFreedBytes,omitempty"`
	Destinations         []DestinationSpace `json:"destinations,omitempty"`
	Warnings             []string           `json:"warnings,omitempty"`
}

// RootResolver maps a symbolic destination root name — exactly the values
// model.Action.DestinationRoot already uses, e.g. "catalog" or an adapter
// name such as "steam"/"playnite"/"decky"/"esde"/"romm" — to the
// server-owned absolute filesystem path backing it. It is always built by
// the caller (the dashboard) from the configured catalog root and the
// active configuration's roots, never from a client-supplied path: a name
// that is absent (or maps to an empty path) is treated as "not configured"
// by AnalyzeManifest, never guessed and never a server error.
type RootResolver map[string]string

// Resolve reports the absolute path configured for name, and whether one
// is configured at all (an empty path is treated the same as absent).
func (r RootResolver) Resolve(name string) (string, bool) {
	if r == nil || name == "" {
		return "", false
	}
	path, ok := r[name]
	if !ok || path == "" {
		return "", false
	}
	return path, true
}

// AnalyzeManifest inspects (never writes to) the destination root each of
// plan's actions resolves to via roots, computing the exact effect the
// action would have, the current destination's content hash (if any), and
// backup/space estimates. Every destination is resolved exclusively through
// roots (see RootResolver) — a manifest can never cause this function to
// touch a path outside the server's own configured roots, and an action
// whose DestinationRoot is not present in roots becomes an explicit
// conflict, never a filesystem probe of a client-supplied location.
func AnalyzeManifest(plan model.Manifest, roots RootResolver) (ManifestAnalysis, error) {
	digest, err := manifest.Digest(plan)
	if err != nil {
		return ManifestAnalysis{}, fmt.Errorf("digest manifest: %w", err)
	}

	analysis := ManifestAnalysis{ManifestDigest: digest}
	needsByRoot := make(map[string]int64)
	rootPathsUsed := make(map[string]string)

	for _, action := range plan.Actions {
		entry := ActionAnalysis{Action: action, SourceBytes: action.SourceSize}

		if action.DestinationPath == "" {
			entry.Effect = EffectNoFilesystemChange
			analysis.Actions = append(analysis.Actions, entry)
			continue
		}

		rootPath, ok := roots.Resolve(action.DestinationRoot)
		if !ok {
			entry.Effect = EffectRootUnavailable
			entry.Conflict = true
			entry.ConflictReason = fmt.Sprintf("destination root %q is not configured", action.DestinationRoot)
			analysis.Conflicts++
			analysis.Warnings = append(analysis.Warnings, entry.ConflictReason)
			analysis.Actions = append(analysis.Actions, entry)
			continue
		}
		rootPathsUsed[action.DestinationRoot] = rootPath

		destination, err := workspace.Contain(rootPath, action.DestinationPath)
		if err != nil {
			return ManifestAnalysis{}, fmt.Errorf("action destination %q is unsafe: %w", action.DestinationPath, err)
		}

		info, statErr := os.Lstat(destination)

		if action.Action == "remove" {
			switch {
			case os.IsNotExist(statErr):
				entry.Effect = EffectNoFilesystemChange
			case statErr != nil:
				return ManifestAnalysis{}, fmt.Errorf("inspect destination %q: %w", action.DestinationPath, workspace.SanitizeFSError(statErr))
			default:
				entry.Effect = EffectWouldRemove
				entry.CurrentDestinationExists = true
				if info.Mode().IsRegular() {
					entry.CurrentDestinationBytes = info.Size()
					analysis.EstimatedFreedBytes += info.Size()
				}
			}
			analysis.Actions = append(analysis.Actions, entry)
			continue
		}

		switch {
		case os.IsNotExist(statErr):
			entry.Effect = EffectCreate
			analysis.EstimatedNewBytes += action.SourceSize
			needsByRoot[action.DestinationRoot] += action.SourceSize
		case statErr != nil:
			return ManifestAnalysis{}, fmt.Errorf("inspect destination %q: %w", action.DestinationPath, workspace.SanitizeFSError(statErr))
		case info.Mode()&os.ModeSymlink != 0:
			entry.Effect = EffectConflict
			entry.Conflict = true
			entry.ConflictReason = "destination is a symlink"
			entry.CurrentDestinationExists = true
			analysis.Conflicts++
			analysis.EstimatedNewBytes += action.SourceSize
			needsByRoot[action.DestinationRoot] += action.SourceSize
		case !info.Mode().IsRegular():
			entry.Effect = EffectConflict
			entry.Conflict = true
			entry.ConflictReason = "destination exists and is not a regular file"
			entry.CurrentDestinationExists = true
			analysis.Conflicts++
			analysis.EstimatedNewBytes += action.SourceSize
			needsByRoot[action.DestinationRoot] += action.SourceSize
		default:
			entry.CurrentDestinationExists = true
			entry.CurrentDestinationBytes = info.Size()
			hash, hashErr := hashFile(destination)
			if hashErr != nil {
				return ManifestAnalysis{}, fmt.Errorf("hash destination %q: %w", action.DestinationPath, workspace.SanitizeFSError(hashErr))
			}
			entry.CurrentDestinationHash = hash
			if action.SourceSHA256 != "" && hash == action.SourceSHA256 {
				entry.Effect = EffectReplaceSameHash
			} else {
				entry.Effect = EffectConflict
				entry.Conflict = true
				entry.ConflictReason = "destination exists with content that differs from the plan"
				analysis.Conflicts++
				analysis.EstimatedBackupBytes += info.Size()
				analysis.EstimatedNewBytes += action.SourceSize
				needsByRoot[action.DestinationRoot] += action.SourceSize
			}
		}

		analysis.Actions = append(analysis.Actions, entry)
	}

	names := make([]string, 0, len(rootPathsUsed))
	for name := range rootPathsUsed {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		space := DestinationSpace{Root: name, Resolvable: true, NeededBytes: needsByRoot[name]}
		if available, ok := diskFreeBytes(rootPathsUsed[name]); ok {
			space.AvailableBytes = available
			space.AvailableBytesKnown = true
			space.Sufficient = available >= space.NeededBytes
		}
		analysis.Destinations = append(analysis.Destinations, space)
	}

	return analysis, nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
