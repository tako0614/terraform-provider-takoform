package formpackage

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestVerificationReportIssuesOnlyVerifiedPackageAfterSuccessfulVerification(t *testing.T) {
	t.Parallel()

	zero, ok := (VerificationReport{}).VerifiedPackage()
	if ok || zero.Valid() {
		t.Fatalf("zero VerificationReport issued package = %#v, %v", zero, ok)
	}

	root := makeValidPackage(t, nil)
	report, err := VerifyDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	verified, ok := report.VerifiedPackage()
	if !ok || !verified.Valid() {
		t.Fatalf("successful VerifyDirectory did not issue a valid package: %#v, %v", verified, ok)
	}

	// The public report fields are descriptive evidence, not an issuance
	// mechanism. Reconstructing the same visible report must not mint the
	// capability, and JSON round-tripping must not preserve it either.
	forged := VerificationReport{
		PackageDigest: report.PackageDigest,
		FormRef:       report.FormRef,
		FileCount:     report.FileCount,
		PayloadBytes:  report.PayloadBytes,
	}
	if candidate, ok := forged.VerifiedPackage(); ok || candidate.Valid() {
		t.Fatalf("manually constructed report issued package = %#v, %v", candidate, ok)
	}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	wantFields := map[string]struct{}{
		"packageDigest": {},
		"formRef":       {},
		"fileCount":     {},
		"payloadBytes":  {},
	}
	if len(fields) != len(wantFields) {
		t.Fatalf("VerificationReport JSON fields = %#v, want exactly %#v", fields, wantFields)
	}
	for field := range wantFields {
		if _, ok := fields[field]; !ok {
			t.Fatalf("VerificationReport JSON omitted %q: %#v", field, fields)
		}
	}
	var decoded VerificationReport
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if candidate, ok := decoded.VerifiedPackage(); ok || candidate.Valid() {
		t.Fatalf("JSON-decoded report issued package = %#v, %v", candidate, ok)
	}
}

func TestVerifiedPackageReturnsCanonicalOwnedDataAndDefensiveCopies(t *testing.T) {
	t.Parallel()

	root := makeValidPackage(t, nil)
	definitionPath := filepath.Join(root, "definition.json")
	definitionRaw, err := os.ReadFile(definitionPath)
	if err != nil {
		t.Fatal(err)
	}
	var definitionValue any
	if err := json.Unmarshal(definitionRaw, &definitionValue); err != nil {
		t.Fatal(err)
	}
	nonCanonicalDefinition, err := json.MarshalIndent(definitionValue, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(nonCanonicalDefinition, definitionRaw) {
		t.Fatal("test definition unexpectedly remained canonical after indentation")
	}
	writeFixtureFile(t, definitionPath, nonCanonicalDefinition, 0o644)
	mutateIndex(t, root, func(index map[string]any) {
		for _, value := range index["files"].([]any) {
			file := value.(map[string]any)
			if file["path"] == "definition.json" {
				file["size"] = len(nonCanonicalDefinition)
				file["digest"] = DigestBytes(nonCanonicalDefinition)
			}
		}
	})
	report, err := VerifyDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	verified, ok := report.VerifiedPackage()
	if !ok {
		t.Fatal("successful verification did not issue a package")
	}

	if got := verified.PackageDigest(); got != report.PackageDigest {
		t.Fatalf("PackageDigest() = %q, want %q", got, report.PackageDigest)
	}
	if got := verified.FormRef(); got != report.FormRef {
		t.Fatalf("FormRef() = %#v, want %#v", got, report.FormRef)
	}

	wantIndex, err := readAndValidateIndexForTest(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := verified.PackageIndex(); !reflect.DeepEqual(got, wantIndex) {
		t.Fatalf("PackageIndex() = %#v, want %#v", got, wantIndex)
	}
	definitionRaw, err = readPackageFileForTest(root, wantIndex.DefinitionPath)
	if err != nil {
		t.Fatal(err)
	}
	wantDefinition, err := Canonicalize(definitionRaw)
	if err != nil {
		t.Fatal(err)
	}
	if got := verified.Definition(); !bytes.Equal(got, wantDefinition) {
		t.Fatalf("Definition() = %q, want canonical %q", got, wantDefinition)
	}

	files := verified.Files()
	if !reflect.DeepEqual(files, wantIndex.Files) {
		t.Fatalf("Files() = %#v, want %#v", files, wantIndex.Files)
	}
	for _, file := range files {
		payload, found := verified.Payload(file.Path)
		if !found {
			t.Fatalf("Payload(%q) was not found", file.Path)
		}
		want, err := readPackageFileForTest(root, file.Path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(payload, want) {
			t.Fatalf("Payload(%q) = %q, want %q", file.Path, payload, want)
		}
	}
	if payload, found := verified.Payload("missing"); found || payload != nil {
		t.Fatalf("Payload(missing) = %q, %v; want nil, false", payload, found)
	}

	// Every aggregate and byte view is copied at the public seam.
	index := verified.PackageIndex()
	index.FormRef.Kind = "Mutated"
	index.Files[0].Path = "mutated"
	if got := verified.PackageIndex(); got.FormRef.Kind == "Mutated" || got.Files[0].Path == "mutated" {
		t.Fatal("PackageIndex() exposed mutable internal state")
	}
	definition := verified.Definition()
	definition[0] ^= 0xff
	if got := verified.Definition(); bytes.Equal(got, definition) {
		t.Fatal("Definition() exposed mutable internal state")
	}
	files[0].Path = "mutated"
	if got := verified.Files(); got[0].Path == "mutated" {
		t.Fatal("Files() exposed mutable internal state")
	}
	payload, found := verified.Payload(wantIndex.Files[0].Path)
	if !found {
		t.Fatal("payload disappeared after defensive-copy checks")
	}
	payload[0] ^= 0xff
	if got, _ := verified.Payload(wantIndex.Files[0].Path); bytes.Equal(got, payload) {
		t.Fatal("Payload() exposed mutable internal state")
	}
}

func readAndValidateIndexForTest(root string) (PackageIndex, error) {
	raw, err := readPackageFileForTest(root, PackageIndexFilename)
	if err != nil {
		return PackageIndex{}, err
	}
	return ValidatePackageIndex(raw)
}

func readPackageFileForTest(root, relative string) ([]byte, error) {
	return os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
}
