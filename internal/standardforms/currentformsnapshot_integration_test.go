package standardforms_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformsnapshot"
)

type familyIndexEntry struct {
	Group        string `json:"group"`
	CandidateSet string `json:"candidateSet"`
	SHA256       string `json:"sha256"`
	FormCount    int    `json:"formCount"`
}

type artifactPointer struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type familyIndex struct {
	Format                string             `json:"format"`
	Families              []familyIndexEntry `json:"families"`
	InterfaceCandidateSet artifactPointer    `json:"interfaceCandidateSet"`
	BindingCandidateSet   artifactPointer    `json:"bindingCandidateSet"`
}

type familyCandidate struct {
	Kind          string              `json:"kind"`
	Role          string              `json:"role"`
	Path          string              `json:"path"`
	FormRef       formpackage.FormRef `json:"formRef"`
	PackageDigest string              `json:"packageDigest"`
}

type familyCandidateSet struct {
	Format            string            `json:"format"`
	Family            string            `json:"family"`
	FormMaturity      string            `json:"formMaturity"`
	PackageAPIVersion string            `json:"packageApiVersion"`
	PublicationStatus string            `json:"publicationStatus"`
	AuthoringSource   string            `json:"authoringSource"`
	AuthoringPolicy   string            `json:"authoringPolicy"`
	Forms             []familyCandidate `json:"forms"`
}

type contractCandidate struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	SchemaDigest string `json:"schemaDigest"`
}

type interfaceCandidateSet struct {
	Format            string              `json:"format"`
	PublicationStatus string              `json:"publicationStatus"`
	AuthoringSource   string              `json:"authoringSource"`
	Interfaces        []contractCandidate `json:"interfaces"`
}

type bindingCandidateSet struct {
	Format            string              `json:"format"`
	PublicationStatus string              `json:"publicationStatus"`
	AuthoringSource   string              `json:"authoringSource"`
	Bindings          []contractCandidate `json:"bindings"`
}

func TestOfficialArtifactsCompileThroughNeutralCore(t *testing.T) {
	input, expected, repositoryRoot := loadOfficialInput(t)
	forward, diagnostics := currentformsnapshot.Compile(input)
	if len(diagnostics) != 0 || forward == nil {
		t.Fatalf("official artifacts did not compile: snapshot=%v diagnostics=%#v", forward != nil, diagnostics)
	}
	if got := forward.Forms(); len(got) != 31 || len(got) != len(expected) {
		t.Fatalf("compiled official Form count = %d, inventory = %d, want 31", len(got), len(expected))
	} else {
		for _, form := range got {
			if _, ok := expected[form.Ref]; !ok {
				t.Fatalf("compiled Snapshot added unknown official Form %#v", form.Ref)
			}
		}
	}
	if got := len(forward.Interfaces()); got != 13 {
		t.Fatalf("compiled official Interface count = %d, want 13", got)
	}
	if got := len(forward.Bindings()); got != 6 {
		t.Fatalf("compiled official Binding count = %d, want 6", got)
	}

	reversed := input
	reversed.Packages = reverseCopy(input.Packages)
	reversed.Interfaces = reverseCopy(input.Interfaces)
	reversed.Bindings = reverseCopy(input.Bindings)
	reversed.DefaultCreates = reverseCopy(input.DefaultCreates)
	backward, reversedDiagnostics := currentformsnapshot.Compile(reversed)
	if len(reversedDiagnostics) != 0 || backward == nil {
		t.Fatalf("reversed official artifacts did not compile: snapshot=%v diagnostics=%#v", backward != nil, reversedDiagnostics)
	}
	if forward.Digest() != backward.Digest() || !reflect.DeepEqual(forward.Forms(), backward.Forms()) ||
		!reflect.DeepEqual(forward.Interfaces(), backward.Interfaces()) || !reflect.DeepEqual(forward.Bindings(), backward.Bindings()) {
		t.Fatalf("official artifact order changed Snapshot: %s != %s", forward.Digest(), backward.Digest())
	}

	for ref, path := range expected {
		if ref.APIVersion != "table.forms.takoform.com" || ref.Kind != "Table" {
			continue
		}
		desired, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(path), "fixtures", "desired.json"))
		if err != nil {
			t.Fatalf("read current Table fixture: %v", err)
		}
		if err := forward.Validate(ref, desired); err != nil {
			t.Fatalf("current non-Edge Table fixture: %v", err)
		}
		return
	}
	t.Fatal("official inventory contains no Table Form")
}

