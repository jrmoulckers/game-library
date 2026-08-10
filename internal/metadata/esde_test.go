package metadata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
)

func TestReadGamelistUsesExactRawStem(t *testing.T) {
	xml := `<gameList>
  <game><path>./Example Game (Region) [Dump].zip</path><name>Example Display Name</name><desc>ignored</desc></game>
  <game><path>folder\Second Game.rom</path><name>Second Display Name</name></game>
</gameList>`
	entries, err := ReadGamelist(strings.NewReader(xml), "n64")
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 2 {
		t.Fatalf("entries = %#v", entries)
	}
	if entries[0].RawStem != "Example Game (Region) [Dump]" || entries[0].Name != "Example Display Name" {
		t.Fatalf("first entry = %+v", entries[0])
	}
	if entries[1].RawStem != "Second Game" {
		t.Fatalf("second entry = %+v", entries[1])
	}
}

func TestResolveESDEUsesSiblingGamelists(t *testing.T) {
	base := t.TempDir()
	mediaRoot := filepath.Join(base, "downloaded_media")
	gamelist := filepath.Join(base, "gamelists", "n64", "gamelist.xml")
	if err := os.MkdirAll(filepath.Dir(gamelist), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(mediaRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	data := `<gameList><game><path>./Example Game (Region).rom</path><name>Resolved Example</name></game></gameList>`
	if err := os.WriteFile(gamelist, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	catalog := ResolveESDE([]model.Root{{Kind: "esde-media", Path: mediaRoot}}, ESDEFileSystem{})
	record, ok := catalog.Title("retro:n64:example-game-region")
	if !ok || record.Title != "Resolved Example" {
		t.Fatalf("resolved title = %+v, %v", record, ok)
	}
}

func TestReadGamelistRejectsMalformedOrOversizedInput(t *testing.T) {
	if _, err := ReadGamelist(strings.NewReader(`<gameList><game><path>x.rom</path>`), "n64"); err == nil {
		t.Fatal("expected truncated XML to fail")
	}
	oversized := `<gameList><game><path>` + strings.Repeat("x", maxGamelistText+1) + `</path><name>Example</name></game></gameList>`
	if _, err := ReadGamelist(strings.NewReader(oversized), "n64"); err == nil {
		t.Fatal("expected oversized value to fail")
	}
}

func TestReadGamelistDoesNotExpandCustomEntities(t *testing.T) {
	value := `<!DOCTYPE gameList [<!ENTITY sample "expanded">]><gameList><game><path>x.rom</path><name>&sample;</name></game></gameList>`
	if _, err := ReadGamelist(strings.NewReader(value), "n64"); err == nil {
		t.Fatal("expected custom entity to be rejected")
	}

}

func TestResolveESDEDisambiguatesLossyStemCollisions(t *testing.T) {
	base := t.TempDir()
	mediaRoot := filepath.Join(base, "downloaded_media")
	gamelist := filepath.Join(base, "gamelists", "n64", "gamelist.xml")
	if err := os.MkdirAll(filepath.Dir(gamelist), 0o755); err != nil {
		t.Fatal(err)
	}
	data := `<gameList>
<game><path>A-B.rom</path><name>First Synthetic Game</name></game>
<game><path>A B.rom</path><name>Second Synthetic Game</name></game>
</gameList>`
	if err := os.WriteFile(gamelist, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	catalog := ResolveESDE([]model.Root{{Kind: "esde-media", Path: mediaRoot}}, ESDEFileSystem{})
	first := DisambiguatedRetroIdentity("n64", "A-B")
	second := DisambiguatedRetroIdentity("n64", "A B")
	if first == second {
		t.Fatal("disambiguated identities collided")
	}
	if title, ok := catalog.Title(first); !ok || title.Title != "First Synthetic Game" {
		t.Fatalf("first title = %+v, %v", title, ok)
	}
	if title, ok := catalog.Title(second); !ok || title.Title != "Second Synthetic Game" {
		t.Fatalf("second title = %+v, %v", title, ok)
	}
}
