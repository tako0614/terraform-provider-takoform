package currentformsnapshot

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

func TestCompileAcceptsZeroFamilies(t *testing.T) {
	snapshot, diagnostics := Compile(Input{HostAPI: "forms.takoform.com/v1"})
	if len(diagnostics) != 0 {
		t.Fatalf("Compile returned diagnostics: %#v", diagnostics)
	}
	if snapshot == nil {
		t.Fatal("Compile returned no Snapshot")
	}
	if got := snapshot.Forms(); len(got) != 0 {
		t.Fatalf("zero-family Snapshot contains %d Forms: %#v", len(got), got)
	}
	if snapshot.Digest() == "" {
		t.Fatal("zero-family Snapshot has no deterministic digest")
	}
}

func TestCompileRefusesHistoricalHostAPILanesForZeroFamilies(t *testing.T) {
	for _, hostAPI := range []string{
		"forms.takoform.com/v1beta1",
		"forms.takoform.com/v1beta4",
	} {
		t.Run(hostAPI, func(t *testing.T) {
			snapshot, diagnostics := Compile(Input{HostAPI: hostAPI})
			if snapshot != nil {
				t.Fatal("Compile returned a zero-family Snapshot for an unserved Host API lane")
			}
			if len(diagnostics) != 1 || diagnostics[0].Code != DiagnosticUnsupportedHostAPI {
				t.Fatalf("historical Host API diagnostics = %#v, want one %q", diagnostics, DiagnosticUnsupportedHostAPI)
			}
			if diagnostics[0].Pointer != "/hostApi" {
				t.Fatalf("historical Host API diagnostic pointer = %q, want /hostApi", diagnostics[0].Pointer)
			}
		})
	}
}

func TestHostAPILaneComparisonRetainsHistoricalLowerBounds(t *testing.T) {
	for _, test := range []struct {
		name     string
		served   string
		required string
		want     bool
	}{
		{name: "stable serves retained beta", served: "forms.takoform.com/v1", required: "forms.takoform.com/v1beta1", want: true},
		{name: "beta4 serves beta1 lower bound", served: "forms.takoform.com/v1beta4", required: "forms.takoform.com/v1beta1", want: true},
		{name: "beta1 does not serve beta4", served: "forms.takoform.com/v1beta1", required: "forms.takoform.com/v1beta4", want: false},
		{name: "unknown lane fails closed", served: "forms.takoform.com/v2", required: "forms.takoform.com/v1", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := hostAPISatisfies(test.served, test.required); got != test.want {
				t.Fatalf("hostAPISatisfies(%q, %q) = %v, want %v", test.served, test.required, got, test.want)
			}
		})
	}
}

func TestCompileRefusesUnknownOrInsufficientHostAPILanes(t *testing.T) {
	artifact, ref := syntheticPackage(t, "forms.example.com", "NeedsStable", "0.1.0")
	for _, test := range []struct {
		name    string
		hostAPI string
		code    DiagnosticCode
	}{
		{name: "unknown syntactically valid lane", hostAPI: "forms.takoform.com/v2beta1", code: DiagnosticInvalidInput},
		{name: "known but too old lane", hostAPI: "forms.takoform.com/v1beta4", code: DiagnosticUnsupportedHostAPI},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot, diagnostics := Compile(Input{
				HostAPI:        test.hostAPI,
				Packages:       []PackageArtifact{artifact},
				DefaultCreates: []DefaultPin{{Group: ref.APIVersion, Kind: ref.Kind, Ref: ref}},
			})
			if snapshot != nil {
				t.Fatal("Compile returned a Snapshot for an unsupported Host API")
			}
			if len(diagnostics) != 1 || diagnostics[0].Code != test.code {
				t.Fatalf("Compile diagnostics = %#v, want one %q", diagnostics, test.code)
			}
		})
	}
}

