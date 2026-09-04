// Package identity proposes steam:/playnite:/retro: identity hints for observed
// inventory files. Proposals are advisory and never applied automatically.
package identity

import (
	"sort"
	"strings"

	"github.com/jrmoulckers/game-library/internal/model"
)

func Propose(inventory model.Inventory) model.IdentityReport {
	report := model.IdentityReport{
		Version:     model.SchemaVersion,
		ToolVersion: model.ToolVersion,
	}
	for _, observation := range inventory.Observations {
		if observation.IdentityHint == "" {
			report.Unmapped = append(report.Unmapped, model.UnmappedItem{
				RootID: observation.RootID, RelativePath: observation.RelativePath,
				Reason: "no deterministic identity in source path",
			})
			continue
		}
		proposal := model.IdentityProposal{
			RootID: observation.RootID, RelativePath: observation.RelativePath,
			CanonicalID: observation.IdentityHint, Confidence: "high",
			Reason: "deterministic source convention",
		}
		switch {
		case strings.HasPrefix(observation.IdentityHint, "steam:"):
			proposal.MappingType = "steam-appid"
		case strings.HasPrefix(observation.IdentityHint, "playnite:"):
			proposal.MappingType = "playnite-adapter"
		case strings.HasPrefix(observation.IdentityHint, "retro:"):
			proposal.MappingType = "retro-filename-alias"
			proposal.Confidence = "proposal"
			proposal.Reason = "ROM stem is a mutable alias; content hash is required for release identity"
		}
		report.Proposals = append(report.Proposals, proposal)
	}
	sort.Slice(report.Proposals, func(i, j int) bool {
		if report.Proposals[i].CanonicalID != report.Proposals[j].CanonicalID {
			return report.Proposals[i].CanonicalID < report.Proposals[j].CanonicalID
		}
		if report.Proposals[i].RootID != report.Proposals[j].RootID {
			return report.Proposals[i].RootID < report.Proposals[j].RootID
		}
		return report.Proposals[i].RelativePath < report.Proposals[j].RelativePath
	})
	sort.Slice(report.Unmapped, func(i, j int) bool {
		if report.Unmapped[i].RootID != report.Unmapped[j].RootID {
			return report.Unmapped[i].RootID < report.Unmapped[j].RootID
		}
		return report.Unmapped[i].RelativePath < report.Unmapped[j].RelativePath
	})
	return report
}
