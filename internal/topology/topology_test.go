package topology

import (
	"path/filepath"
	"testing"
)

func TestDefaultIsValid(t *testing.T) {
	if err := Validate(Default()); err != nil {
		t.Fatalf("the default topology must be usable as shipped: %v", err)
	}
}

func TestProfileKeyIsScopedToItsPlatform(t *testing.T) {
	steam := Profile{Platform: "steam", Name: "Minimalist"}
	retro := Profile{Platform: "retro", Name: "Minimalist"}
	if steam.Key() == retro.Key() {
		t.Fatal("a profile name reused on another platform must stay a separate profile")
	}
	if steam.Key() != "steam/minimalist" {
		t.Fatalf("unexpected key %q", steam.Key())
	}
}

func TestSlugHandlesPunctuationAndSpacing(t *testing.T) {
	cases := map[string]string{
		"Standard":      "standard",
		"  Tasteful  ":  "tasteful",
		"Retro Gaming":  "retro-gaming",
		"Canon (JP)":    "canon-jp",
		"A -- B":        "a-b",
		"Minimalist!!!": "minimalist",
		"2600 Classics": "2600-classics",
	}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDevicesForReportsHardwareRunningAPlatform(t *testing.T) {
	doc := Default()
	steam := doc.DevicesFor("steam")
	if len(steam) != 3 {
		t.Fatalf("steam should reach all three devices, got %d", len(steam))
	}
	playnite := doc.DevicesFor("playnite")
	if len(playnite) != 1 || playnite[0].ID != "pc" {
		t.Fatalf("playnite should only reach the PC, got %+v", playnite)
	}
	if len(doc.DevicesFor("nope")) != 0 {
		t.Fatal("an unknown platform must reach no hardware")
	}
}

func TestValidateRejectsDanglingAndDuplicateReferences(t *testing.T) {
	t.Run("unknown platform on a device", func(t *testing.T) {
		doc := Default()
		doc.Devices[0].Platforms = append(doc.Devices[0].Platforms, "ghost")
		if err := Validate(doc); err == nil {
			t.Fatal("expected a dangling platform reference to be rejected")
		}
	})
	t.Run("unknown platform on a profile", func(t *testing.T) {
		doc := Default()
		doc.Profiles = append(doc.Profiles, Profile{Platform: "ghost", Name: "X"})
		if err := Validate(doc); err == nil {
			t.Fatal("expected a profile on an unknown platform to be rejected")
		}
	})
	t.Run("duplicate profile on one platform", func(t *testing.T) {
		doc := Default()
		doc.Profiles = append(doc.Profiles, Profile{Platform: "steam", Name: "standard"})
		if err := Validate(doc); err == nil {
			t.Fatal("expected a duplicate profile key to be rejected")
		}
	})
	t.Run("unsafe artwork set", func(t *testing.T) {
		doc := Default()
		doc.Profiles[0].Artwork = "../escape"
		if err := Validate(doc); err == nil {
			t.Fatal("expected a path-unsafe artwork set to be rejected")
		}
	})
	t.Run("wrong version", func(t *testing.T) {
		doc := Default()
		doc.Version = 2
		if err := Validate(doc); err == nil {
			t.Fatal("expected an unsupported version to be rejected")
		}
	})
}

func TestLoadReturnsDefaultBeforeAnythingIsSaved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "topology.json")
	doc, found, err := Load(path)
	if err != nil {
		t.Fatalf("a missing topology must not be an error: %v", err)
	}
	if found {
		t.Fatal("expected found=false before the first save")
	}
	if len(doc.Profiles) == 0 {
		t.Fatal("the fallback must be the usable default, not an empty document")
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "topology.json")
	doc := Default()
	doc.Profiles[0].Artwork = "deck-default"
	if err := Save(path, doc); err != nil {
		t.Fatal(err)
	}
	got, found, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected found=true after a save")
	}
	if got.Profiles[0].Artwork != "deck-default" {
		t.Fatalf("binding did not round-trip: %+v", got.Profiles[0])
	}
}

func TestSaveRejectsAnInvalidDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "topology.json")
	doc := Default()
	doc.Profiles = append(doc.Profiles, Profile{Platform: "ghost", Name: "X"})
	if err := Save(path, doc); err == nil {
		t.Fatal("expected save to refuse an invalid document")
	}
	if _, found, _ := Load(path); found {
		t.Fatal("a rejected save must not leave a file behind")
	}
}
