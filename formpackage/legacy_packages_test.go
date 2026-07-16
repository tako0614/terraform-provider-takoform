package formpackage

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type legacyPackageExpectation struct {
	kind          string
	path          string
	packageDigest string
	schemaDigest  string
	mutateInvalid func(map[string]any)
}

type legacyPackageSet struct {
	Format                 string                  `json:"format"`
	Classification         string                  `json:"classification"`
	DefinitionVersion      string                  `json:"definitionVersion"`
	PackageVersion         string                  `json:"packageVersion"`
	PublicationReady       bool                    `json:"publicationReady"`
	SignatureStatus        string                  `json:"signatureStatus"`
	SignatureBundle        any                     `json:"signatureBundle"`
	SourceCharacterization string                  `json:"sourceCharacterization"`
	WireMapping            string                  `json:"wireMapping"`
	ConformanceManifest    string                  `json:"conformanceManifest"`
	Packages               []legacyPackageSetEntry `json:"packages"`
}

type legacyPackageSetEntry struct {
	Kind            string  `json:"kind"`
	Path            string  `json:"path"`
	ConformanceCase string  `json:"conformanceCase"`
	FormRef         FormRef `json:"formRef"`
	PackageDigest   string  `json:"packageDigest"`
}

func TestLegacyCompatibilityPackagesAreExactAndClosed(t *testing.T) {
	t.Parallel()
	for _, test := range legacyPackageExpectations() {
		test := test
		t.Run(test.kind, func(t *testing.T) {
			t.Parallel()
			root := filepath.Join("..", "conformance", "form-package-v1", "positive", "legacy", test.path)
			report, err := VerifyDirectory(root)
			if err != nil {
				t.Fatal(err)
			}
			if report.PackageDigest != test.packageDigest {
				t.Fatalf("package digest = %q, want %q", report.PackageDigest, test.packageDigest)
			}
			if report.FormRef.Kind != test.kind || report.FormRef.DefinitionVersion != "0.0.0-legacy.1" || report.FormRef.SchemaDigest != test.schemaDigest {
				t.Fatalf("unexpected FormRef: %+v", report.FormRef)
			}

			definitionRaw, err := os.ReadFile(filepath.Join(root, "definition.json"))
			if err != nil {
				t.Fatal(err)
			}
			definition, _, err := validateDefinition(definitionRaw)
			if err != nil {
				t.Fatal(err)
			}
			if definition.Status != "compatibility-candidate" {
				t.Fatalf("status = %q, want compatibility-candidate", definition.Status)
			}
			if !slices.Equal(definition.ImmutableFields, []string{"/name"}) {
				t.Fatalf("immutableFields = %v, want only /name", definition.ImmutableFields)
			}
			if !slices.Equal(definition.LifecycleCapabilities, []string{"create", "update", "observe", "delete", "import"}) {
				t.Fatalf("lifecycleCapabilities = %v, want complete characterized lifecycle", definition.LifecycleCapabilities)
			}
			if len(definition.Interfaces) != 0 {
				t.Fatalf("interfaces = %v, want no invented runtime interface descriptors", definition.Interfaces)
			}
			observedProperties, ok := definition.ObservedSchema["properties"].(map[string]any)
			if definition.ObservedSchema["type"] != "object" || definition.ObservedSchema["additionalProperties"] != false || !ok || len(observedProperties) != 0 {
				t.Fatalf("observedSchema = %v, want an exact closed empty host-authority boundary", definition.ObservedSchema)
			}
			if test.kind == "VectorIndex" && slices.Contains(definition.ImmutableFields, "/dimensions") {
				t.Fatal("VectorIndex dimensions must not be invented as immutable")
			}

			compiled, err := compileInlineSchema(definition.DesiredSchema, test.kind+".desiredSchema")
			if err != nil {
				t.Fatal(err)
			}
			fixtureRaw, err := os.ReadFile(filepath.Join(root, "fixtures", "desired.json"))
			if err != nil {
				t.Fatal(err)
			}
			var desired map[string]any
			if err := json.Unmarshal(fixtureRaw, &desired); err != nil {
				t.Fatal(err)
			}
			if err := compiled.Validate(desired); err != nil {
				t.Fatalf("positive fixture rejected: %v", err)
			}

			unknown := cloneJSONMap(t, desired)
			unknown["hostExtension"] = true
			if err := compiled.Validate(unknown); err == nil {
				t.Fatal("unknown host extension unexpectedly accepted")
			}

			invalid := cloneJSONMap(t, desired)
			test.mutateInvalid(invalid)
			if err := compiled.Validate(invalid); err == nil {
				t.Fatal("kind-specific invalid fixture unexpectedly accepted")
			}
		})
	}
}

