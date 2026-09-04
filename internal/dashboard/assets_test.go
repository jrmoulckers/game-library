package dashboard

import (
	"bytes"
	"io/fs"
	"testing"
)

// TestEmbeddedAssetsUseLFOnly guards the determinism constraint in PRODUCT.md:
// a Windows build and a Linux build of the same commit must embed identical
// bytes.
//
// These assets are compiled in with //go:embed, so the embedded bytes are
// whatever the working tree held at build time. `* text=auto` alone normalizes
// only the index and still materializes CRLF in a Windows working tree, which
// previously made a Windows build embed 3,308 more bytes (+3.53%) than a Linux
// build. The `eol=lf` rules in .gitattributes are what keep the two equal, and
// nothing else would notice if they were dropped — the build stays green and
// the dashboard still renders, it just serves host-dependent bytes.
//
// Asserting on the embedded bytes rather than on the files on disk is
// deliberate: it tests the artifact that actually ships.
func TestEmbeddedAssetsUseLFOnly(t *testing.T) {
	sources := map[string]fs.FS{
		"index template": indexTemplateSource,
		"stylesheet":     appCSS,
		"ES modules":     jsFiles,
	}

	checked := 0
	for label, fsys := range sources {
		err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			data, readErr := fs.ReadFile(fsys, path)
			if readErr != nil {
				return readErr
			}
			checked++
			if i := bytes.Index(data, []byte("\r\n")); i >= 0 {
				line := 1 + bytes.Count(data[:i], []byte("\n"))
				t.Errorf("embedded %s asset %q contains CRLF at line %d; "+
					"a Windows build would embed different bytes than a Linux build. "+
					"Check that .gitattributes still pins this file to `eol=lf`.",
					label, path, line)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking embedded %s: %v", label, err)
		}
	}

	// A silent zero-file walk would make this test pass without examining
	// anything, which is the failure mode the test exists to prevent.
	if want := 8; checked != want {
		t.Errorf("checked %d embedded assets, want %d; "+
			"update this count when assets are added or removed so the walk "+
			"cannot silently stop covering them", checked, want)
	}
}
