package report

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteJSONReplacesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(path, []byte(`{"old":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(path, map[string]bool{"new": true}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{\n  \"new\": true\n}\n" {
		t.Fatalf("unexpected replacement contents: %s", data)
	}
}
