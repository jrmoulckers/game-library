package review

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jrmoulckers/game-library/internal/media"
	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/workspace"
)

// ErrMediaNotFound is returned when an observation ID does not resolve to
// any observation in the snapshot, or its root is no longer configured.
var ErrMediaNotFound = errors.New("review: media not found")

// ErrMediaUnsafe is returned whenever the resolved filesystem target fails
// any containment, symlink, or size safety check. The dashboard maps this
// to a generic 4xx response; it never surfaces the underlying path.
var ErrMediaUnsafe = errors.New("review: media is unsafe to serve")

// MaxMediaServeBytes bounds how large a file this package will ever read
// and serve for a single media request, independent of whatever size was
// recorded at scan time (a file can change on disk between scan and serve).
// It is a var rather than a const solely so tests can shrink it instead of
// generating multi-megabyte fixtures.
var MaxMediaServeBytes int64 = 64 << 20 // 64 MiB

// MediaResolution describes a single observation's file, resolved and
// safety-checked, ready to be streamed by the dashboard's media handler.
// Path is intentionally not JSON-tagged for export to an API response: it
// exists only to open the file, never to hand back to a client. SHA256 is
// the content hash already recorded for this observation at scan time (not
// recomputed here, so serving a large file never requires a second full
// read just to produce a cache validator) and is safe to expose as an
// ETag: it identifies content, not a filesystem location.
type MediaResolution struct {
	Path   string
	MIME   string
	Size   int64
	SHA256 string
}

// ResolveMedia looks up the observation addressed by id (a value produced
// by ObservationID, never a raw filesystem path) and safety-checks the
// underlying file before it is served:
//
//   - containment: the resolved path is joined and validated the same way
//     internal/workspace.Contain validates workspace-relative paths, so a
//     malformed relative path can never escape the configured root;
//   - symlink-at-serve-time: the leaf file, and every directory between
//     the configured root and it, is re-checked for symlinks right now
//     (not just at scan time), defending against a symlink substituted in
//     after the original inventory scan (TOCTOU);
//   - MIME: the content type is sniffed fresh from the file's current
//     bytes using internal/media (the same detector the scanner uses),
//     never trusted from the stored inventory record or file extension;
//   - size: the file must not exceed MaxMediaServeBytes.
func ResolveMedia(snapshot Snapshot, id string) (MediaResolution, error) {
	observation, ok := snapshot.FindObservation(id)
	if !ok {
		return MediaResolution{}, ErrMediaNotFound
	}
	root, ok := snapshot.Roots[observation.RootID]
	if !ok {
		return MediaResolution{}, fmt.Errorf("%w: observation root %q is no longer configured", ErrMediaNotFound, observation.RootID)
	}

	target, err := workspace.Contain(root.Path, observation.RelativePath)
	if err != nil {
		return MediaResolution{}, fmt.Errorf("%w: %w", ErrMediaUnsafe, err)
	}

	info, err := os.Lstat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return MediaResolution{}, ErrMediaNotFound
		}
		return MediaResolution{}, fmt.Errorf("%w: %w", ErrMediaUnsafe, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return MediaResolution{}, fmt.Errorf("%w: target is a symlink", ErrMediaUnsafe)
	}
	if !info.Mode().IsRegular() {
		return MediaResolution{}, fmt.Errorf("%w: target is not a regular file", ErrMediaUnsafe)
	}
	if info.Size() > MaxMediaServeBytes {
		return MediaResolution{}, fmt.Errorf("%w: file exceeds the maximum servable size", ErrMediaUnsafe)
	}

	if err := requireNoSymlinkAncestor(root.Path, target); err != nil {
		return MediaResolution{}, err
	}

	facts, err := media.Inspect(target, root.Kind, observation.RelativePath)
	if err != nil {
		return MediaResolution{}, fmt.Errorf("%w: %w", ErrMediaUnsafe, err)
	}

	return MediaResolution{Path: target, MIME: facts.MIME, Size: info.Size(), SHA256: observation.SHA256}, nil
}

// requireNoSymlinkAncestor defends against a directory between root and
// target being replaced with a symlink after the original scan: it resolves
// both root and target's parent directory through the filesystem's real
// (symlink-free) path and confirms the resolved parent is still contained
// within the resolved root.
func requireNoSymlinkAncestor(root, target string) error {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrMediaUnsafe, err)
	}
	realParent, err := filepath.EvalSymlinks(filepath.Dir(target))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrMediaUnsafe, err)
	}
	rel, err := filepath.Rel(realRoot, realParent)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrMediaUnsafe, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: a directory between the root and the file has been replaced by a symlink", ErrMediaUnsafe)
	}
	return nil
}

// ObservationRootKind is a small helper the dashboard uses to label a media
// response; it never exposes a path.
func ObservationRootKind(snapshot Snapshot, observation model.Observation) string {
	if root, ok := snapshot.Roots[observation.RootID]; ok {
		return root.Kind
	}
	return ""
}