func TestCompileIsGroupFirstAndOrderIndependent(t *testing.T) {
	first, firstRef := syntheticPackage(t, "forms.example.com", "SharedKind", "0.1.0")
	second, secondRef := syntheticPackage(t, "forms.other.example", "SharedKind", "1.2.0")

	forward := Input{
		HostAPI:  "forms.takoform.com/v1",
		Packages: []PackageArtifact{first, second},
		DefaultCreates: []DefaultPin{
			{Group: firstRef.APIVersion, Kind: firstRef.Kind, Ref: firstRef},
			{Group: secondRef.APIVersion, Kind: secondRef.Kind, Ref: secondRef},
		},
	}
	reverse := Input{
		HostAPI:  forward.HostAPI,
		Packages: []PackageArtifact{second, first},
		DefaultCreates: []DefaultPin{
			forward.DefaultCreates[1],
			forward.DefaultCreates[0],
		},
	}

	left, leftDiagnostics := Compile(forward)
	right, rightDiagnostics := Compile(reverse)
	if len(leftDiagnostics) != 0 || len(rightDiagnostics) != 0 {
		t.Fatalf("Compile diagnostics differ: left=%#v right=%#v", leftDiagnostics, rightDiagnostics)
	}
	if left == nil || right == nil {
		t.Fatalf("Compile returned partial Snapshots: left=%v right=%v", left != nil, right != nil)
	}
	if left.Digest() != right.Digest() {
		t.Fatalf("input permutation changed Snapshot digest: %s != %s", left.Digest(), right.Digest())
	}
	if !reflect.DeepEqual(left.Forms(), right.Forms()) {
		t.Fatalf("input permutation changed stable Form view:\nleft=%#v\nright=%#v", left.Forms(), right.Forms())
	}

	gotFirst, ok := left.Default(firstRef.APIVersion, firstRef.Kind)
	if !ok || gotFirst != firstRef {
		t.Fatalf("first group default = %#v, %v; want %#v", gotFirst, ok, firstRef)
	}
	gotSecond, ok := left.Default(secondRef.APIVersion, secondRef.Kind)
	if !ok || gotSecond != secondRef {
		t.Fatalf("second group default = %#v, %v; want %#v", gotSecond, ok, secondRef)
	}
	if _, ok := left.Default("wrong.example", firstRef.Kind); ok {
		t.Fatal("Kind-only fallback crossed a family group")
	}
}

func TestCompileReturnsNoPartialSnapshotOnFailure(t *testing.T) {
	valid, validRef := syntheticPackage(t, "forms.example.com", "Valid", "0.1.0")
	invalid, invalidRef := syntheticPackage(t, "forms.other.example", "Invalid", "0.1.0")
	invalid.ExpectedDigest = "sha256:" + repeatHex("f")

	input := Input{
		HostAPI:  "forms.takoform.com/v1",
		Packages: []PackageArtifact{valid, invalid},
		DefaultCreates: []DefaultPin{
			{Group: validRef.APIVersion, Kind: validRef.Kind, Ref: validRef},
			{Group: invalidRef.APIVersion, Kind: invalidRef.Kind, Ref: invalidRef},
		},
	}
	snapshot, diagnostics := Compile(input)
	if snapshot != nil {
		t.Fatal("Compile returned a partial Snapshot after an artifact failure")
	}
	if len(diagnostics) == 0 || diagnostics[0].Code != DiagnosticDigestMismatch {
		t.Fatalf("Compile diagnostics = %#v, want leading %q", diagnostics, DiagnosticDigestMismatch)
	}

	reversed := input
	reversed.Packages = []PackageArtifact{invalid, valid}
	_, reversedDiagnostics := Compile(reversed)
	if !reflect.DeepEqual(diagnostics, reversedDiagnostics) {
		t.Fatalf("input permutation changed diagnostics:\nforward=%#v\nreverse=%#v", diagnostics, reversedDiagnostics)
	}
}

func TestSnapshotOwnsDefinitionBytesAndValidatesExactRef(t *testing.T) {
	artifact, ref := syntheticPackage(t, "forms.example.com", "Immutable", "0.1.0")
	snapshot, diagnostics := Compile(Input{
		HostAPI:        "forms.takoform.com/v1",
		Packages:       []PackageArtifact{artifact},
		DefaultCreates: []DefaultPin{{Group: ref.APIVersion, Kind: ref.Kind, Ref: ref}},
	})
	if len(diagnostics) != 0 || snapshot == nil {
		t.Fatalf("Compile = %v, %#v", snapshot != nil, diagnostics)
	}

	inputDefinition := artifact.Package.Definition()
	inputDefinition[0] = '['
	first, ok := snapshot.Definition(ref)
	if !ok || !bytes.HasPrefix(first, []byte("{")) {
		t.Fatalf("Snapshot Definition was mutated through input: %q, %v", first, ok)
	}
	first[0] = '['
	second, ok := snapshot.Definition(ref)
	if !ok || !bytes.HasPrefix(second, []byte("{")) {
		t.Fatalf("Snapshot Definition was mutated through output: %q, %v", second, ok)
	}

	if err := snapshot.Validate(ref, []byte(`{}`)); err != nil {
		t.Fatalf("Validate exact FormRef: %v", err)
	}
	if err := snapshot.Validate(ref, []byte(`{"unexpected":true}`)); err == nil {
		t.Fatal("Validate accepted desired state outside the exact closed schema")
	}
	wrong := ref
	wrong.SchemaDigest = "sha256:" + repeatHex("0")
	if err := snapshot.Validate(wrong, []byte(`{}`)); err == nil {
		t.Fatal("Validate fell back from a wrong exact digest")
	}
}