func legacyPackageExpectations() []legacyPackageExpectation {
	return []legacyPackageExpectation{
		{
			kind: "EdgeWorker", path: "edge-worker",
			packageDigest: "sha256:c44e2ad933de36d77c61b9b24df76a56b9ee9ff265e3085e88061ce755d6f8b6",
			schemaDigest:  "sha256:ce55ac9ea700ac391637ca29f149439ee0fcc54a9983d4513023f097cccf02b0",
			mutateInvalid: func(value map[string]any) {
				value["source"] = map[string]any{"artifactUrl": "https://artifacts.example.test/edge.js"}
			},
		},
		{
			kind: "ObjectBucket", path: "object-bucket",
			packageDigest: "sha256:0c43dfbf565c959ad627a6cd8d19aa77bf56d9e3655f44f71bb207fb79b264f2",
			schemaDigest:  "sha256:ee32286a40681296fc6f3db9ece79c2d651821aa2e947d1fa1cd6e28e8be8391",
			mutateInvalid: func(value map[string]any) { value["storageClass"] = "cold" },
		},
		{
			kind: "KVStore", path: "kv-store",
			packageDigest: "sha256:7bdc1933764bcd7687980acb97b8fcb82f12ce7a5e853c988b508997a60895dd",
			schemaDigest:  "sha256:3b3f8d369eba1e41c4de7093229698ecc54c30103351e670422f2da4d8a033d6",
			mutateInvalid: func(value map[string]any) { value["consistency"] = "linearizable" },
		},
		{
			kind: "Queue", path: "queue",
			packageDigest: "sha256:87dcc5fb75f980ade0b0775751a0ee6a49cd8ff3aab4523f4bf8651719c7dd0e",
			schemaDigest:  "sha256:313fc48201f2b324519d5869a2b819df31a09411704265f3ad633bc0d7384a15",
			mutateInvalid: func(value map[string]any) {
				value["delivery"] = map[string]any{"maxRetries": -1}
			},
		},
		{
			kind: "SQLDatabase", path: "sql-database",
			packageDigest: "sha256:1e206b6bcaf069a4fd6aea48cc5a2262b2758e47bf29dddef690ba4ba3f97a90",
			schemaDigest:  "sha256:8ba271241cca83d802c0e3e2e3fc1ee488ef912389364ca35e7db54abbb6c17c",
			mutateInvalid: func(value map[string]any) { value["engine"] = "not a token" },
		},
		{
			kind: "ContainerService", path: "container-service",
			packageDigest: "sha256:13fee163873e9ed84c6be612b28c1df273fdeeedfbc8ddbb405f14e897c0d075",
			schemaDigest:  "sha256:85f290f96799788f8bd544894b9a26f8e0b2551b6537ad8eb4c7e11fea52d9d7",
			mutateInvalid: func(value map[string]any) { value["ports"] = []any{0.0} },
		},
		{
			kind: "VectorIndex", path: "vector-index",
			packageDigest: "sha256:968bdd942bfa404f38eb33cc9adca185b108c7653b7030d236186b9c5521cb00",
			schemaDigest:  "sha256:328c601c50511184d46266f28cdb09a46d3b526127d46ca66f2a8f41f04bc884",
			mutateInvalid: func(value map[string]any) { value["dimensions"] = 0.0 },
		},
		{
			kind: "DurableWorkflow", path: "durable-workflow",
			packageDigest: "sha256:8bc7d360e007a1d69ba8ba9aae2356da3a03c0e32ca9a6dbffe173fa42fcef59",
			schemaDigest:  "sha256:fb713cdaa4db5da7dbfae1b106fc9ac498566356026f3c3427eb90a2c356ba7d",
			mutateInvalid: func(value map[string]any) {
				value["retry"] = map[string]any{"maxAttempts": 0.0}
			},
		},
		{
			kind: "StatefulActorNamespace", path: "stateful-actor-namespace",
			packageDigest: "sha256:b9a1fea011dcc1b049c21abeef214519982f138ff832812e9546f3918c7bb1ea",
			schemaDigest:  "sha256:feb30f237bee2f8baaeefb70147c96ed84d3f3d7a71963e290b135bdec83962f",
			mutateInvalid: func(value map[string]any) { value["className"] = "bad-name" },
		},
		{
			kind: "Schedule", path: "schedule",
			packageDigest: "sha256:50f97f5bf7f62763103bf716e3a9efba8856ef5fc7d117934c0dd6f896eea4c6",
			schemaDigest:  "sha256:04595cf30f2e92f899bda8655db3e6677c408a30f615d79b4273e4b0f98bf7ba",
			mutateInvalid: func(value map[string]any) { value["connections"] = map[string]any{} },
		},
	}
}

