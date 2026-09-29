package publishing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/jrmoulckers/game-library/internal/config"
	"github.com/jrmoulckers/game-library/internal/manifest"
	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/workspace"
)

// Receipt is a durable write-ahead record. Prepared entries may have been
// replaced even if a later copy or journal write failed; rollback inspects bytes.
type Receipt struct {
	Plan           Plan     `json:"plan"`
	Prepared       []string `json:"prepared"`
	State          string   `json:"state"`
	RollbackDigest string   `json:"rollbackDigest,omitempty"`
}

type Engine struct {
	Paths workspace.Paths
	mu    sync.Mutex
}

func (e *Engine) local(relative string) (string, error) {
	return safePath(e.Paths.Root, relative, true)
}

func (e *Engine) lock() (func(), error) {
	name, err := e.local(".publishing.lock")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(e.Paths.Root, 0o700); err != nil {
		return nil, workspace.SanitizeFSError(err)
	}
	return lockFile(name)
}

func (e *Engine) readReceipt(digest string) (Receipt, bool, error) {
	if len(digest) != 64 || !config.IsSafeID(digest) {
		return Receipt{}, false, fmt.Errorf("invalid operation digest")
	}
	name, err := e.local("publishing/" + digest + "/receipt.json")
	if err != nil {
		return Receipt{}, false, err
	}
	data, err := os.ReadFile(name)
	if os.IsNotExist(err) {
		return Receipt{}, false, nil
	}
	if err != nil {
		return Receipt{}, false, workspace.SanitizeFSError(err)
	}
	var result Receipt
	if err := json.Unmarshal(data, &result); err != nil {
		return Receipt{}, false, fmt.Errorf("decode publishing receipt: %w", err)
	}
	verified, err := finish(result.Plan)
	if err != nil || verified.Digest != digest || result.Plan.Digest != digest {
		return Receipt{}, false, fmt.Errorf("publishing receipt integrity mismatch")
	}
	return result, true, nil
}

func (e *Engine) saveReceipt(receipt Receipt) error {
	name, err := e.local("publishing/" + receipt.Plan.Digest + "/receipt.json")
	if err != nil {
		return err
	}
	return workspace.WriteLocalJSON(name, receipt)
}

func (e *Engine) validateRoots(source Source, target Target) error {
	if target.Root != "" && overlaps(target.Root, e.Paths.Root) {
		return fmt.Errorf("frontend target overlaps the local workspace")
	}
	for _, root := range source.Roots {
		if overlaps(root, e.Paths.Root) || (target.Root != "" && overlaps(root, target.Root)) {
			return fmt.Errorf("source, workspace and frontend target must be separate trees")
		}
	}
	return nil
}

func (e *Engine) stage(source Source, plan Plan) (string, error) {
	if plan.Blocked == len(plan.Files) {
		return "", fmt.Errorf("all source files are blocked; no export can be generated")
	}
	for _, action := range source.Plan.Actions {
		if action.Action == "blocked" {
			continue
		}
		sourcePath, err := safePath(source.Roots[action.SourceRoot], action.SourcePath, false)
		if err != nil {
			return "", err
		}
		relative := "exports/" + plan.Digest + "/files/" + action.DestinationPath
		destination, err := e.local(relative)
		if err != nil {
			return "", err
		}
		existing, err := observedHash(e.Paths.Root, relative)
		if err != nil {
			return "", err
		}
		if existing != "" {
			if existing != action.SourceSHA256 {
				return "", fmt.Errorf("staging hash drift; export left untouched")
			}
			continue
		}
		if err := copyAtomic(sourcePath, destination, action.SourceSHA256); err != nil {
			return "", err
		}
	}
	name, err := e.local("exports/" + plan.Digest + "/manifest.json")
	if err != nil {
		return "", err
	}
	exportPaths := e.Paths
	exportPaths.Artifacts = filepath.Dir(name)
	if _, _, err := workspace.WriteArtifact(exportPaths, "manifest.json", plan); err != nil {
		return "", err
	}
	return "exports/" + plan.Digest, nil
}

// Stage is permitted for unavailable devices; it does not claim observation.
// Approval here binds only the reviewed export, not live publication.
func (e *Engine) Stage(source Source, target Target, approved string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	unlock, err := e.lock()
	if err != nil {
		return "", err
	}
	defer unlock()
	if target.Adapter != "steam" || target.Platform != "steam" {
		return "", fmt.Errorf("local export staging only supports Steam frontend targets")
	}
	if err := e.validateRoots(source, target); err != nil {
		return "", err
	}
	plan, err := Build(source, target)
	if err != nil {
		return "", err
	}
	if approved == "" || approved != plan.Digest {
		return "", fmt.Errorf("preview changed; review the exact export again")
	}
	return e.stage(source, plan)
}

