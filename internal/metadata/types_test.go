package metadata

import "testing"

func TestBuilderRejectsConflictingAliases(t *testing.T) {
	builder := NewBuilder()
	builder.AddAlias("playnite:example", "steam:1")
	builder.AddAlias("playnite:example", "steam:2")
	catalog := builder.Build()
	if got := catalog.Canonical("playnite:example"); got != "playnite:example" {
		t.Fatalf("canonical identity = %q", got)
	}
	if len(catalog.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v", catalog.Diagnostics)
	}
	if !catalog.Ambiguous["playnite:example"] {
		t.Fatal("conflicting identity was not marked ambiguous")
	}
}

func TestStoreNameIsHonest(t *testing.T) {
	if StoreName(SteamPluginID) != "Steam" {
		t.Fatal("Steam plugin was not recognized")
	}

	if StoreName("00000000-0000-0000-0000-000000000000") != "Manually added" {
		t.Fatal("zero plugin should be manually added")
	}

	if StoreName("11111111-1111-1111-1111-111111111111") != "Other library" {
		t.Fatal("unknown plugin should stay generic")
	}

}

func TestTitleUsesSourcePrecedenceAndCanonicalIdentity(t *testing.T) {
	builder := NewBuilder()
	builder.AddTitle("steam:10", "Manifest Name", "steam-manifest")
	builder.AddTitle("steam:10", "Cache Name", "steam-appinfo")
	builder.AddTitle("playnite:synthetic", "Playnite Name", "playnite")
	builder.AddAlias("playnite:synthetic", "steam:10")
	record, ok := builder.Build().Title("playnite:synthetic")
	if !ok || record.Title != "Cache Name" {
		t.Fatalf("resolved title = %+v, %v", record, ok)
	}
}
