package review

import (
	"time"

	"github.com/jrmoulckers/game-library/internal/manifest"
	"github.com/jrmoulckers/game-library/internal/model"
)

// RootOverview summarizes one configured root for the dashboard's overview
// screen. It is derived entirely from the existing model.RootSummary and
// model.ValidationIssue produced by internal/inventory.
type RootOverview struct {
	RootID     string `json:"rootId"`
	Kind       string `json:"kind"`
	System     string `json:"system,omitempty"`
	FileCount  int    `json:"fileCount"`
	TotalBytes int64  `json:"totalBytes"`
	MediaCount int    `json:"mediaCount"`
	ImageCount int    `json:"imageCount"`
	IssueCount int    `json:"issueCount"`
}

// Overview is the top-level review dashboard summary: how stale the
// snapshot is, one row per configured root, and the full issue list so a
// reviewer can jump straight to what needs attention.
type Overview struct {
	Source           SnapshotSource          `json:"source"`
	Privacy          string                  `json:"privacy"`
	ScannedAt        string                  `json:"scannedAt,omitempty"`
	ScanAgeSeconds   int64                   `json:"scanAgeSeconds"`
	Roots            []RootOverview          `json:"roots"`
	TotalIssues      int                     `json:"totalIssues"`
	Issues           []model.ValidationIssue `json:"issues,omitempty"`
	DuplicateSummary model.DuplicateSummary  `json:"duplicateSummary"`
	// InventoryDigest is the content digest of the underlying private
	// inventory (manifest.Digest(snapshot.Inventory)), exposed so a
	// dashboard client can cite it verbatim on a Gate A review
	// (GateAReview.InventoryDigest) without ever recomputing a digest
	// itself. Left empty (never a zero-value placeholder digest) if it
	// could not be computed.
	InventoryDigest string `json:"inventoryDigest,omitempty"`
}

// BuildOverview computes an Overview for snapshot as of now.
func BuildOverview(snapshot Snapshot, now time.Time) Overview {
	issueCounts := make(map[string]int, len(snapshot.Inventory.Roots))
	for _, issue := range snapshot.Inventory.Issues {
		issueCounts[issue.RootID]++
	}

	roots := make([]RootOverview, 0, len(snapshot.Inventory.Roots))
	for _, summary := range snapshot.Inventory.Roots {
		roots = append(roots, RootOverview{
			RootID:     summary.ID,
			Kind:       summary.Kind,
			System:     summary.System,
			FileCount:  summary.FileCount,
			TotalBytes: summary.TotalBytes,
			MediaCount: summary.MediaCount,
			ImageCount: summary.ImageCount,
			IssueCount: issueCounts[summary.ID],
		})
	}

	scannedAt := ""
	if !snapshot.ScannedAt.IsZero() {
		scannedAt = snapshot.ScannedAt.UTC().Format(time.RFC3339)
	}

	// A digest that cannot be computed is left empty rather than filled with a
	// placeholder, so a client can never cite a value that is not real.
	inventoryDigest := ""
	if digest, err := manifest.Digest(snapshot.Inventory); err == nil {
		inventoryDigest = digest
	}

	return Overview{
		Source:           snapshot.Source,
		Privacy:          snapshot.Inventory.Privacy,
		ScannedAt:        scannedAt,
		ScanAgeSeconds:   int64(snapshot.ScanAge(now).Seconds()),
		Roots:            roots,
		TotalIssues:      len(snapshot.Inventory.Issues),
		Issues:           snapshot.Inventory.Issues,
		DuplicateSummary: snapshot.Inventory.DuplicateSummary,
		InventoryDigest:  inventoryDigest,
	}
}
