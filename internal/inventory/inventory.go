// Package inventory scans configured roots read-only and produces private or
// sanitized inventory reports, including duplicate summaries.
package inventory

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jrmoulckers/game-library/internal/media"
	"github.com/jrmoulckers/game-library/internal/model"
)

func Scan(roots []model.Root) (model.Inventory, error) {
	result := model.Inventory{
		Version:     model.SchemaVersion,
		ToolVersion: model.ToolVersion,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		Privacy:     "private",
	}

	for _, root := range roots {
		summary, observations, issues, err := scanRoot(root)
		if err != nil {
			return result, err
		}
		result.Roots = append(result.Roots, summary)
		result.Observations = append(result.Observations, observations...)
		result.Issues = append(result.Issues, issues...)
	}
	sort.Slice(result.Roots, func(i, j int) bool { return result.Roots[i].ID < result.Roots[j].ID })
	sort.Slice(result.Observations, func(i, j int) bool {
		if result.Observations[i].RootID != result.Observations[j].RootID {
			return result.Observations[i].RootID < result.Observations[j].RootID
		}
		return result.Observations[i].RelativePath < result.Observations[j].RelativePath
	})
	result.DuplicateSummary = SummarizeDuplicates(result.Observations)
	return result, nil
}

func scanRoot(root model.Root) (model.RootSummary, []model.Observation, []model.ValidationIssue, error) {
	summary := model.RootSummary{
		ID:         root.ID,
		Kind:       root.Kind,
		System:     root.System,
		Extensions: map[string]int{},
		Roles:      map[string]int{},
		Dimensions: map[string]int{},
	}
	var observations []model.Observation
	var issues []model.ValidationIssue

	info, err := os.Stat(root.Path)
	if err != nil {
		return summary, nil, nil, fmt.Errorf("inspect root %q: %w", root.ID, err)
	}
	if !info.IsDir() {
		return summary, nil, nil, fmt.Errorf("root %q is not a directory", root.ID)
	}

	err = filepath.WalkDir(root.Path, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk root %q: %w", root.ID, walkErr)
		}
		if path == root.Path {
			return nil
		}
		rel, err := filepath.Rel(root.Path, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.Type()&os.ModeSymlink != 0 {
			issues = append(issues, model.ValidationIssue{
				RootID: root.ID, RelativePath: rel, Code: "symlink-rejected",
				Message: "symlinks are not inventoried",
			})
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		fileInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if !fileInfo.Mode().IsRegular() {
			issues = append(issues, model.ValidationIssue{
				RootID: root.ID, RelativePath: rel, Code: "non-regular-rejected",
				Message: "only regular files are inventoried",
			})
			return nil
		}
		digest, err := hashFile(path)
		if err != nil {
			return fmt.Errorf("hash %s:%s: %w", root.ID, rel, err)
		}
		facts, err := media.Inspect(path, root.Kind, rel)
		if err != nil {
			return fmt.Errorf("inspect media %s:%s: %w", root.ID, rel, err)
		}
		system := media.InferSystem(root, rel)
		observation := model.Observation{
			RootID:       root.ID,
			RootKind:     root.Kind,
			RelativePath: rel,
			Size:         fileInfo.Size(),
			SHA256:       digest,
			Media:        facts,
			System:       system,
			IdentityHint: media.InferIdentityHint(root.Kind, rel, system),
		}
		observations = append(observations, observation)
		summary.FileCount++
		summary.TotalBytes += fileInfo.Size()
		if facts.Extension != "" {
			summary.Extensions[facts.Extension]++
		}
		if facts.Role != "" {
			summary.Roles[facts.Role]++
		}
		if key := media.DimensionKey(facts); key != "" {
			summary.Dimensions[key]++
		}
		if strings.HasPrefix(facts.MIME, "image/") {
			summary.ImageCount++
			summary.MediaCount++
		} else if strings.HasPrefix(facts.MIME, "video/") || facts.MIME == "application/pdf" {
			summary.MediaCount++
		}
		if err := media.ValidateRole(facts); err != nil {
			issues = append(issues, model.ValidationIssue{
				RootID: root.ID, RelativePath: rel, Code: "role-media-mismatch",
				Message: err.Error(),
			})
		}
		if err := media.ValidateType(facts); err != nil {
			issues = append(issues, model.ValidationIssue{
				RootID: root.ID, RelativePath: rel, Code: "media-type-mismatch",
				Message: err.Error(),
			})
		}
		return nil
	})
	if err != nil {
		return summary, nil, nil, err
	}
	return summary, observations, issues, nil
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

func SummarizeDuplicates(observations []model.Observation) model.DuplicateSummary {
	groups := groupByHash(observations)
	result := model.DuplicateSummary{UniqueFileHashes: len(groups)}
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		result.Groups++
		result.Copies += len(group)
		result.ExcessBytes += int64(len(group)-1) * group[0].Size
		roots := make(map[string]struct{})
		for _, item := range group {
			roots[item.RootID] = struct{}{}
		}
		if len(roots) > 1 {
			result.CrossRootGroups++
		}
	}
	return result
}

func BuildDuplicateReport(inventory model.Inventory) model.DuplicateReport {
	report := model.DuplicateReport{
		Version:     model.SchemaVersion,
		ToolVersion: model.ToolVersion,
		Summary:     SummarizeDuplicates(inventory.Observations),
	}
	for hash, group := range groupByHash(inventory.Observations) {
		if len(group) < 2 {
			continue
		}
		item := model.DuplicateGroup{SHA256: hash, Size: group[0].Size}
		for _, observation := range group {
			item.Occurrences = append(item.Occurrences, model.DuplicateLocation{
				RootID: observation.RootID, RelativePath: observation.RelativePath,
			})
		}
		sort.Slice(item.Occurrences, func(i, j int) bool {
			if item.Occurrences[i].RootID != item.Occurrences[j].RootID {
				return item.Occurrences[i].RootID < item.Occurrences[j].RootID
			}
			return item.Occurrences[i].RelativePath < item.Occurrences[j].RelativePath
		})
		report.Groups = append(report.Groups, item)
	}
	sort.Slice(report.Groups, func(i, j int) bool { return report.Groups[i].SHA256 < report.Groups[j].SHA256 })
	return report
}

func groupByHash(observations []model.Observation) map[string][]model.Observation {
	groups := make(map[string][]model.Observation)
	for _, observation := range observations {
		groups[observation.SHA256] = append(groups[observation.SHA256], observation)
	}
	return groups
}

func Sanitize(input model.Inventory) model.Inventory {
	input.Privacy = "sanitized"
	input.Observations = nil
	for i := range input.Issues {
		input.Issues[i].RelativePath = ""
	}
	return input
}
