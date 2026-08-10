package workspace

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jrmoulckers/game-library/internal/model"
)

func testPaths(t *testing.T) Paths {
	t.Helper()
	return NewPaths(t.TempDir())
}

func examplePolicyFile() model.PolicyFile {
	return model.PolicyFile{
		Version: model.SchemaVersion,
		Default: "tracked-external",
		Rules: []model.PolicyRule{
			{Source: "canonical-catalog", Mode: "managed"},
		},
	}
}

func exampleProfile(id string) model.Profile {
	return model.Profile{
		Version: model.SchemaVersion,
		ID:      id,
		Name:    "Example profile",
		Games: []model.ProfileGame{
			{
				ID: "steam:123",
				Assets: map[string]model.AssetSelection{
					"grid": {SHA256: strings.Repeat("a", 64), Extension: "png"},
				},
			},
		},
	}
}

func TestSavePolicyDraftRequiresEmptyBaseDigestWhenAbsent(t *testing.T) {
	paths := testPaths(t)
	if _, err := SavePolicyDraft(paths, "stale", examplePolicyFile()); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict for a non-empty base digest with no existing draft, got %v", err)
	}
}

func TestSavePolicyDraftRoundTripsAndUpdatesDigest(t *testing.T) {
	paths := testPaths(t)
	Clock = func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }
	defer func() { Clock = time.Now }()

	envelope, err := SavePolicyDraft(paths, "", examplePolicyFile())
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Digest == "" {
		t.Fatal("expected a non-empty digest")
	}
	if envelope.UpdatedAt != "2026-01-02T03:04:05Z" {
		t.Fatalf("unexpected timestamp: %q", envelope.UpdatedAt)
	}

	loaded, found, err := LoadPolicyDraft(paths)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected the draft to be found after saving")
	}
	if loaded.Digest != envelope.Digest {
		t.Fatalf("loaded digest %q != saved digest %q", loaded.Digest, envelope.Digest)
	}

	updated := examplePolicyFile()
	updated.Default = "quarantined"
	next, err := SavePolicyDraft(paths, envelope.Digest, updated)
	if err != nil {
		t.Fatal(err)
	}
	if next.Digest == envelope.Digest {
		t.Fatal("expected the digest to change after editing the draft")
	}
}

func TestSavePolicyDraftRejectsStaleBaseDigest(t *testing.T) {
	paths := testPaths(t)
	envelope, err := SavePolicyDraft(paths, "", examplePolicyFile())
	if err != nil {
		t.Fatal(err)
	}
	_, err = SavePolicyDraft(paths, "not-"+envelope.Digest, examplePolicyFile())
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict for a stale base digest, got %v", err)
	}
	// The on-disk draft must remain untouched after a rejected write.
	loaded, _, err := LoadPolicyDraft(paths)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Digest != envelope.Digest {
		t.Fatal("a rejected stale write must not modify the stored draft")
	}
}

func TestSavePolicyDraftRejectsInvalidPolicy(t *testing.T) {
	paths := testPaths(t)
	invalid := model.PolicyFile{Version: model.SchemaVersion, Default: "not-a-real-mode"}
	if _, err := SavePolicyDraft(paths, "", invalid); err == nil {
		t.Fatal("expected an error for an invalid policy default mode")
	}
}

func TestProfileDraftPathRejectsUnsafeID(t *testing.T) {
	paths := testPaths(t)
	if _, err := ProfileDraftPath(paths, "../escape"); err == nil {
		t.Fatal("expected an error for a path-unsafe profile id")
	}
	if _, err := ProfileDraftPath(paths, "con"); err == nil {
		t.Fatal("expected an error for a reserved Windows device name id")
	}
}

func TestSaveProfileDraftRoundTripsAndConflicts(t *testing.T) {
	paths := testPaths(t)
	profile := exampleProfile("example")

	envelope, err := SaveProfileDraft(paths, "", profile)
	if err != nil {
		t.Fatal(err)
	}

	loaded, found, err := LoadProfileDraft(paths, "example")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected the draft to be found after saving")
	}
	if loaded.Digest != envelope.Digest {
		t.Fatalf("loaded digest %q != saved digest %q", loaded.Digest, envelope.Digest)
	}

	if _, err := SaveProfileDraft(paths, "stale-digest", profile); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict for a stale base digest, got %v", err)
	}
}

func TestSaveProfileDraftRejectsInvalidProfile(t *testing.T) {
	paths := testPaths(t)
	invalid := model.Profile{Version: model.SchemaVersion, ID: "example"} // no name, no games
	if _, err := SaveProfileDraft(paths, "", invalid); err == nil {
		t.Fatal("expected an error for an invalid profile")
	}
}

func TestProfileDraftNeverContainsACanonicalLibraryPath(t *testing.T) {
	paths := testPaths(t)
	profile := exampleProfile("example")
	envelope, err := SaveProfileDraft(paths, "", profile)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	// Profile drafts address assets purely by content hash; the canonical
	// content-addressed storage layout ("library/assets/sha256/...") is
	// derived by the existing profile package at resolve time and must
	// never be written into a draft.
	if strings.Contains(string(data), "library/assets") {
		t.Fatalf("profile draft leaked a canonical library path: %s", data)
	}
}

func TestListProfileDraftIDsReportsEmptyBeforeAnySave(t *testing.T) {
	paths := testPaths(t)
	ids, err := ListProfileDraftIDs(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected no profile draft ids yet, got %v", ids)
	}
}

func TestListProfileDraftIDsReturnsSortedSavedDrafts(t *testing.T) {
	paths := testPaths(t)
	for _, id := range []string{"zeta", "alpha", "mid"} {
		if _, err := SaveProfileDraft(paths, "", exampleProfile(id)); err != nil {
			t.Fatal(err)
		}
	}
	ids, err := ListProfileDraftIDs(paths)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"alpha", "mid", "zeta"}
	if len(ids) != len(want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("ids = %v, want %v", ids, want)
		}
	}
}

func TestListProfileDraftIDsIgnoresUnrelatedFiles(t *testing.T) {
	paths := testPaths(t)
	if _, err := SavePolicyDraft(paths, "", examplePolicyFile()); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveProfileDraft(paths, "", exampleProfile("example")); err != nil {
		t.Fatal(err)
	}
	ids, err := ListProfileDraftIDs(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "example" {
		t.Fatalf("expected only the profile draft id, got %v", ids)
	}
}
