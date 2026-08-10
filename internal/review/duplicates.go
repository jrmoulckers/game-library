package review

import (
	"sort"

	"github.com/jrmoulckers/game-library/internal/model"
)

// DuplicateClassification explains what a reviewer should do about an
// exact-content-hash duplicate group.
type DuplicateClassification string

const (
	// ClassCanonicalOpportunity marks a group with no generated/export
	// copy involved at all: every occurrence is an independent source
	// copy, so designating one as the canonical managed copy would remove
	// real duplication.
	ClassCanonicalOpportunity DuplicateClassification = "canonical-opportunity"
	// ClassExpectedCopy marks a group that spans a source root and a
	// generated/export output root (for example a Decky catalog tree):
	// the duplication is the expected result of exporting a source asset,
	// not something to dedupe away.
	ClassExpectedCopy DuplicateClassification = "expected-generated-or-export-copy"
	// ClassReview marks a group this package cannot confidently classify
	// either way (for example the same content appearing in more than one
	// generated/export output), so a human should look at it.
	ClassReview DuplicateClassification = "review"
)

// generatedOutputKinds lists the model.Root.Kind values this package
// treats as generated/export output rather than an independent source of
// truth. It is intentionally small and explicit rather than pattern-based,
// so classification stays deterministic and auditable; extend it as new
// generated-output root kinds are introduced.
var generatedOutputKinds = map[string]struct{}{
	"decky-catalog": {},
}

func isGeneratedOutputKind(kind string) bool {
	_, ok := generatedOutputKinds[kind]
	return ok
}

// DuplicateOccurrenceView is one occurrence within a duplicate group,
// annotated with its server-derived observation ID and root kind so a
// reviewer can tell a source copy from a generated one without exposing a
// filesystem path.
type DuplicateOccurrenceView struct {
	ID           string `json:"id"`
	RootID       string `json:"rootId"`
	RootKind     string `json:"rootKind"`
	System       string `json:"system,omitempty"`
	RelativePath string `json:"relativePath"`
}

// DuplicateGroupView is one exact-content-hash duplicate group with a
// classification and human-readable reason attached.
type DuplicateGroupView struct {
	SHA256         string                    `json:"sha256"`
	Size           int64                     `json:"size"`
	Occurrences    []DuplicateOccurrenceView `json:"occurrences"`
	Classification DuplicateClassification   `json:"classification"`
	Reason         string                    `json:"reason"`
}

// BuildDuplicateView groups snapshot's private observations by exact
// SHA-256 content hash (reusing the same "size and hash" identity every
// other duplicate/import/manifest computation in this repository uses) and
// classifies each group with Go logic and an explanatory reason. It
// requires snapshot to carry private observations (Inventory.Privacy ==
// "private"); a sanitized snapshot has none and always returns an empty
// slice.
func BuildDuplicateView(snapshot Snapshot) []DuplicateGroupView {
	groups := make(map[string][]model.Observation)
	for _, observation := range snapshot.Inventory.Observations {
		groups[observation.SHA256] = append(groups[observation.SHA256], observation)
	}

	hashes := make([]string, 0, len(groups))
	for hash, group := range groups {
		if len(group) > 1 {
			hashes = append(hashes, hash)
		}
	}
	sort.Strings(hashes)

	views := make([]DuplicateGroupView, 0, len(hashes))
	for _, hash := range hashes {
		group := groups[hash]
		sort.Slice(group, func(i, j int) bool {
			if group[i].RootID != group[j].RootID {
				return group[i].RootID < group[j].RootID
			}
			return group[i].RelativePath < group[j].RelativePath
		})

		occurrences := make([]DuplicateOccurrenceView, 0, len(group))
		hasGenerated, allGenerated := false, true
		for _, observation := range group {
			kind := observation.RootKind
			if isGeneratedOutputKind(kind) {
				hasGenerated = true
			} else {
				allGenerated = false
			}
			occurrences = append(occurrences, DuplicateOccurrenceView{
				ID:           ObservationID(observation.RootID, observation.RelativePath),
				RootID:       observation.RootID,
				RootKind:     kind,
				System:       observation.System,
				RelativePath: observation.RelativePath,
			})
		}

		classification, reason := classifyDuplicateGroup(hasGenerated, allGenerated)
		views = append(views, DuplicateGroupView{
			SHA256:         hash,
			Size:           group[0].Size,
			Occurrences:    occurrences,
			Classification: classification,
			Reason:         reason,
		})
	}
	return views
}

func classifyDuplicateGroup(hasGenerated, allGenerated bool) (DuplicateClassification, string) {
	switch {
	case hasGenerated && !allGenerated:
		return ClassExpectedCopy,
			"one or more occurrences are generated/export output; duplication against their source is expected and is not a candidate for deduplication"
	case hasGenerated && allGenerated:
		return ClassReview,
			"multiple generated/export outputs share identical content; confirm this is expected before treating either as canonical"
	default:
		return ClassCanonicalOpportunity,
			"no generated/export copy is involved; consider designating one of these source copies as the managed canonical asset"
	}
}
