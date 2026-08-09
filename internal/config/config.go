package config

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/jrmoulckers/game-library/internal/model"
)

var safeID = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

func Load(path string) (model.Config, error) {
	var cfg model.Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("decode config: %w", err)
	}
	if err := Validate(cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func Validate(cfg model.Config) error {
	if cfg.Version != model.SchemaVersion {
		return fmt.Errorf("config version must be %d", model.SchemaVersion)
	}
	if len(cfg.Roots) == 0 {
		return fmt.Errorf("config must define at least one root")
	}
	seen := make(map[string]struct{}, len(cfg.Roots))
	for _, root := range cfg.Roots {
		if !safeID.MatchString(root.ID) {
			return fmt.Errorf("root id %q is not path-safe", root.ID)
		}
		if root.Kind == "" {
			return fmt.Errorf("root %q kind is required", root.ID)
		}
		if root.Path == "" {
			return fmt.Errorf("root %q path is required", root.ID)
		}
		if _, ok := seen[root.ID]; ok {
			return fmt.Errorf("duplicate root id %q", root.ID)
		}
		seen[root.ID] = struct{}{}
	}
	if cfg.Policy.Version != model.SchemaVersion {
		return fmt.Errorf("policy version must be %d", model.SchemaVersion)
	}
	return nil
}

func IsSafeID(value string) bool {
	if !safeID.MatchString(value) {
		return false
	}
	upper := strings.ToUpper(strings.TrimSuffix(value, "."))
	switch upper {
	case "CON", "PRN", "AUX", "NUL":
		return false
	}
	for i := 1; i <= 9; i++ {
		suffix := strconv.Itoa(i)
		if upper == "COM"+suffix || upper == "LPT"+suffix {
			return false
		}
	}
	return true
}
