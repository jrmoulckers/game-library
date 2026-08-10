package review

import (
	"github.com/jrmoulckers/game-library/internal/identity"
	"github.com/jrmoulckers/game-library/internal/manifest"
	"github.com/jrmoulckers/game-library/internal/model"
)

// IdentityProposalEntry pairs an identity.Propose result with the
// server-derived observation ID a dashboard client can use to fetch the
// underlying media safely, without ever learning a filesystem path.
type IdentityProposalEntry struct {
	ID       string                 `json:"id"`
	Proposal model.IdentityProposal `json:"proposal"`
}

// UnmappedEntry mirrors IdentityProposalEntry for observations identity.Propose
// could not confidently propose an identity for.
type UnmappedEntry struct {
	ID   string             `json:"id"`
	Item model.UnmappedItem `json:"item"`
}

// IdentityView is the dashboard's identity-proposal review surface: the
// exact output of identity.Propose (this package never re-derives identity
// mapping logic), annotated with observation IDs.
type IdentityView struct {
	Proposals []IdentityProposalEntry `json:"proposals"`
	Unmapped  []UnmappedEntry         `json:"unmapped"`
	// InventoryDigest and IdentityDigest are the content digests of the
	// underlying private inventory and of this exact identity report
	// (manifest.Digest), exposed so a dashboard client can cite both
	// verbatim on a Gate A review (GateAReview.InventoryDigest/
	// IdentityDigest) without ever recomputing a digest itself. Left
	// empty (never a placeholder) if a digest could not be computed.
	InventoryDigest string `json:"inventoryDigest,omitempty"`
	IdentityDigest  string `json:"identityDigest,omitempty"`
}

// BuildIdentityView runs identity.Propose over snapshot's inventory and
// attaches each result's observation ID.
func BuildIdentityView(snapshot Snapshot) IdentityView {
	report := identity.Propose(snapshot.Inventory)
	view := IdentityView{
		Proposals: make([]IdentityProposalEntry, 0, len(report.Proposals)),
		Unmapped:  make([]UnmappedEntry, 0, len(report.Unmapped)),
	}
	for _, proposal := range report.Proposals {
		view.Proposals = append(view.Proposals, IdentityProposalEntry{
			ID:       ObservationID(proposal.RootID, proposal.RelativePath),
			Proposal: proposal,
		})
	}
	for _, item := range report.Unmapped {
		view.Unmapped = append(view.Unmapped, UnmappedEntry{
			ID:   ObservationID(item.RootID, item.RelativePath),
			Item: item,
		})
	}
	view.InventoryDigest, _ = manifest.Digest(snapshot.Inventory)
	view.IdentityDigest, _ = manifest.Digest(report)
	return view
}
