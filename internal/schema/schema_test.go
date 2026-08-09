package schema

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/jrmoulckers/game-library/internal/model"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
)

func TestSchemasCompileAndValidateFixtures(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	schemaPaths, err := filepath.Glob(filepath.Join(repoRoot, "schemas", "v1", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(schemaPaths)
	if len(schemaPaths) == 0 {
		t.Fatal("no schemas found")
	}

	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	ids := make(map[string]string)
	for _, schemaPath := range schemaPaths {
		data, err := os.ReadFile(schemaPath)
		if err != nil {
			t.Fatal(err)
		}
		var header struct {
			ID string `json:"$id"`
		}
		if err := json.Unmarshal(data, &header); err != nil {
			t.Fatalf("%s: %v", schemaPath, err)
		}
		if header.ID == "" {
			t.Fatalf("%s has no $id", schemaPath)
		}
		if prior, ok := ids[header.ID]; ok {
			t.Fatalf("duplicate schema $id %q in %s and %s", header.ID, prior, schemaPath)
		}
		ids[header.ID] = schemaPath
		if err := compiler.AddResource(header.ID, bytes.NewReader(data)); err != nil {
			t.Fatalf("register %s: %v", schemaPath, err)
		}
	}
	for id, schemaPath := range ids {
		if _, err := compiler.Compile(id); err != nil {
			t.Fatalf("compile %s: %v", schemaPath, err)
		}
	}

	validateJSONFile(t, compiler,
		"https://schemas.game-library.dev/v1/canonical-profile.schema.json",
		filepath.Join(repoRoot, "testdata", "profiles", "example.json"))
	validateJSONFile(t, compiler,
		"https://schemas.game-library.dev/v1/inventory-report.schema.json",
		filepath.Join(repoRoot, "reports", "baseline", "desktop-2026-08-09.json"))
	validateJSONFile(t, compiler,
		"https://schemas.game-library.dev/v1/decky-profile-v1.schema.json",
		filepath.Join(repoRoot, "testdata", "decky", "deck-default.json"))
	validateJSONFile(t, compiler,
		"https://schemas.game-library.dev/v1/decky-profile-v1.schema.json",
		filepath.Join(repoRoot, "testdata", "decky", "steam-default.json"))
	artwork := "steam-default"
	validateValue(t, compiler,
		"https://schemas.game-library.dev/v1/decky-profile-v1.schema.json",
		model.DeckyProfileV1{
			Version: 1, ID: "steam-default", Name: "Steam default",
			Artwork: &artwork, Mods: []model.DeckyModV1{},
		})
	validateValue(t, compiler,
		"https://schemas.game-library.dev/v1/policy.schema.json",
		model.PolicyFile{
			Version: 1, Default: "tracked-external",
			Rules: []model.PolicyRule{{System: "n64", Role: "manual", Mode: "managed"}},
		})
	validateValue(t, compiler,
		"https://schemas.game-library.dev/v1/migration-manifest.schema.json",
		model.Manifest{
			Version: 1, ToolVersion: "test", OperationID: "import-1234", Kind: "import-plan",
			SourceDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Actions: []model.Action{{
				Action: "copy", SourceRoot: "source", SourcePath: "relative/asset.png",
				SourceSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				SourceSize:   12, DestinationRoot: "catalog",
				DestinationPath:     "library/assets/sha256/aa/hash/content.png",
				ExpectedDestination: "absent-or-same-hash", Reason: "test",
			}},
		})
}

func validateJSONFile(t *testing.T, compiler *jsonschema.Compiler, schemaID, instancePath string) {
	t.Helper()
	schema, err := compiler.Compile(schemaID)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(instancePath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var value any
	decoder := json.NewDecoder(file)
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(value); err != nil {
		t.Fatalf("%s: %v", instancePath, err)
	}
}

func validateValue(t *testing.T, compiler *jsonschema.Compiler, schemaID string, value any) {
	t.Helper()
	schema, err := compiler.Compile(schemaID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var raw any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(raw); err != nil {
		t.Fatalf("%s: %v", schemaID, err)
	}
}