func TestExternalPublisherUsesTheSameVerifiedPackagePath(t *testing.T) {
	input, _, _ := loadOfficialInput(t)
	external, externalRef := externalPackage(t)
	input.Packages = append(input.Packages, external)
	input.DefaultCreates = append(input.DefaultCreates, currentformsnapshot.DefaultPin{
		Group: externalRef.APIVersion, Kind: externalRef.Kind, Ref: externalRef,
	})

	snapshot, diagnostics := currentformsnapshot.Compile(input)
	if len(diagnostics) != 0 || snapshot == nil {
		t.Fatalf("official plus external artifacts did not compile: snapshot=%v diagnostics=%#v", snapshot != nil, diagnostics)
	}
	if got := len(snapshot.Forms()); got != 32 {
		t.Fatalf("official plus external Form count = %d, want 32", got)
	}
	if _, ok := snapshot.Definition(externalRef); !ok {
		t.Fatal("external exact FormRef is absent from the common Snapshot")
	}
	if got, ok := snapshot.Default(externalRef.APIVersion, externalRef.Kind); !ok || got != externalRef {
		t.Fatalf("external default = %#v, %v; want %#v", got, ok, externalRef)
	}
}

func loadOfficialInput(t *testing.T) (currentformsnapshot.Input, map[formpackage.FormRef]string, string) {
	t.Helper()
	repositoryRoot := filepath.Join("..", "..")
	indexRaw, err := os.ReadFile(filepath.Join(repositoryRoot, "forms", "candidates", "current-family-index.json"))
	if err != nil {
		t.Fatalf("read current family index: %v", err)
	}
	var current familyIndex
	if err := formpackage.DecodeStrictIJSON(indexRaw, &current); err != nil {
		t.Fatalf("decode current family index: %v", err)
	}
	if current.Format != "takoform.current-family-index@v1" || len(current.Families) != 8 {
		t.Fatalf("current family index = %#v", current)
	}

	interfaces, bindings := loadCurrentContractArtifacts(t, repositoryRoot, current)
	input := currentformsnapshot.Input{HostAPI: "forms.takoform.com/v1", Interfaces: interfaces, Bindings: bindings}
	expected := make(map[formpackage.FormRef]string)
	for _, family := range current.Families {
		candidateRaw, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(family.CandidateSet)))
		if err != nil {
			t.Fatalf("read %s: %v", family.CandidateSet, err)
		}
		if digest := fmt.Sprintf("%x", sha256.Sum256(candidateRaw)); digest != family.SHA256 {
			t.Fatalf("%s digest = %s, index says %s", family.CandidateSet, digest, family.SHA256)
		}
		var candidates familyCandidateSet
		if err := formpackage.DecodeStrictIJSON(candidateRaw, &candidates); err != nil {
			t.Fatalf("decode %s: %v", family.CandidateSet, err)
		}
		if candidates.Format != "takoform.form-family-candidates@v1" || candidates.Family != family.Group || len(candidates.Forms) != family.FormCount {
			t.Fatalf("candidate set %s does not match family index: %#v", family.CandidateSet, candidates)
		}
		for _, candidate := range candidates.Forms {
			packageRoot := filepath.Join(repositoryRoot, filepath.FromSlash(candidate.Path))
			report, err := formpackage.VerifyDirectory(packageRoot)
			if err != nil {
				t.Fatalf("verify complete package %s: %v", candidate.Path, err)
			}
			verified, ok := report.VerifiedPackage()
			if !ok {
				t.Fatalf("complete package verification issued no package for %s", candidate.Path)
			}
			if verified.FormRef() != candidate.FormRef || verified.PackageDigest() != candidate.PackageDigest {
				t.Fatalf("%s candidate identity drift: package=%#v/%s candidate=%#v/%s", candidate.Path, verified.FormRef(), verified.PackageDigest(), candidate.FormRef, candidate.PackageDigest)
			}
			if prior, duplicate := expected[candidate.FormRef]; duplicate {
				t.Fatalf("exact FormRef %#v appears at both %s and %s", candidate.FormRef, prior, candidate.Path)
			}
			expected[candidate.FormRef] = candidate.Path
			input.Packages = append(input.Packages, currentformsnapshot.PackageArtifact{
				Origin: "repo://" + candidate.Path, ExpectedDigest: candidate.PackageDigest, Package: verified,
			})
			input.DefaultCreates = append(input.DefaultCreates, currentformsnapshot.DefaultPin{
				Group: candidate.FormRef.APIVersion, Kind: candidate.FormRef.Kind, Ref: candidate.FormRef,
			})
		}
	}
	return input, expected, repositoryRoot
}

