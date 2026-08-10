package review

import (
	"sort"
	"strings"

	"github.com/jrmoulckers/game-library/internal/media"
	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/policy"
)

// ObservationFilter narrows the observations ListObservations returns.
// Every field is optional (empty means "no filter on this dimension"), and
// filters combine with AND semantics.
type ObservationFilter struct {
	// Source matches Observation.RootID exactly.
	Source string
	// System matches Observation.System exactly.
	System string
	// Identity matches Observation.IdentityHint exactly, or as a
	// "prefix:" match (for example "steam" matches "steam:123").
	Identity string
	// Role matches Observation.Media.Role exactly.
	Role string
	// Dimensions matches the "WxH" dimension key (see media.DimensionKey)
	// exactly.
	Dimensions string
	// PolicyOutcome matches the resolved policy mode exactly (requires a
	// policy file be supplied to ListObservations; ignored otherwise).
	PolicyOutcome string
	// Theme matches profile-theme membership: the observation's SHA256
	// must be referenced by at least one supplied profile whose Theme
	// equals this value.
	Theme string
	// Validation is one of "", "issues", or "clean". "issues" keeps only
	// observations with at least one matching model.ValidationIssue;
	// "clean" keeps only observations with none.
	Validation string
}

// ObservationEntry is one row in a reviewed observation listing: the raw
// observation, its server-derived (never-a-path) ID, its resolved policy
// outcome (if a policy file was supplied), the profile themes that
// reference its content hash, and any validation issues recorded against
// it.
type ObservationEntry struct {
	ID                string                  `json:"id"`
	Observation       model.Observation       `json:"observation"`
	PolicyMode        string                  `json:"policyMode,omitempty"`
	PolicyRuleMatched bool                    `json:"policyRuleMatched"`
	PolicyRuleIndex   int                     `json:"policyRuleIndex"`
	Themes            []string                `json:"themes,omitempty"`
	ValidationIssues  []model.ValidationIssue `json:"validationIssues,omitempty"`
}

// ObservationPage is a paginated, filtered observation listing.
type ObservationPage struct {
	Total    int                `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"pageSize"`
	Items    []ObservationEntry `json:"items"`
}

// ListObservations filters, sorts (by root then relative path, matching
// internal/inventory.Scan's own deterministic order), and paginates the
// observations in snapshot. policyFile (optional, pass a zero value to skip
// policy resolution) is resolved per-observation with
// policy.ResolveDetailed. profiles (optional) supplies the profile drafts
// used to compute Theme membership. page is 1-indexed; a page or pageSize
// of 0 or less defaults to page 1 / pageSize 50.
func ListObservations(snapshot Snapshot, policyFile model.PolicyFile, profiles []model.Profile, filter ObservationFilter, page, pageSize int) ObservationPage {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}

	issuesByKey := make(map[string][]model.ValidationIssue, len(snapshot.Inventory.Issues))
	for _, issue := range snapshot.Inventory.Issues {
		key := issue.RootID + "\x00" + issue.RelativePath
		issuesByKey[key] = append(issuesByKey[key], issue)
	}

	themesByHash := themeIndex(profiles)

	hasPolicy := policyFile.Version != 0 || len(policyFile.Rules) > 0 || policyFile.Default != ""

	entries := make([]ObservationEntry, 0, len(snapshot.Inventory.Observations))
	for _, observation := range snapshot.Inventory.Observations {
		entry := ObservationEntry{
			ID:          ObservationID(observation.RootID, observation.RelativePath),
			Observation: observation,
		}
		if hasPolicy {
			mode, ruleIndex, matched := policy.ResolveDetailed(policyFile, observation)
			entry.PolicyMode = mode
			entry.PolicyRuleIndex = ruleIndex
			entry.PolicyRuleMatched = matched
		}
		entry.Themes = themesByHash[observation.SHA256]
		entry.ValidationIssues = issuesByKey[observation.RootID+"\x00"+observation.RelativePath]

		if !matchesFilter(entry, observation, filter, hasPolicy) {
			continue
		}
		entries = append(entries, entry)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i].Observation, entries[j].Observation
		if a.RootID != b.RootID {
			return a.RootID < b.RootID
		}
		return a.RelativePath < b.RelativePath
	})

	total := len(entries)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	return ObservationPage{
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Items:    entries[start:end],
	}
}

func matchesFilter(entry ObservationEntry, observation model.Observation, filter ObservationFilter, hasPolicy bool) bool {
	if filter.Source != "" && observation.RootID != filter.Source {
		return false
	}
	if filter.System != "" && observation.System != filter.System {
		return false
	}
	if filter.Identity != "" {
		hint := observation.IdentityHint
		if hint != filter.Identity && !strings.HasPrefix(hint, filter.Identity+":") {
			return false
		}
	}
	if filter.Role != "" && observation.Media.Role != filter.Role {
		return false
	}
	if filter.Dimensions != "" && media.DimensionKey(observation.Media) != filter.Dimensions {
		return false
	}
	if filter.PolicyOutcome != "" {
		if !hasPolicy || entry.PolicyMode != filter.PolicyOutcome {
			return false
		}
	}
	if filter.Theme != "" {
		found := false
		for _, theme := range entry.Themes {
			if theme == filter.Theme {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	switch filter.Validation {
	case "issues":
		if len(entry.ValidationIssues) == 0 {
			return false
		}
	case "clean":
		if len(entry.ValidationIssues) != 0 {
			return false
		}
	}
	return true
}

// themeIndex maps an asset SHA-256 to the sorted, de-duplicated set of
// profile themes that reference it, across every supplied profile draft.
// A profile with no Theme set does not contribute to the index: an empty
// theme cannot be filtered on.
func themeIndex(profiles []model.Profile) map[string][]string {
	seen := make(map[string]map[string]struct{})
	for _, p := range profiles {
		if p.Theme == "" {
			continue
		}
		for _, game := range p.Games {
			for _, asset := range game.Assets {
				if seen[asset.SHA256] == nil {
					seen[asset.SHA256] = make(map[string]struct{})
				}
				seen[asset.SHA256][p.Theme] = struct{}{}
			}
		}
	}
	result := make(map[string][]string, len(seen))
	for hash, themes := range seen {
		list := make([]string, 0, len(themes))
		for theme := range themes {
			list = append(list, theme)
		}
		sort.Strings(list)
		result[hash] = list
	}
	return result
}
