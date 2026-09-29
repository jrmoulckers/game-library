package publishing

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/jrmoulckers/game-library/internal/manifest"
	"github.com/jrmoulckers/game-library/internal/model"
)

type File struct {
	Path     string `json:"path"`
	SHA256   string `json:"sha256"`
	Previous string `json:"previous,omitempty"`
	Status   string `json:"status"`
	Blocked  bool   `json:"blocked"`
	Reason   string `json:"reason,omitempty"`
}

type Counts struct {
	Files     int `json:"files"`
	Matching  int `json:"matching"`
	Missing   int `json:"missing"`
	Different int `json:"different"`
	Unknown   int `json:"unknown"`
}

// Plan is deterministic and binds approval to roots, target identity, adapter,
// profile, exact sources and the observed destination bytes. No timestamps.
type Plan struct {
	Version      int            `json:"version"`
	Profile      string         `json:"profile"`
	Target       string         `json:"target"`
	TargetDigest string         `json:"targetDigest"`
	RootsDigest  string         `json:"rootsDigest"`
	Manifest     model.Manifest `json:"manifest"`
	Files        []File         `json:"files"`
	State        string         `json:"state"`
	Message      string         `json:"message,omitempty"`
	Matching     int            `json:"matching"`
	Missing      int            `json:"missing"`
	Different    int            `json:"different"`
	Unknown      int            `json:"unknown"`
	Blocked      int            `json:"blocked"`
	Publishable  Counts         `json:"publishable"`
	Digest       string         `json:"digest"`
}

func finish(plan Plan) (Plan, error) {
	plan.Digest = ""
	digest, err := manifest.Digest(plan)
	plan.Digest = digest
	return plan, err
}

func Build(source Source, target Target) (Plan, error) {
	if !strings.HasPrefix(source.Key, target.Platform+"/") {
		return Plan{}, fmt.Errorf("profile and target platforms must match")
	}
	targetDigest, err := manifest.Digest(target)
	if err != nil {
		return Plan{}, err
	}
	rootsDigest, err := manifest.Digest(source.Roots)
	if err != nil {
		return Plan{}, err
	}
	result := Plan{Version: 1, Profile: source.Key, Target: target.ID,
		TargetDigest: targetDigest, RootsDigest: rootsDigest, Manifest: source.Plan,
		Files: []File{}, State: "observed"}
	if target.Adapter != "steam" || target.Platform != "steam" {
		result.State, result.Message = "unsupported", "No local publishing adapter is enabled for this frontend."
	} else if target.Root == "" {
		result.State, result.Message = "unobserved", "No actual frontend folder has been configured on this host."
	} else if info, err := os.Stat(target.Root); err != nil || !info.IsDir() {
		result.State, result.Message = "offline", "Configured frontend folder is unavailable on this host."
	}
	for _, action := range source.Plan.Actions {
		if (action.Action != "copy" && action.Action != "blocked") || action.DestinationRoot != "steam" {
			return Plan{}, fmt.Errorf("only adapter copy/blocked actions can be previewed")
		}
		name, err := safePath(source.Roots[action.SourceRoot], action.SourcePath, false)
		if err != nil {
			return Plan{}, fmt.Errorf("source containment failed: %w", err)
		}
		hash, err := hashFile(name)
		if err != nil || hash != action.SourceSHA256 {
			return Plan{}, fmt.Errorf("source hash unavailable or changed")
		}
		file := File{Path: action.DestinationPath, SHA256: hash, Status: "unknown"}
		if action.Action == "blocked" {
			file.Blocked, file.Reason = true, action.Reason
			result.Blocked++
		}
		if result.State == "observed" {
			file.Previous, err = observedHash(target.Root, file.Path)
			if err != nil {
				result.Message = "Some target files cannot be safely observed; publishing is disabled."
			} else {
				switch file.Previous {
				case "":
					file.Status = "missing"
				case hash:
					file.Status = "matching"
				default:
					file.Status = "different"
				}
			}
		}
		switch file.Status {
		case "matching":
			result.Matching++
		case "missing":
			result.Missing++
		case "different":
			result.Different++
		default:
			result.Unknown++
		}
		result.Files = append(result.Files, file)
		if !file.Blocked {
			result.Publishable.Files++
			switch file.Status {
			case "matching":
				result.Publishable.Matching++
			case "missing":
				result.Publishable.Missing++
			case "different":
				result.Publishable.Different++
			default:
				result.Publishable.Unknown++
			}
		}
	}
	if len(result.Files) == 0 {
		return Plan{}, fmt.Errorf("no files to export")
	}
	sort.Slice(result.Files, func(i, j int) bool { return result.Files[i].Path < result.Files[j].Path })
	if err := collisionCheck("", result.Files); err != nil {
		return Plan{}, err
	}
	if result.State == "observed" {
		if err := collisionCheck(target.Root, result.Files); err != nil {
			return Plan{}, err
		}
		if result.Unknown != 0 {
			result.State = "unknown"
		}
	}
	return finish(result)
}