func TestSnapshotMaterializesPortableDefaultsBeforeValidation(t *testing.T) {
	definition := map[string]any{
		"apiVersion":        "forms.example.com",
		"kind":              "Defaulted",
		"definitionVersion": "0.1.0",
		"title":             "Defaulted",
		"role":              "identity",
		"requiresHostApi":   "forms.takoform.com/v1",
		"desiredSchema": map[string]any{
			"$schema":              "https://json-schema.org/draft/2020-12/schema",
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"enabled": map[string]any{"type": "boolean", "default": true},
				"settings": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"mode": map[string]any{"type": "string", "enum": []string{"safe", "fast"}, "default": "safe"},
					},
					"default": map[string]any{},
				},
			},
		},
		"lifecycleCapabilities": []string{"create", "read", "delete", "import", "observe"},
	}
	artifact, ref := packageFromDefinition(t, definition)
	snapshot, diagnostics := Compile(Input{
		HostAPI:        "forms.takoform.com/v1",
		Packages:       []PackageArtifact{artifact},
		DefaultCreates: []DefaultPin{{Group: ref.APIVersion, Kind: ref.Kind, Ref: ref}},
	})
	if len(diagnostics) != 0 || snapshot == nil {
		t.Fatalf("Compile = %v, %#v", snapshot != nil, diagnostics)
	}

	omitted, err := snapshot.Materialize(ref, []byte(`{}`))
	if err != nil {
		t.Fatalf("Materialize omitted defaults: %v", err)
	}
	explicit, err := snapshot.Materialize(ref, []byte(`{"enabled":true,"settings":{"mode":"safe"}}`))
	if err != nil {
		t.Fatalf("Materialize explicit defaults: %v", err)
	}
	if !bytes.Equal(omitted, explicit) {
		t.Fatalf("omitted and explicit defaults differ:\nomitted=%s\nexplicit=%s", omitted, explicit)
	}
	if !bytes.Equal(omitted, []byte(`{"enabled":true,"settings":{"mode":"safe"}}`)) {
		t.Fatalf("Materialize = %s", omitted)
	}
	if _, err := snapshot.Materialize(ref, []byte(`{"unexpected":true}`)); err == nil {
		t.Fatal("Materialize accepted desired state outside the exact schema")
	}
}

func TestCompileClosesCrossFamilyExactReferences(t *testing.T) {
	target, targetRef := syntheticPackage(t, "target.forms.example", "SharedKind", "0.1.0")
	source, sourceRef := syntheticReferencePackage(t, "source.forms.example", "Consumer", "0.1.0", targetRef)

	snapshot, diagnostics := Compile(Input{
		HostAPI:  "forms.takoform.com/v1",
		Packages: []PackageArtifact{source, target},
		DefaultCreates: []DefaultPin{
			{Group: sourceRef.APIVersion, Kind: sourceRef.Kind, Ref: sourceRef},
			{Group: targetRef.APIVersion, Kind: targetRef.Kind, Ref: targetRef},
		},
	})
	if len(diagnostics) != 0 || snapshot == nil {
		t.Fatalf("complete cross-family graph did not compile: snapshot=%v diagnostics=%#v", snapshot != nil, diagnostics)
	}

	snapshot, diagnostics = Compile(Input{
		HostAPI:        "forms.takoform.com/v1",
		Packages:       []PackageArtifact{source},
		DefaultCreates: []DefaultPin{{Group: sourceRef.APIVersion, Kind: sourceRef.Kind, Ref: sourceRef}},
	})
	if snapshot != nil {
		t.Fatal("Compile returned a Snapshot with an unresolved exact target")
	}
	if len(diagnostics) != 1 || diagnostics[0].Code != DiagnosticUnresolvedReference {
		t.Fatalf("unresolved target diagnostics = %#v", diagnostics)
	}
}

