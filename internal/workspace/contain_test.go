package workspace

import (
	"path/filepath"
	"testing"
)

func TestContainAcceptsSimpleRelativePaths(t *testing.T) {
	base := t.TempDir()
	got, err := Contain(base, "profile-example.json")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(base, "profile-example.json")
	if got != want {
		t.Fatalf("Contain() = %q, want %q", got, want)
	}
}

func TestContainAcceptsNestedRelativePaths(t *testing.T) {
	base := t.TempDir()
	got, err := Contain(base, "gate-reviews/example/report.json")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(base, "gate-reviews", "example", "report.json")
	if got != want {
		t.Fatalf("Contain() = %q, want %q", got, want)
	}
}

func TestContainSupportsUnicodeNames(t *testing.T) {
	base := t.TempDir()
	got, err := Contain(base, "profil-café-日本語.json")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(base, "profil-café-日本語.json")
	if got != want {
		t.Fatalf("Contain() = %q, want %q", got, want)
	}
}

func TestContainRejectsTraversal(t *testing.T) {
	base := t.TempDir()
	cases := []string{
		"..",
		"../secret.json",
		"a/../../secret.json",
		"a/../..",
		"./../secret.json",
	}
	for _, c := range cases {
		if _, err := Contain(base, c); err == nil {
			t.Fatalf("Contain(%q) unexpectedly succeeded", c)
		}
	}
}

func TestContainRejectsAbsoluteAndDriveLetterAndUNC(t *testing.T) {
	base := t.TempDir()
	cases := []string{
		"/etc/passwd",
		"C:/Windows/System32",
		"D:/GamingProfiles/library",
	}
	for _, c := range cases {
		if _, err := Contain(base, c); err == nil {
			t.Fatalf("Contain(%q) unexpectedly succeeded", c)
		}
	}
}

func TestContainRejectsBackslashesRegardlessOfOS(t *testing.T) {
	base := t.TempDir()
	cases := []string{
		`..\secret.json`,
		`a\..\..\secret.json`,
		`\\server\share\file.json`,
	}
	for _, c := range cases {
		if _, err := Contain(base, c); err == nil {
			t.Fatalf("Contain(%q) unexpectedly succeeded", c)
		}
	}
}

func TestContainRejectsEmptyAndNulAndEmptySegments(t *testing.T) {
	base := t.TempDir()
	cases := []string{
		"",
		"a//b.json",
		"a/\x00b.json",
	}
	for _, c := range cases {
		if _, err := Contain(base, c); err == nil {
			t.Fatalf("Contain(%q) unexpectedly succeeded", c)
		}
	}
}
