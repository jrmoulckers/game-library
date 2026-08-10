package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pngContent is a minimal byte sequence beginning with the real PNG magic
// number so net/http's built-in content sniffing (used by
// internal/media.Inspect) reliably classifies it as image/png without
// needing a fully valid, decodable PNG.
const pngContent = "\x89PNG\r\n\x1a\n" + "0123456789abcdefghijklmnopqrstuvwxyz"

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sha256Like(seed byte) string {
	return strings.Repeat(string(rune('a'+int(seed)%26)), 64)
}

// setMaxMediaServeBytesForTest overrides the package-level media size limit
// for the duration of a test, restoring it via t.Cleanup.
func setMaxMediaServeBytesForTest(t *testing.T, limit int64) {
	t.Helper()
	previous := MaxMediaServeBytes
	MaxMediaServeBytes = limit
	t.Cleanup(func() { MaxMediaServeBytes = previous })
}

// readArtifactBytes reads back an immutable artifact file written by
// workspace.WriteArtifact, for tests that need to assert on the exact
// on-disk bytes (not just the in-memory struct the writer returned).
func readArtifactBytes(t *testing.T, path string) ([]byte, error) {
	t.Helper()
	return os.ReadFile(path)
}
