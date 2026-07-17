package providerlifecycle

import (
	"strings"
	"testing"
)

func TestLoadCLIMatrixPinsDistinctCanonicalAddresses(t *testing.T) {
	root, err := RepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	requirements, descriptorDigest, err := LoadCLIMatrix(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(requirements) != 2 || !validDigest(descriptorDigest) {
		t.Fatalf("unexpected matrix identity: %#v %q", requirements, descriptorDigest)
	}
	seen := map[string]CLIRequirement{}
	for _, requirement := range requirements {
		seen[requirement.Product] = requirement
	}
	if seen["OpenTofu"].Version != "1.12.1" || seen["OpenTofu"].ProviderAddress != OpenTofuProviderAddress {
		t.Fatalf("unexpected OpenTofu matrix entry: %#v", seen["OpenTofu"])
	}
	if seen["Terraform"].Version != "1.15.8" || seen["Terraform"].ProviderAddress != TerraformProviderAddress {
		t.Fatalf("unexpected Terraform matrix entry: %#v", seen["Terraform"])
	}
}

func TestStackConfigUsesExactCLIProviderAddress(t *testing.T) {
	openTofu := stackConfig("https://forms.example.test", OpenTofuProviderAddress, 1)
	if !strings.Contains(openTofu, `source = "`+OpenTofuProviderAddress+`"`) || strings.Contains(openTofu, TerraformProviderAddress) {
		t.Fatalf("OpenTofu config did not retain its exact FQN:\n%s", openTofu)
	}
	terra := stackConfig("https://forms.example.test", TerraformProviderAddress, 1)
	if !strings.Contains(terra, `source = "`+TerraformProviderAddress+`"`) || strings.Contains(terra, OpenTofuProviderAddress) {
		t.Fatalf("Terraform config did not retain its exact FQN:\n%s", terra)
	}
}

func TestValidateMatrixRejectsAddressAliasingAndEvidenceDrift(t *testing.T) {
	requirements := []CLIRequirement{
		{Product: "OpenTofu", Version: "1.12.1", ProviderAddress: OpenTofuProviderAddress},
		{Product: "Terraform", Version: "1.15.8", ProviderAddress: TerraformProviderAddress},
	}
	openTofu := completeReport("OpenTofu", "1.12.1", OpenTofuProviderAddress)
	terra := completeReport("Terraform", "1.15.8", TerraformProviderAddress)
	matrix := MatrixReport{
		Format: MatrixReportFormat, Classification: "supported-cli-fqn-candidate-matrix", PublicationReady: false,
		ReleaseDescriptorSHA256: "sha256:" + strings.Repeat("a", 64),
		CandidateSetSHA256:      candidateSetSHA256(), ProviderSchemaSHA256: "sha256:" + strings.Repeat("b", 64),
		Reports: []Report{openTofu, terra},
	}
	if err := ValidateMatrix(matrix, requirements); err != nil {
		t.Fatalf("valid matrix: %v", err)
	}

	aliased := matrix
	aliased.Reports = append([]Report(nil), matrix.Reports...)
	aliased.Reports[0].CLI.ProviderAddress = TerraformProviderAddress
	if err := ValidateMatrix(aliased, requirements); err == nil {
		t.Fatal("matrix accepted an aliased OpenTofu provider address")
	}

	drifted := matrix
	drifted.Reports = append([]Report(nil), matrix.Reports...)
	drifted.Reports[1].Resources = append([]ResourceEvidence(nil), matrix.Reports[1].Resources...)
	drifted.Reports[1].Resources[0].Checks.Delete = false
	if err := ValidateMatrix(drifted, requirements); err == nil {
		t.Fatal("matrix accepted divergent lifecycle evidence")
	}
}

func completeReport(product, version, address string) Report {
	checks := CheckEvidence{Create: true, Read: true, Update: true, Observe: true, Refresh: true, NativeImport: true, CLIImport: true, Delete: true, DriftState: true, NameReplace: true}
	resources := make([]ResourceEvidence, 0, len(resourceCases))
	immutable := make([]ImmutableReplaceEvidence, 0, len(resourceCases)+2)
	for _, item := range resourceCases {
		resources = append(resources, ResourceEvidence{Kind: item.Kind, ResourceType: item.ResourceType, Checks: checks})
		immutable = append(immutable, ImmutableReplaceEvidence{Kind: item.Kind, Field: "/name", Passed: true})
	}
	immutable = append(immutable,
		ImmutableReplaceEvidence{Kind: "SQLDatabase", Field: "/engine", Passed: true},
		ImmutableReplaceEvidence{Kind: "VectorIndex", Field: "/dimensions", Passed: true},
	)
	return Report{
		Format: ReportFormat, Classification: "generic-lifecycle-candidate", PublicationReady: false,
		BindingStatus: "exact-structural-candidate-set", RunnerSubject: RunnerSubject,
		Protocol:           "Terraform provider protocol v6 + versioned Form host HTTP",
		CandidateSetSHA256: candidateSetSHA256(), ProviderSchemaSHA256: "sha256:" + strings.Repeat("b", 64),
		ProviderBinary: ProviderBinaryIdentity{Version: "0.1.0-rc.1", SHA256: "sha256:" + strings.Repeat("d", 64)},
		CLI:            CLIIdentity{Product: product, Version: version, ProviderAddress: address, ExecutableName: strings.ToLower(product), ExecutableSHA256: "sha256:" + strings.Repeat("c", 64)},
		Resources:      resources,
		NegativeChecks: []NegativeEvidence{
			{Name: "response-name-substitution-rejected", Kind: "ObjectBucket", Fixture: "fixture", Passed: true},
			{Name: "response-package-digest-substitution-rejected", Kind: "KVStore", Fixture: "fixture", Passed: true},
		},
		ImmutableReplace: immutable,
	}
}
