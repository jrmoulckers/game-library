package workspace

import (
	"fmt"

	"github.com/jrmoulckers/game-library/internal/config"
	"github.com/jrmoulckers/game-library/internal/manifest"
	"github.com/jrmoulckers/game-library/internal/model"
)

// LoadActiveConfig reads and validates the active host-local configuration.
// It reports found=false with a zero-value config and nil error when the
// file has never been written.
func LoadActiveConfig(path string) (cfg model.Config, found bool, err error) {
	found, err = readJSONIfExists(path, &cfg)
	if err != nil || !found {
		return model.Config{}, found, err
	}
	if err := config.Validate(cfg); err != nil {
		return model.Config{}, true, fmt.Errorf("stored config is invalid: %w", err)
	}
	return cfg, true, nil
}

// WriteActiveConfig validates cfg with the shared config validator, checks
// baseDigest against the digest of whatever active configuration currently
// exists (or "" when absent), and atomically replaces the active host-local
// configuration file. baseDigest must be "" when no configuration exists
// yet. A stale baseDigest returns ErrConflict and leaves the on-disk
// configuration untouched, giving concurrent dashboard writers optimistic
// concurrency instead of a silent last-writer-wins overwrite. This is the
// only mutation the dashboard/CLI perform against a live location, and it
// is always host-local: it never writes to the canonical GamingProfiles
// tree, bundles, generated Decky output, or any other synced/live path.
func WriteActiveConfig(path string, baseDigest string, cfg model.Config) error {
	if err := config.Validate(cfg); err != nil {
		return err
	}
	current, found, err := LoadActiveConfig(path)
	if err != nil {
		return err
	}
	currentDigest := ""
	if found {
		currentDigest, err = manifest.Digest(current)
		if err != nil {
			return fmt.Errorf("digest existing active configuration: %w", err)
		}
	}
	if baseDigest != currentDigest {
		return fmt.Errorf("%w: active config base digest is stale", ErrConflict)
	}
	return atomicWriteJSON(path, cfg)
}
