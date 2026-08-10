package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jrmoulckers/game-library/internal/config"
	"github.com/jrmoulckers/game-library/internal/manifest"
	"github.com/jrmoulckers/game-library/internal/model"
	"github.com/jrmoulckers/game-library/internal/policy"
	"github.com/jrmoulckers/game-library/internal/profile"
)

// PolicyDraftEnvelope is a validated host-local draft of a policy file. It
// is never the canonical/active policy: promoting a draft to the active
// configuration is an explicit future workflow outside this release.
type PolicyDraftEnvelope struct {
	Version    int              `json:"version"`
	BaseDigest string           `json:"baseDigest"`
	Digest     string           `json:"digest"`
	UpdatedAt  string           `json:"updatedAt"`
	Policy     model.PolicyFile `json:"policy"`
}

// ProfileDraftEnvelope is a validated host-local draft of a canonical
// profile document, keyed by the profile's path-safe id. A draft never
// contains canonical library paths: assets are always addressed by
// SHA-256 content hash, never by filesystem path, matching the canonical
// profile schema this package reuses.
type ProfileDraftEnvelope struct {
	Version    int           `json:"version"`
	BaseDigest string        `json:"baseDigest"`
	Digest     string        `json:"digest"`
	UpdatedAt  string        `json:"updatedAt"`
	Profile    model.Profile `json:"profile"`
}

// Clock allows tests to control the timestamp recorded on drafts. It
// defaults to time.Now.
var Clock = time.Now

func policyDraftPath(paths Paths) string {
	return filepath.Join(paths.Drafts, "policy.json")
}

// ProfileDraftPath returns the draft path for a profile id. The id is
// validated with the same path-safety rule as the rest of the repository
// (config.IsSafeID) and additionally passed through Contain so a malicious
// id can never escape the drafts directory.
func ProfileDraftPath(paths Paths, profileID string) (string, error) {
	if !config.IsSafeID(profileID) {
		return "", fmt.Errorf("profile id %q is not path-safe", profileID)
	}
	return Contain(paths.Drafts, "profile-"+profileID+".json")
}

// LoadPolicyDraft returns the current on-disk policy draft envelope, if
// any.
func LoadPolicyDraft(paths Paths) (PolicyDraftEnvelope, bool, error) {
	var envelope PolicyDraftEnvelope
	found, err := readJSONIfExists(policyDraftPath(paths), &envelope)
	if err != nil {
		return PolicyDraftEnvelope{}, false, err
	}
	return envelope, found, nil
}

// SavePolicyDraft validates file, checks baseDigest against the digest of
// whatever draft (or absence of a draft) currently exists, and atomically
// writes the new draft envelope. baseDigest must be "" when no draft
// exists yet. A stale baseDigest returns ErrConflict and leaves the
// on-disk draft untouched.
func SavePolicyDraft(paths Paths, baseDigest string, file model.PolicyFile) (PolicyDraftEnvelope, error) {
	if err := policy.Validate(file); err != nil {
		return PolicyDraftEnvelope{}, err
	}
	current, found, err := LoadPolicyDraft(paths)
	if err != nil {
		return PolicyDraftEnvelope{}, err
	}
	currentDigest := ""
	if found {
		currentDigest = current.Digest
	}
	if baseDigest != currentDigest {
		return PolicyDraftEnvelope{}, fmt.Errorf("%w: policy draft base digest is stale", ErrConflict)
	}
	digest, err := manifest.Digest(file)
	if err != nil {
		return PolicyDraftEnvelope{}, fmt.Errorf("digest policy draft: %w", err)
	}
	envelope := PolicyDraftEnvelope{
		Version:    model.SchemaVersion,
		BaseDigest: baseDigest,
		Digest:     digest,
		UpdatedAt:  Clock().UTC().Format(time.RFC3339),
		Policy:     file,
	}
	if err := atomicWriteJSON(policyDraftPath(paths), envelope); err != nil {
		return PolicyDraftEnvelope{}, err
	}
	return envelope, nil
}

// LoadProfileDraft returns the current on-disk profile draft envelope for
// profileID, if any.
func LoadProfileDraft(paths Paths, profileID string) (ProfileDraftEnvelope, bool, error) {
	path, err := ProfileDraftPath(paths, profileID)
	if err != nil {
		return ProfileDraftEnvelope{}, false, err
	}
	var envelope ProfileDraftEnvelope
	found, err := readJSONIfExists(path, &envelope)
	if err != nil {
		return ProfileDraftEnvelope{}, false, err
	}
	return envelope, found, nil
}

// ListProfileDraftIDs returns the path-safe profile ids that currently have
// a saved draft under paths.Drafts, sorted for deterministic output. It
// reports an empty slice (not an error) when the drafts directory does not
// exist yet, matching the "not found" semantics the rest of this package
// uses for absent state.
func ListProfileDraftIDs(paths Paths) ([]string, error) {
	entries, err := os.ReadDir(paths.Drafts)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read drafts directory: %w", SanitizeFSError(err))
	}
	var ids []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "profile-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(name, "profile-"), ".json")
		if !config.IsSafeID(id) {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

// SaveProfileDraft validates value, checks baseDigest against the digest of
// whatever draft currently exists for value.ID (or "" when absent), and
// atomically writes the new draft envelope. A stale baseDigest returns
// ErrConflict and leaves the on-disk draft untouched.
func SaveProfileDraft(paths Paths, baseDigest string, value model.Profile) (ProfileDraftEnvelope, error) {
	if err := profile.Validate(value); err != nil {
		return ProfileDraftEnvelope{}, err
	}
	path, err := ProfileDraftPath(paths, value.ID)
	if err != nil {
		return ProfileDraftEnvelope{}, err
	}
	var current ProfileDraftEnvelope
	found, err := readJSONIfExists(path, &current)
	if err != nil {
		return ProfileDraftEnvelope{}, err
	}
	currentDigest := ""
	if found {
		currentDigest = current.Digest
	}
	if baseDigest != currentDigest {
		return ProfileDraftEnvelope{}, fmt.Errorf("%w: profile draft base digest is stale", ErrConflict)
	}
	digest, err := manifest.Digest(value)
	if err != nil {
		return ProfileDraftEnvelope{}, fmt.Errorf("digest profile draft: %w", err)
	}
	envelope := ProfileDraftEnvelope{
		Version:    model.SchemaVersion,
		BaseDigest: baseDigest,
		Digest:     digest,
		UpdatedAt:  Clock().UTC().Format(time.RFC3339),
		Profile:    value,
	}
	if err := atomicWriteJSON(path, envelope); err != nil {
		return ProfileDraftEnvelope{}, err
	}
	return envelope, nil
}