func loadCurrentContractArtifacts(t *testing.T, repositoryRoot string, current familyIndex) ([]currentformsnapshot.InterfaceArtifact, []currentformsnapshot.BindingArtifact) {
	t.Helper()
	interfaceRaw, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(current.InterfaceCandidateSet.Path)))
	if err != nil {
		t.Fatalf("read Interface candidate set: %v", err)
	}
	if digest := fmt.Sprintf("%x", sha256.Sum256(interfaceRaw)); digest != current.InterfaceCandidateSet.SHA256 {
		t.Fatalf("Interface candidate set digest = %s, index says %s", digest, current.InterfaceCandidateSet.SHA256)
	}
	var interfaceCandidates interfaceCandidateSet
	if err := formpackage.DecodeStrictIJSON(interfaceRaw, &interfaceCandidates); err != nil {
		t.Fatalf("decode Interface candidate set: %v", err)
	}
	interfaceRoot := filepath.Dir(filepath.Join(repositoryRoot, filepath.FromSlash(current.InterfaceCandidateSet.Path)))
	interfaces := make([]currentformsnapshot.InterfaceArtifact, 0, len(interfaceCandidates.Interfaces))
	for _, candidate := range interfaceCandidates.Interfaces {
		raw, err := os.ReadFile(filepath.Join(interfaceRoot, candidate.Name, "definition.json"))
		if err != nil {
			t.Fatalf("read Interface %s: %v", candidate.Name, err)
		}
		interfaces = append(interfaces, currentformsnapshot.InterfaceArtifact{Origin: "repo://interfaces/" + candidate.Name, ExpectedDigest: candidate.SchemaDigest, Definition: raw})
	}

	bindingRaw, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(current.BindingCandidateSet.Path)))
	if err != nil {
		t.Fatalf("read Binding candidate set: %v", err)
	}
	if digest := fmt.Sprintf("%x", sha256.Sum256(bindingRaw)); digest != current.BindingCandidateSet.SHA256 {
		t.Fatalf("Binding candidate set digest = %s, index says %s", digest, current.BindingCandidateSet.SHA256)
	}
	var bindingCandidates bindingCandidateSet
	if err := formpackage.DecodeStrictIJSON(bindingRaw, &bindingCandidates); err != nil {
		t.Fatalf("decode Binding candidate set: %v", err)
	}
	bindingRoot := filepath.Dir(filepath.Join(repositoryRoot, filepath.FromSlash(current.BindingCandidateSet.Path)))
	bindings := make([]currentformsnapshot.BindingArtifact, 0, len(bindingCandidates.Bindings))
	for _, candidate := range bindingCandidates.Bindings {
		raw, err := os.ReadFile(filepath.Join(bindingRoot, candidate.Name, "definition.json"))
		if err != nil {
			t.Fatalf("read Binding %s: %v", candidate.Name, err)
		}
		bindings = append(bindings, currentformsnapshot.BindingArtifact{Origin: "repo://bindings/" + candidate.Name, ExpectedDigest: candidate.SchemaDigest, Definition: raw})
	}
	return interfaces, bindings
}

func externalPackage(t *testing.T) (currentformsnapshot.PackageArtifact, formpackage.FormRef) {
	t.Helper()
	definition := map[string]any{
		"apiVersion": "forms.external.example", "kind": "ExternalStore", "definitionVersion": "1.0.0",
		"title": "External Store", "role": "identity", "requiresHostApi": "forms.takoform.com/v1",
		"desiredSchema": map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object",
			"additionalProperties": false, "properties": map[string]any{},
		},
		"lifecycleCapabilities": []string{"create", "read", "delete", "import", "observe"},
	}
	definitionRaw, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	schemaDigest, err := formpackage.DigestCanonicalJSON(definitionRaw)
	if err != nil {
		t.Fatal(err)
	}
	ref := formpackage.FormRef{APIVersion: "forms.external.example", Kind: "ExternalStore", DefinitionVersion: "1.0.0", SchemaDigest: schemaDigest}
	index := formpackage.PackageIndex{
		APIVersion: formpackage.VersionlessFamilyPackageAPIVersion, Kind: formpackage.PackageKind,
		FormRef: ref, DefinitionPath: "definition.json",
		Files: []formpackage.PackageFile{{Path: "definition.json", MediaType: formpackage.DefinitionMediaType, Size: int64(len(definitionRaw)), Digest: formpackage.DigestBytes(definitionRaw)}},
	}
	indexRaw, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "definition.json"), definitionRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, formpackage.PackageIndexFilename), indexRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := formpackage.VerifyDirectory(root)
	if err != nil {
		t.Fatalf("verify external publisher package: %v", err)
	}
	verified, ok := report.VerifiedPackage()
	if !ok {
		t.Fatal("verified external package was not issued")
	}
	return currentformsnapshot.PackageArtifact{Origin: "external://forms.external.example/ExternalStore", ExpectedDigest: verified.PackageDigest(), Package: verified}, ref
}

func reverseCopy[T any](input []T) []T {
	output := append([]T(nil), input...)
	for left, right := 0, len(output)-1; left < right; left, right = left+1, right-1 {
		output[left], output[right] = output[right], output[left]
	}
	return output
}
