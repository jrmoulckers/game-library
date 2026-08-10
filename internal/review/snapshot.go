package review

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jrmoulckers/game-library/internal/inventory"
	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/report"
)

// Clock allows tests to control "now" when computing scan age. It defaults
// to time.Now.
var Clock = time.Now

// SnapshotSource records where a Snapshot's inventory came from, so an
// overview can explain to the reviewer whether they are looking at a
// previously captured private report or a fresh scan of the active
// configuration's roots.
type SnapshotSource string

const (
	SourceReport SnapshotSource = "report"
	SourceScan   SnapshotSource = "scan"
)

// Snapshot is the in-memory inventory state the review domain operates
// over. It is never persisted by this package: it is either read from an
// existing private report file the operator already produced with
// `gamelib inventory scan`, or produced by scanning the active
// configuration's roots directly, each time the dashboard needs it.
type Snapshot struct {
	Inventory model.Inventory
	Source    SnapshotSource
	ScannedAt time.Time
	Roots     map[string]model.Root

	// index maps a server-derived observation ID (see ObservationID) to
	// the observation it identifies, so the rest of this package (and the
	// dashboard's media handler) never has to search linearly and never
	// has to accept or expose a raw filesystem path.
	index map[string]model.Observation
}

// LoadSnapshot builds a Snapshot for cfg. When reportPath is non-empty, the
// inventory is read from that existing private report file (its recorded
// CreatedAt becomes ScannedAt); otherwise the configured roots are scanned
// fresh (Clock() becomes ScannedAt). Loading from a report never touches
// the filesystem roots themselves, so it is always available even when the
// configured source roots are offline, unmounted, or slow.
func LoadSnapshot(cfg model.Config, reportPath string) (Snapshot, error) {
	var inv model.Inventory
	var scannedAt time.Time
	var source SnapshotSource

	if reportPath != "" {
		if err := report.ReadJSON(reportPath, &inv); err != nil {
			return Snapshot{}, fmt.Errorf("load private inventory report: %w", err)
		}

		source = SourceReport
		if parsed, err := time.Parse(time.RFC3339, inv.CreatedAt); err == nil {
			scannedAt = parsed
		} else {
			scannedAt = Clock().UTC()
		}
	} else {
		scanned, err := inventory.Scan(cfg.Roots)
		if err != nil {
			return Snapshot{}, fmt.Errorf("scan configured roots: %w", err)
		}
		inv = scanned
		source = SourceScan
		scannedAt = Clock().UTC()
	}

	roots := make(map[string]model.Root, len(cfg.Roots))
	for _, root := range cfg.Roots {
		roots[root.ID] = root
	}

	index := make(map[string]model.Observation, len(inv.Observations))
	for _, observation := range inv.Observations {
		index[ObservationID(observation.RootID, observation.RelativePath)] = observation
	}

	return Snapshot{
		Inventory: inv,
		Source:    source,
		ScannedAt: scannedAt,
		Roots:     roots,
		index:     index,
	}, nil
}

// NewSnapshot builds the same opaque media index as LoadSnapshot for an
// inventory assembled incrementally by the local dashboard.
func NewSnapshot(cfg model.Config, inv model.Inventory, source SnapshotSource, scannedAt time.Time) Snapshot {
	roots := make(map[string]model.Root, len(cfg.Roots))
	for _, root := range cfg.Roots {
		roots[root.ID] = root
	}
	index := make(map[string]model.Observation, len(inv.Observations))
	for _, observation := range inv.Observations {
		index[ObservationID(observation.RootID, observation.RelativePath)] = observation
	}
	return Snapshot{Inventory: inv, Source: source, ScannedAt: scannedAt, Roots: roots, index: index}
}

// ObservationID derives a stable, opaque identifier for an observation from
// its root id and relative path. It is a one-way SHA-256 digest: it never
// contains and cannot be reversed into a filesystem path, so it is safe to
// hand to a browser (for example as a media URL segment) even though the
// dashboard only trusts the local OS user, matching ADR-0007's requirement
// that media is "addressed by a server-derived observation ID, never a raw
// filesystem path".
func ObservationID(rootID, relativePath string) string {
	sum := sha256.Sum256([]byte(rootID + "\x00" + relativePath))
	return hex.EncodeToString(sum[:])
}

// FindObservation looks up an observation by its server-derived ID. It
// reports found=false for any ID that does not correspond to a known
// observation in this snapshot (including a syntactically valid ID whose
// underlying root/path no longer appears in the current snapshot).
func (s Snapshot) FindObservation(id string) (model.Observation, bool) {
	observation, ok := s.index[id]
	return observation, ok
}

// ScanAge reports how long ago the snapshot was captured, relative to now.
// It never returns a negative duration.
func (s Snapshot) ScanAge(now time.Time) time.Duration {
	if s.ScannedAt.IsZero() {
		return 0
	}
	age := now.Sub(s.ScannedAt)
	if age < 0 {
		return 0
	}
	return age
}