func syntheticPackage(t *testing.T, group, kind, version string) (PackageArtifact, formpackage.FormRef) {
	t.Helper()
	definition := map[string]any{
		"apiVersion":        group,
		"kind":              kind,
		"definitionVersion": version,
		"title":             kind,
		"role":              "identity",
		"requiresHostApi":   "forms.takoform.com/v1",
		"desiredSchema": map[string]any{
			"$schema":              "https://json-schema.org/draft/2020-12/schema",
			"type":                 "object",
			"additionalProperties": false,
			"properties":           map[string]any{},
		},
		"lifecycleCapabilities": []string{"create", "read", "delete", "import", "observe"},
	}
	return packageFromDefinition(t, definition)
}

func syntheticReferencePackage(t *testing.T, group, kind, version string, target formpackage.FormRef) (PackageArtifact, formpackage.FormRef) {
	t.Helper()
	definition := map[string]any{
		"apiVersion":        group,
		"kind":              kind,
		"definitionVersion": version,
		"title":             kind,
		"role":              "attachment",
		"requiresHostApi":   "forms.takoform.com/v1",
		"desiredSchema": map[string]any{
			"$schema":              "https://json-schema.org/draft/2020-12/schema",
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"parent": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"apiVersion": map[string]any{"type": "string", "const": target.APIVersion},
						"kind":       map[string]any{"type": "string", "const": target.Kind},
						"name":       map[string]any{"type": "string", "pattern": "^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$"},
					},
					"required":                   []string{"apiVersion", "kind", "name"},
					"x-takoform-target-formrefs": []formpackage.FormRef{target},
				},
			},
			"required": []string{"parent"},
		},
		"immutableFields":       []string{"/parent"},
		"lifecycleCapabilities": []string{"create", "read", "delete", "import", "observe"},
	}
	return packageFromDefinition(t, definition)
}

func packageFromDefinition(t *testing.T, definition map[string]any) (PackageArtifact, formpackage.FormRef) {
	t.Helper()
	definitionRaw, err := json.Marshal(definition)
	if err != nil {
		t.Fatalf("marshal synthetic Definition: %v", err)
	}
	schemaDigest, err := formpackage.DigestCanonicalJSON(definitionRaw)
	if err != nil {
		t.Fatalf("digest synthetic Definition: %v", err)
	}
	ref := formpackage.FormRef{
		APIVersion:        definition["apiVersion"].(string),
		Kind:              definition["kind"].(string),
		DefinitionVersion: definition["definitionVersion"].(string),
		SchemaDigest:      schemaDigest,
	}
	index := formpackage.PackageIndex{
		APIVersion:     formpackage.VersionlessFamilyPackageAPIVersion,
		Kind:           formpackage.PackageKind,
		FormRef:        ref,
		DefinitionPath: "definition.json",
		Files: []formpackage.PackageFile{{
			Path:      "definition.json",
			MediaType: formpackage.DefinitionMediaType,
			Size:      int64(len(definitionRaw)),
			Digest:    formpackage.DigestBytes(definitionRaw),
		}},
	}
	indexRaw, err := json.Marshal(index)
	if err != nil {
		t.Fatalf("marshal synthetic package index: %v", err)
	}
	packageDigest, err := formpackage.DigestCanonicalJSON(indexRaw)
	if err != nil {
		t.Fatalf("digest synthetic package index: %v", err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "definition.json"), definitionRaw, 0o600); err != nil {
		t.Fatalf("write synthetic Definition: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, formpackage.PackageIndexFilename), indexRaw, 0o600); err != nil {
		t.Fatalf("write synthetic package index: %v", err)
	}
	report, err := formpackage.VerifyDirectory(root)
	if err != nil {
		t.Fatalf("verify complete synthetic package: %v", err)
	}
	verified, ok := report.VerifiedPackage()
	if !ok {
		t.Fatal("complete synthetic package verification issued no package")
	}
	return PackageArtifact{
		Origin:         "test://" + ref.APIVersion + "/" + ref.Kind + "@" + ref.DefinitionVersion,
		ExpectedDigest: packageDigest,
		Package:        verified,
	}, ref
}

func repeatHex(value string) string {
	var output bytes.Buffer
	for range 64 {
		output.WriteString(value)
	}
	return output.String()
}

func reverseCopy[T any](input []T) []T {
	output := append([]T(nil), input...)
	for left, right := 0, len(output)-1; left < right; left, right = left+1, right-1 {
		output[left], output[right] = output[right], output[left]
	}
	return output
}
