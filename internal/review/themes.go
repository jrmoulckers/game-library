package review

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jrmoulckers/game-library/internal/config"
	"github.com/jrmoulckers/game-library/internal/workspace"
)

// ListSafeThemeIDs enumerates the path-safe theme IDs currently present
// under catalogRoot's canonical "library/themes/<id>/theme.json" layout
// (see docs/architecture/tree.md). It is a read-only directory listing: it
// never creates, modifies, or deletes anything, never reads theme.json's
// content, and never returns a filesystem path — only the bare, already
// path-validated directory name for each theme that actually has a
// theme.json file present.
func ListSafeThemeIDs(catalogRoot string) ([]string, error) {
	if catalogRoot == "" {
		return nil, fmt.Errorf("catalog root is required")
	}
	dir := filepath.Join(catalogRoot, "library", "themes")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read themes directory: %w", workspace.SanitizeFSError(err))
	}
	var ids []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		if !config.IsSafeID(id) {
			continue
		}
		info, statErr := os.Stat(filepath.Join(dir, id, "theme.json"))
		if statErr != nil || info.IsDir() {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}
