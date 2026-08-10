package policy

import (
	"fmt"
	"strings"

	"github.com/jrmoulckers/game-library/internal/model"
)

var validModes = map[string]struct{}{
	"managed":             {},
	"tracked-external":    {},
	"promote-on-approval": {},
	"quarantined":         {},
}

func Validate(file model.PolicyFile) error {
	if file.Version != model.SchemaVersion {
		return fmt.Errorf("policy version must be %d", model.SchemaVersion)
	}
	if _, ok := validModes[file.Default]; !ok {
		return fmt.Errorf("invalid default policy mode %q", file.Default)
	}
	seen := make(map[string]struct{})
	for i, rule := range file.Rules {
		if _, ok := validModes[rule.Mode]; !ok {
			return fmt.Errorf("rule %d has invalid mode %q", i, rule.Mode)
		}
		key := strings.Join([]string{rule.Source, rule.System, rule.Role, rule.AssetSHA256}, "\x00")
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate policy selector at rule %d", i)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func Resolve(file model.PolicyFile, observation model.Observation) string {
	mode, _, _ := ResolveDetailed(file, observation)
	return mode
}

// ResolveDetailed resolves the same policy mode as Resolve, and additionally
// reports which rule won: matchedRule is -1 and matched is false when no
// rule matched and file.Default was used, otherwise matchedRule is the
// index into file.Rules of the most specific matching rule. This lets
// callers (for example a review/preview surface) explain *why* an
// observation resolved to a given mode without duplicating the
// specificity-scoring logic.
func ResolveDetailed(file model.PolicyFile, observation model.Observation) (mode string, matchedRule int, matched bool) {
	mode = file.Default
	best := -1
	matchedRule = -1
	for i, rule := range file.Rules {
		score, matches := specificity(rule, observation)
		if matches && score > best {
			mode = rule.Mode
			best = score
			matchedRule = i
			matched = true
		}
	}
	return mode, matchedRule, matched
}

func specificity(rule model.PolicyRule, observation model.Observation) (int, bool) {
	score := 0
	if rule.Source != "" {
		if rule.Source != observation.RootID {
			return 0, false
		}
		score += 1
	}
	if rule.System != "" {
		if rule.System != observation.System {
			return 0, false
		}
		score += 2
	}
	if rule.Role != "" {
		if rule.Role != observation.Media.Role {
			return 0, false
		}
		score += 4
	}
	if rule.AssetSHA256 != "" {
		if rule.AssetSHA256 != observation.SHA256 {
			return 0, false
		}
		score += 8
	}
	return score, true
}
