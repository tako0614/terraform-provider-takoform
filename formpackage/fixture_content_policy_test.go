package formpackage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyDirectoryUsesTheReviewedCommandSchemaForDesiredFixtures(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		desired map[string]any
		wantErr string
	}{
		"bounded declarative command": {
			desired: map[string]any{
				"mainModule": "worker.mjs",
				"command":    []any{"/app/server", "--serve"},
			},
		},
		"embedded script remains forbidden": {
			desired: map[string]any{
				"mainModule": "worker.mjs",
				"script":     "rm -rf /",
			},
			wantErr: `forbidden field "script"`,
		},
		"backend command remains forbidden": {
			desired: map[string]any{
				"mainModule":     "worker.mjs",
				"backendCommand": []any{"restart"},
			},
			wantErr: `forbidden field "backendCommand"`,
		},
		"free form command payload is not accepted": {
			desired: map[string]any{
				"mainModule": "worker.mjs",
				"command": map[string]any{
					"payload": "arbitrary shell text",
				},
			},
			wantErr: "schema validation failed",
		},
	}
	for name, testCase := range tests {
		name, testCase := name, testCase
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := makeFamilyPackage(t, func(definition map[string]any) {
				desired := definition["desiredSchema"].(map[string]any)
				properties := desired["properties"].(map[string]any)
				properties["command"] = reviewedCommandSchema()
				definition["immutableFields"] = append(
					definition["immutableFields"].([]any),
					"/command",
				)
			})
			replacePackageJSONPayload(t, root, "fixtures/desired.json", testCase.desired)
			_, err := VerifyDirectory(root)
			if testCase.wantErr == "" {
				if err != nil {
					t.Fatalf("bounded command fixture failed: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("VerifyDirectory error = %v, want %q", err, testCase.wantErr)
			}
		})
	}
}

func reviewedCommandSchema() map[string]any {
	return map[string]any{
		"type":        "array",
		"maxItems":    64,
		"description": "Bounded declarative process entrypoint override.",
		"items": map[string]any{
			"type":      "string",
			"maxLength": 256,
			"pattern":   `^[^\x00\r\n]{1,256}$`,
		},
	}
}

func replacePackageJSONPayload(t *testing.T, root, relative string, value any) {
	t.Helper()
	raw := canonicalMarshal(t, value)
	writeFixtureFile(t, filepath.Join(root, filepath.FromSlash(relative)), raw, 0o644)
	indexPath := filepath.Join(root, PackageIndexFilename)
	indexRaw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	var index map[string]any
	if err := json.Unmarshal(indexRaw, &index); err != nil {
		t.Fatal(err)
	}
	updated := false
	for _, entry := range index["files"].([]any) {
		file := entry.(map[string]any)
		if file["path"] != relative {
			continue
		}
		file["size"] = len(raw)
		file["digest"] = DigestBytes(raw)
		updated = true
	}
	if !updated {
		t.Fatalf("package index does not list %s", relative)
	}
	writeFixtureFile(t, indexPath, canonicalMarshal(t, index), 0o644)
}