// Publish requires an exact preview digest and revalidates the entire file set
// before the first target write, and each destination again before replacement.
// Multi-file Steam publication is per-file atomic, not a directory transaction.
// A partial failure leaves its backups and prepared receipt recoverable.
func (e *Engine) Publish(source Source, target Target, approved string) (Receipt, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	unlock, err := e.lock()
	if err != nil {
		return Receipt{}, err
	}
	defer unlock()
	if err := e.validateRoots(source, target); err != nil {
		return Receipt{}, err
	}
	// An already completed request is idempotent only while its bytes still
	// match. Never turn a drifted repeat into an implicit overwrite.
	old, found, err := e.readReceipt(approved)
	if err != nil {
		return Receipt{}, err
	}
	if found {
		targetDigest, err := manifest.Digest(target)
		if err != nil || old.Plan.TargetDigest != targetDigest || old.Plan.Profile != source.Key || old.State != "complete" {
			return old, fmt.Errorf("operation already recorded; use a fresh preview or rollback")
		}
		current, err := Build(source, target)
		if err != nil || current.State != "observed" || !copiesMatch(current) ||
			current.Manifest.SourceDigest != old.Plan.Manifest.SourceDigest || current.RootsDigest != old.Plan.RootsDigest {
			return old, fmt.Errorf("completed publication drifted; use a fresh preview")
		}
		return old, nil
	}
	plan, err := Build(source, target)
	if err != nil {
		return Receipt{}, err
	}
	if approved == "" || approved != plan.Digest {
		return Receipt{}, fmt.Errorf("preview changed; approval does not match these exact files")
	}
	if plan.State != "observed" || plan.Unknown != 0 {
		return Receipt{}, fmt.Errorf("target must be observed and every file safely hashed before publishing")
	}
	if _, err := e.stage(source, plan); err != nil {
		return Receipt{}, err
	}
	receipt := Receipt{Plan: plan, Prepared: []string{}, State: "in-progress"}
	if err := e.saveReceipt(receipt); err != nil {
		return receipt, err
	}
	for _, file := range plan.Files {
		if file.Blocked || file.Status == "matching" {
			continue
		}
		current, err := observedHash(target.Root, file.Path)
		if err != nil || current != file.Previous {
			return receipt, fmt.Errorf("target changed before copy; publication stopped")
		}
		destination, err := safePath(target.Root, file.Path, true)
		if err != nil {
			return receipt, err
		}
		if file.Previous != "" {
			relative := "publishing/" + plan.Digest + "/backup/" + file.Path
			backup, err := e.local(relative)
			if err != nil {
				return receipt, err
			}
			existing, err := observedHash(e.Paths.Root, relative)
			if err != nil || (existing != "" && existing != file.Previous) {
				return receipt, fmt.Errorf("backup unavailable or drifted; replacement stopped")
			}
			if existing == "" {
				if err := copyAtomic(destination, backup, file.Previous); err != nil {
					return receipt, fmt.Errorf("backup failed; replacement stopped: %w", err)
				}
			}
		}
		receipt.Prepared = append(receipt.Prepared, file.Path)
		if err := e.saveReceipt(receipt); err != nil {
			return receipt, err
		}
		staged, err := e.local("exports/" + plan.Digest + "/files/" + file.Path)
		if err != nil {
			return receipt, err
		}
		current, err = observedHash(target.Root, file.Path)
		if err != nil || current != file.Previous {
			return receipt, fmt.Errorf("target changed after backup; replacement stopped")
		}
		if err := copyAtomic(staged, destination, file.SHA256); err != nil {
			return receipt, err
		}
	}
	receipt.State = "complete"
	return receipt, e.saveReceipt(receipt)
}

func copiesMatch(plan Plan) bool {
	for _, file := range plan.Files {
		if !file.Blocked && file.Status != "matching" {
			return false
		}
	}
	return true
}

type Rollback struct {
	Operation string `json:"operation"`
	Plan      Plan   `json:"plan"`
	Retained  int    `json:"retained"`
}

