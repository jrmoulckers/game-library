package review

import (
	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/policy"
)

// PolicyImpactEntry explains how a single observation resolves against a
// policy file: its resolved mode and, when a rule (rather than the
// default) decided it, which rule won.
type PolicyImpactEntry struct {
	ID               string `json:"id"`
	RootID           string `json:"rootId"`
	RelativePath     string `json:"relativePath"`
	Mode             string `json:"mode"`
	MatchedRule      bool   `json:"matchedRule"`
	MatchedRuleIndex int    `json:"matchedRuleIndex"`
}

// PolicyImpactView is a full policy preview across a snapshot: a
// per-observation breakdown plus an aggregate count per resolved mode, so a
// reviewer can see the impact of an edited draft policy before it is ever
// promoted anywhere.
type PolicyImpactView struct {
	Counts  map[string]int      `json:"counts"`
	Entries []PolicyImpactEntry `json:"entries"`
}

// BuildPolicyImpact validates file with policy.Validate and resolves it
// against every observation in snapshot using policy.ResolveDetailed,
// reusing the exact precedence rules internal/policy already implements.
func BuildPolicyImpact(snapshot Snapshot, file model.PolicyFile) (PolicyImpactView, error) {
	if err := policy.Validate(file); err != nil {
		return PolicyImpactView{}, err
	}
	counts := make(map[string]int)
	entries := make([]PolicyImpactEntry, 0, len(snapshot.Inventory.Observations))
	for _, observation := range snapshot.Inventory.Observations {
		mode, ruleIndex, matched := policy.ResolveDetailed(file, observation)
		counts[mode]++
		entries = append(entries, PolicyImpactEntry{
			ID:               ObservationID(observation.RootID, observation.RelativePath),
			RootID:           observation.RootID,
			RelativePath:     observation.RelativePath,
			Mode:             mode,
			MatchedRule:      matched,
			MatchedRuleIndex: ruleIndex,
		})
	}
	return PolicyImpactView{Counts: counts, Entries: entries}, nil
}
