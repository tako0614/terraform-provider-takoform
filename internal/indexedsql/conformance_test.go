package indexedsql_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/tako0614/terraform-provider-takoform/internal/indexedsql"
)

type contractManifest struct {
	Format           string   `json:"format"`
	RequestSchema    string   `json:"requestSchema"`
	ResponseSchema   string   `json:"responseSchema"`
	SuccessStatus    int      `json:"successStatus"`
	ConflictStatus   int      `json:"conflictStatus"`
	RequestPositive  []string `json:"requestPositive"`
	RequestNegative  []string `json:"requestNegative"`
	ResponseSuccess  []string `json:"responseSuccess"`
	ResponseConflict []string `json:"responseConflict"`
	ResponseNegative []string `json:"responseNegative"`
}

// TestPublishedSchemasMatchThisImplementation proves the committed
// `data.indexed@1` schemas are the ones this package produces.
//
// The specification is what a host implements against, so an implementation
// that quietly generates something else would be enforcing an unpublished
// contract.
func TestPublishedSchemasMatchThisImplementation(t *testing.T) {
	for name, generated := range map[string]map[string]any{
		"request":  indexedsql.RequestSchema(),
		"response": indexedsql.ResponseSchema(),
	} {
		if _, err := compileSpecSchema(t, name, generated); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

// TestConformanceCorpusStillHolds keeps the retained request/response corpus a
// live gate rather than an unread directory.
func TestConformanceCorpusStillHolds(t *testing.T) {
	base := filepath.Join("..", "..", "conformance", "data-indexed-v1")
	var manifest contractManifest
	readJSON(t, filepath.Join(base, "manifest.json"), &manifest)
	if manifest.Format != "takoform.data-indexed-conformance@v1" ||
		manifest.SuccessStatus != 200 || manifest.ConflictStatus != 409 ||
		len(manifest.RequestPositive) == 0 || len(manifest.RequestNegative) == 0 ||
		len(manifest.ResponseSuccess) == 0 || len(manifest.ResponseConflict) == 0 ||
		len(manifest.ResponseNegative) == 0 {
		t.Fatal("data.indexed@1 conformance manifest is incomplete")
	}
	request, err := compileSpecSchema(t, "request", indexedsql.RequestSchema())
	if err != nil {
		t.Fatal(err)
	}
	response, err := compileSpecSchema(t, "response", indexedsql.ResponseSchema())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		schema    *jsonschema.Schema
		paths     []string
		wantValid bool
		label     string
	}{
		{request, manifest.RequestPositive, true, "request positive"},
		{request, manifest.RequestNegative, false, "request negative"},
		{response, manifest.ResponseSuccess, true, "HTTP 200 response"},
		{response, manifest.ResponseConflict, true, "HTTP 409 response"},
		{response, manifest.ResponseNegative, false, "response negative"},
	} {
		for _, relative := range item.paths {
			var value any
			readJSON(t, filepath.Join(base, filepath.FromSlash(relative)), &value)
			err := item.schema.Validate(value)
			if item.wantValid && err != nil {
				t.Errorf("%s %s: %v", item.label, relative, err)
			}
			if !item.wantValid && err == nil {
				t.Errorf("%s %s unexpectedly passed", item.label, relative)
			}
		}
	}
}

func compileSpecSchema(t *testing.T, name string, generated map[string]any) (*jsonschema.Schema, error) {
	t.Helper()
	var committed any
	readJSON(t, filepath.Join("..", "..", "spec", "data-indexed", name+".schema.json"), &committed)
	generatedRaw, err := json.Marshal(generated)
	if err != nil {
		t.Fatal(err)
	}
	committedRaw, err := json.Marshal(committed)
	if err != nil {
		t.Fatal(err)
	}
	if string(generatedRaw) != string(committedRaw) {
		t.Fatalf("data.indexed@1 %s schema drifted from the published specification", name)
	}
	urn := "urn:takoform:data-indexed:" + name
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(urn, committed); err != nil {
		return nil, err
	}
	return compiler.Compile(urn)
}

func readJSON(t *testing.T, path string, target any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}