func cloneJSONMap(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}

func TestLegacyCompatibilityDefinitionsContainNoHostAuthorityFields(t *testing.T) {
	t.Parallel()
	for _, test := range legacyPackageExpectations() {
		raw, err := os.ReadFile(filepath.Join("..", "conformance", "form-package-v1", "positive", "legacy", test.path, "definition.json"))
		if err != nil {
			t.Fatal(err)
		}
		text := strings.ToLower(string(raw))
		for _, forbidden := range []string{"targetpool", "providerconnection", "credentialrecipe", "billingaccount", "serviceoffering"} {
			if strings.Contains(text, `"`+forbidden+`"`) {
				t.Fatalf("%s definition contains host authority field %q", test.kind, forbidden)
			}
		}
	}
}

func TestLegacyPackageSetPinsBackfillEvidence(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("..", "forms", "legacy-package-set.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Canonicalize(raw); err != nil {
		t.Fatalf("package set is not I-JSON: %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var set legacyPackageSet
	if err := decoder.Decode(&set); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatalf("package set trailing data: %v", err)
	}
	if set.Format != "takoform.legacy-package-set@v1" || set.Classification != "compatibility-candidate" || set.DefinitionVersion != "0.0.0-legacy.1" || set.PackageVersion != "0.0.0-legacy.1" {
		t.Fatalf("unexpected package-set identity: %+v", set)
	}
	if set.PublicationReady || set.SignatureStatus != "unsigned" || set.SignatureBundle != nil {
		t.Fatalf("legacy package set must remain explicitly unsigned and non-publishable: %+v", set)
	}
	if set.SourceCharacterization != "conformance/compatibility-candidate-v1/manifest.json" || set.WireMapping != "forms/legacy-takosumi-wire-mapping.md" || set.ConformanceManifest != "conformance/form-package-v1/manifest.json" {
		t.Fatalf("unexpected evidence paths: %+v", set)
	}
	for _, evidencePath := range []string{set.SourceCharacterization, set.WireMapping, set.ConformanceManifest} {
		info, err := os.Stat(filepath.Join("..", filepath.FromSlash(evidencePath)))
		if err != nil {
			t.Fatalf("evidence path %q: %v", evidencePath, err)
		}
		if !info.Mode().IsRegular() {
			t.Fatalf("evidence path %q is not a regular file", evidencePath)
		}
	}

	expected := legacyPackageExpectations()
	if len(set.Packages) != len(expected) {
		t.Fatalf("package set has %d entries, want %d", len(set.Packages), len(expected))
	}
	for index, want := range expected {
		entry := set.Packages[index]
		wantPath := "conformance/form-package-v1/positive/legacy/" + want.path
		wantCase := "legacy-" + want.path + "-package"
		if entry.Kind != want.kind || entry.Path != wantPath || entry.ConformanceCase != wantCase || entry.PackageDigest != want.packageDigest {
			t.Fatalf("package-set entry %d = %+v, want %s at %s", index, entry, want.kind, wantPath)
		}
		if entry.FormRef != (FormRef{APIVersion: FormAPIVersion, Kind: want.kind, DefinitionVersion: "0.0.0-legacy.1", SchemaDigest: want.schemaDigest}) {
			t.Fatalf("package-set entry %d has unexpected FormRef: %+v", index, entry.FormRef)
		}
		report, err := VerifyDirectory(filepath.Join("..", filepath.FromSlash(entry.Path)))
		if err != nil {
			t.Fatal(err)
		}
		if report.FormRef != entry.FormRef || report.PackageDigest != entry.PackageDigest {
			t.Fatalf("package-set entry %s drifted from strict verifier: %+v", entry.Kind, report)
		}
	}
}