func (e *Engine) rollbackSource(target Target, operation string) (Source, int, error) {
	receipt, found, err := e.readReceipt(operation)
	if err != nil || !found {
		return Source{}, 0, fmt.Errorf("publishing receipt unavailable")
	}
	digest, err := manifest.Digest(target)
	if err != nil || digest != receipt.Plan.TargetDigest {
		return Source{}, 0, fmt.Errorf("rollback target does not match publication")
	}
	prepared := map[string]bool{}
	for _, name := range receipt.Prepared {
		prepared[name] = true
	}
	var actions []model.Action
	retained := 0
	for _, file := range receipt.Plan.Files {
		if !prepared[file.Path] {
			continue
		}
		actual, err := observedHash(target.Root, file.Path)
		if err != nil {
			return Source{}, 0, err
		}
		if file.Previous == "" {
			if actual != "" {
				retained++
			}
			continue
		}
		if actual != file.SHA256 && actual != file.Previous {
			return Source{}, 0, fmt.Errorf("published file drifted; rollback will not overwrite it")
		}
		actions = append(actions, model.Action{
			Action: "copy", SourceRoot: "backup", SourcePath: file.Path, SourceSHA256: file.Previous,
			DestinationRoot: "steam", DestinationPath: file.Path,
			ExpectedDestination: actual, Reason: "restore retained predecessor; never delete added files",
		})
	}
	plan, err := manifest.NewPlan("steam-rollback-plan", actions, "newly added files are retained; rollback never deletes artwork")
	plan.SourceDigest = operation
	return Source{Key: receipt.Plan.Profile, Plan: plan, Roots: map[string]string{
		"backup": filepath.Join(e.Paths.Root, "publishing", operation, "backup"),
	}}, retained, err
}

func (e *Engine) rollbackPreview(target Target, operation string) (Rollback, Source, error) {
	source, retained, err := e.rollbackSource(target, operation)
	if err != nil {
		return Rollback{}, Source{}, err
	}
	// No replacement was prepared (or additions only): still allow an honest,
	// approved no-delete rollback with an empty restore list.
	var plan Plan
	if len(source.Plan.Actions) == 0 {
		targetDigest, err := manifest.Digest(target)
		if err != nil {
			return Rollback{}, Source{}, err
		}
		plan, err = finish(Plan{Version: 1, Profile: source.Key, Target: target.ID,
			TargetDigest: targetDigest, Manifest: source.Plan, Files: []File{}, State: "observed"})
		if err != nil {
			return Rollback{}, Source{}, err
		}
	} else {
		plan, err = Build(source, target)
		if err != nil {
			return Rollback{}, Source{}, err
		}
	}
	return Rollback{Operation: operation, Plan: plan, Retained: retained}, source, nil
}

func (e *Engine) PreviewRollback(target Target, operation string) (Rollback, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	preview, _, err := e.rollbackPreview(target, operation)
	return preview, err
}

func (e *Engine) Rollback(target Target, operation, approved string) (Receipt, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	unlock, err := e.lock()
	if err != nil {
		return Receipt{}, err
	}
	defer unlock()
	preview, source, err := e.rollbackPreview(target, operation)
	if err != nil {
		return Receipt{}, err
	}
	if approved == "" || approved != preview.Plan.Digest || preview.Plan.State != "observed" {
		return Receipt{}, fmt.Errorf("rollback preview changed; review exact restore files again")
	}
	name, err := e.local("publishing/" + operation + "/rollbacks/" + preview.Plan.Digest + ".json")
	if err != nil {
		return Receipt{}, err
	}
	recordPaths := e.Paths
	recordPaths.Artifacts = filepath.Dir(name)
	if _, _, err := workspace.WriteArtifact(recordPaths, filepath.Base(name), preview); err != nil {
		return Receipt{}, err
	}
	receipt, _, err := e.readReceipt(operation)
	if err != nil {
		return Receipt{}, err
	}
	receipt.State, receipt.RollbackDigest = "restoring", preview.Plan.Digest
	if err := e.saveReceipt(receipt); err != nil {
		return receipt, err
	}
	for _, file := range preview.Plan.Files {
		if file.Status == "matching" {
			continue
		}
		before, err := observedHash(target.Root, file.Path)
		if err != nil || before != file.Previous {
			return Receipt{}, fmt.Errorf("rollback destination changed")
		}
		backup, err := safePath(source.Roots["backup"], file.Path, false)
		if err != nil {
			return Receipt{}, err
		}
		destination, err := safePath(target.Root, file.Path, false)
		if err != nil {
			return Receipt{}, err
		}
		if err := copyAtomic(backup, destination, file.SHA256); err != nil {
			return Receipt{}, err
		}
	}
	receipt.State = "rolled-back"
	return receipt, e.saveReceipt(receipt)
}

func (e *Engine) History() ([]Receipt, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	root, err := e.local("publishing")
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return []Receipt{}, nil
	}
	if err != nil {
		return nil, workspace.SanitizeFSError(err)
	}
	result := []Receipt{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		receipt, found, err := e.readReceipt(entry.Name())
		if err != nil {
			return nil, err
		}
		if found {
			result = append(result, receipt)
		}
	}
	return result, nil
}
