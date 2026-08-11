// Package workspace locates and writes the host-local files the dashboard
// server is allowed to mutate: the active configuration, policy/profile
// drafts, and immutable local artifacts. It never touches the canonical
// GamingProfiles tree, bundles, generated Decky output, the Playnite
// database, or any homelab path; it only reuses the existing config,
// policy, profile, and manifest validators from this repository.
package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Paths describes the host-local directories and files the dashboard is
// allowed to read and write.
type Paths struct {
	// Root is the workspace root directory, platform-specific by default.
	Root string
	// Config is the active host-local configuration file.
	Config string
	// Drafts is the directory holding policy/profile local draft envelopes.
	Drafts string
	// Artifacts is the directory holding create-if-absent immutable JSON
	// records (for example future plan/gate-review evidence).
	Artifacts string
	// Topology is the owner's description of their devices, platforms and
	// named profiles. It stays here rather than in the synced catalog so
	// nothing gamelib invents is replicated to the Deck.
	Topology string
}

// DefaultRoot returns the platform-local default workspace directory. It
// never creates the directory; callers that need it to exist should create
// it with restrictive permissions when writing.
func DefaultRoot() (string, error) {
	if runtime.GOOS == "windows" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = os.Getenv("APPDATA")
		}
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("resolve windows home directory: %w", err)
			}
			base = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(base, "gamelib"), nil
	}

	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "gamelib"), nil
}

// NewPaths derives the standard workspace layout from a root directory.
func NewPaths(root string) Paths {
	return Paths{
		Root:      root,
		Config:    filepath.Join(root, "config", "active.json"),
		Drafts:    filepath.Join(root, "drafts"),
		Artifacts: filepath.Join(root, "artifacts"),
		Topology:  filepath.Join(root, "config", "topology.json"),
	}
}

// Default resolves the platform-local default workspace paths.
func Default() (Paths, error) {
	root, err := DefaultRoot()
	if err != nil {
		return Paths{}, err
	}
	return NewPaths(root), nil
}
