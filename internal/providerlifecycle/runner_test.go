package providerlifecycle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCLIMatrixPinsCanonicalAndDualPublishedAddresses(t *testing.T) {
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
	if CanonicalProviderAddress != TerraformProviderAddress || OpenTofuProviderAddress == CanonicalProviderAddress {
		t.Fatalf("provider distribution identities collapsed: canonical=%q opentofu=%q", CanonicalProviderAddress, OpenTofuProviderAddress)
	}
	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"dual-publishes", "distinct state identities", "state replace-provider"} {
		if !strings.Contains(string(readme), required) {
			t.Fatalf("provider distribution guidance lacks %q", required)
		}
	}
}

func TestStackConfigUsesExactCLIProviderAddress(t *testing.T) {
	openTofu := stackConfig("https://forms.example.test", OpenTofuProviderAddress, "1.0.0", 1)
	if !strings.Contains(openTofu, `source = "`+OpenTofuProviderAddress+`"`) || !strings.Contains(openTofu, `version = "1.0.0"`) || strings.Contains(openTofu, TerraformProviderAddress) {
		t.Fatalf("OpenTofu config did not retain its exact FQN:\n%s", openTofu)
	}
	terra := stackConfig("https://forms.example.test", TerraformProviderAddress, "1.0.0", 1)
	if !strings.Contains(terra, `source = "`+TerraformProviderAddress+`"`) || strings.Contains(terra, OpenTofuProviderAddress) {
		t.Fatalf("Terraform config did not retain its exact FQN:\n%s", terra)
	}
}

func TestFindInstalledProviderBinaryRequiresOneExecutableRegularFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	providerDir := filepath.Join(root, ".terraform", "providers", "registry.example.test", "tako0614", "takoform", "1.0.0", "linux_amd64")
	if err := os.MkdirAll(providerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(providerDir, "terraform-provider-takoform_v1.0.0")
	if err := os.WriteFile(binary, []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := findInstalledProviderBinary(root, "1.0.0")
	if err != nil || got != binary {
		t.Fatalf("installed binary = %q, %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(providerDir, "terraform-provider-takoform_duplicate"), []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := findInstalledProviderBinary(root, "1.0.0"); err == nil || !strings.Contains(err.Error(), "2 provider binaries") {
		t.Fatalf("duplicate binary error = %v", err)
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
		InstallationSource:      LocalDevOverride,
		ReleaseDescriptorSHA256: "sha256:" + strings.Repeat("a", 64),
		CandidateSetSHA256:      candidateSetSHA256(), ProviderSchemaSHA256: "sha256:" + strings.Repeat("b", 64),
		Reports: []Report{openTofu, terra},
	}
	if err := ValidateMatrix(matrix, requirements); err != nil {
		t.Fatalf("valid matrix: %v", err)
	}
	if err := ValidateRegistryMatrix(matrix, requirements); err == nil {
		t.Fatal("Registry matrix accepted local dev-override evidence")
	}
	registry := matrix
	registry.InstallationSource = DirectRegistryInstall
	registry.Reports = append([]Report(nil), matrix.Reports...)
	for index := range registry.Reports {
		registry.Reports[index].InstallationSource = DirectRegistryInstall
	}
	if err := ValidateRegistryMatrix(registry, requirements); err != nil {
		t.Fatalf("valid direct Registry matrix: %v", err)
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

	duplicateResource := registry
	duplicateResource.Reports = append([]Report(nil), registry.Reports...)
	duplicateResource.Reports[0].Resources = append([]ResourceEvidence(nil), registry.Reports[0].Resources...)
	duplicateResource.Reports[0].Resources[1] = duplicateResource.Reports[0].Resources[0]
	if err := ValidateRegistryMatrix(duplicateResource, requirements); err == nil {
		t.Fatal("Registry matrix accepted a duplicated resource identity")
	}

	unknownNegative := registry
	unknownNegative.Reports = append([]Report(nil), registry.Reports...)
	unknownNegative.Reports[0].NegativeChecks = append([]NegativeEvidence(nil), registry.Reports[0].NegativeChecks...)
	unknownNegative.Reports[0].NegativeChecks[0].Name = "unreviewed-negative"
	if err := ValidateRegistryMatrix(unknownNegative, requirements); err == nil {
		t.Fatal("Registry matrix accepted an unreviewed negative fixture")
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
		Protocol: providerProtocol, InstallationSource: LocalDevOverride,
		CandidateSetSHA256: candidateSetSHA256(), ProviderSchemaSHA256: "sha256:" + strings.Repeat("b", 64),
		ProviderBinary: ProviderBinaryIdentity{Version: "0.1.0-rc.2", SHA256: "sha256:" + strings.Repeat("d", 64)},
		CLI:            CLIIdentity{Product: product, Version: version, ProviderAddress: address, ExecutableName: strings.ToLower(product), ExecutableSHA256: "sha256:" + strings.Repeat("c", 64)},
		Resources:      resources,
		NegativeChecks: []NegativeEvidence{
			{Name: "response-name-substitution-rejected", Kind: "ObjectBucket", Fixture: "fixture", Passed: true},
			{Name: "response-package-digest-substitution-rejected", Kind: "KVStore", Fixture: "fixture", Passed: true},
		},
		ImmutableReplace: immutable,
	}
}
