// Package report reads and writes the JSON documents gamelib exchanges on disk,
// with deterministic encoding so repeated runs produce identical bytes.
package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func WriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".gamelib-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary output: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("write temporary output: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync temporary output: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary output: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("publish output: %w", err)
	}
	return nil
}

func ReadJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read JSON: %w", err)
	}
	if err := json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("decode JSON: %w", err)
	}
	return nil
}
