package review

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/jrmoulckers/game-library/internal/manifest"
	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/workspace"
)

// HistoryEntry describes one immutable local artifact this workspace has
// persisted — a plan or a gate review — addressed only by its symbolic
// type/id/kind and a content digest, never by a filesystem path.
type HistoryEntry struct {
	// Type is "plan", "gate-a", "gate-b", or "gate-c".
	Type string `json:"type"`
	ID   string `json:"id"`
	// Kind is only set for Type == "plan" (e.g. "import-plan",
	// "steam-export-plan").
	Kind string `json:"kind,omitempty"`
	// Digest is the SHA-256 of the artifact's own persisted JSON bytes: a
	// content-addressed integrity marker, never a path.
	Digest    string `json:"digest"`
	CreatedAt string `json:"createdAt,omitempty"`
	// Verified is true when the artifact's content still hashes to the id
	// it is filed under (see verifyPriorGate / manifest.OperationIDFor):
	// false means the on-disk content no longer matches its own claimed
	// identity, which should never happen for anything this workspace
	// itself wrote and is surfaced here rather than hidden.
	Verified bool `json:"verified"`
}

// forbiddenHistoryNames guards, defensively, against ever surfacing a
// forward-looking applied/rollback record as if it were real, executed
// history. This repository has no apply/rollback capability: these
// filenames are reserved for a future canonical-tree feature (see
// docs/architecture/tree.md's state/migration/<operation_id>/{applied,
// rolled-back}.json, which lives outside this workspace entirely and this
// package never reads) and must never be listed here even if one somehow
// appeared under the local artifacts directory.
var forbiddenHistoryNames = map[string]bool{
	"applied.json":     true,
	"rolled-back.json": true,
}

// ListHistory enumerates every immutable plan and gate-review artifact
// persisted under paths.Artifacts, by symbolic reference only. It never
// treats anything outside the specific "plans/**" and "gate-reviews/{a,b,c}"
// subdirectories as history, and never returns a filesystem path.
func ListHistory(paths workspace.Paths) ([]HistoryEntry, error) {
	var entries []HistoryEntry

	planEntries, err := listPlanHistory(paths)
	if err != nil {
		return nil, err
	}
	entries = append(entries, planEntries...)

	for _, letter := range []string{"a", "b", "c"} {
		gateEntries, err := listGateHistory(paths, letter)
		if err != nil {
			return nil, err
		}
		entries = append(entries, gateEntries...)
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Type != entries[j].Type {
			return entries[i].Type < entries[j].Type
		}
		return entries[i].ID < entries[j].ID
	})
	return entries, nil
}

func listPlanHistory(paths workspace.Paths) ([]HistoryEntry, error) {
	root := filepath.Join(paths.Artifacts, "plans")
	var entries []HistoryEntry
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return workspace.SanitizeFSError(err)
		}
		if d.IsDir() {
			return nil
		}
		name := filepath.Base(path)
		if forbiddenHistoryNames[name] || filepath.Ext(name) != ".json" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return workspace.SanitizeFSError(readErr)
		}
		var plan model.Manifest
		if !decodeArtifact(data, &plan) {
			// Not a plan artifact this package understands; skip rather
			// than fail the whole listing.
			return nil
		}
		expectedID, idErr := manifest.OperationIDFor(plan.Kind, plan.Actions)
		verified := idErr == nil && expectedID == plan.OperationID
		entries = append(entries, HistoryEntry{
			Type:     "plan",
			ID:       plan.OperationID,
			Kind:     plan.Kind,
			Digest:   sha256Hex(data),
			Verified: verified,
		})
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("list plan history: %w", err)
	}
	return entries, nil
}

func listGateHistory(paths workspace.Paths, letter string) ([]HistoryEntry, error) {
	root := filepath.Join(paths.Artifacts, "gate-reviews", letter)
	var entries []HistoryEntry
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return workspace.SanitizeFSError(err)
		}
		if d.IsDir() {
			return nil
		}
		name := filepath.Base(path)
		if forbiddenHistoryNames[name] || filepath.Ext(name) != ".json" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return workspace.SanitizeFSError(readErr)
		}

		var id, createdAt string
		var verified bool
		switch letter {
		case "a":
			var stored GateAReview
			if !decodeArtifact(data, &stored) {
				return nil
			}
			id, createdAt = stored.ID, stored.CreatedAt
			clean := stored
			clean.ID = ""
			expected, idErr := gateID(clean)
			verified = idErr == nil && expected == stored.ID
		case "b":
			var stored GateBReview
			if !decodeArtifact(data, &stored) {
				return nil
			}
			id, createdAt = stored.ID, stored.CreatedAt
			clean := stored
			clean.ID = ""
			expected, idErr := gateID(clean)
			verified = idErr == nil && expected == stored.ID
		case "c":
			var stored GateCReview
			if !decodeArtifact(data, &stored) {
				return nil
			}
			id, createdAt = stored.ID, stored.CreatedAt
			clean := stored
			clean.ID = ""
			expected, idErr := gateID(clean)
			verified = idErr == nil && expected == stored.ID
		default:
			return nil
		}

		entries = append(entries, HistoryEntry{
			Type:      "gate-" + letter,
			ID:        id,
			Digest:    sha256Hex(data),
			CreatedAt: createdAt,
			Verified:  verified,
		})
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("list gate-%s history: %w", letter, err)
	}
	return entries, nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
