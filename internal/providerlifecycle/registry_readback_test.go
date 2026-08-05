package providerlifecycle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildRegistryReadbackAcceptsCurrentProviderEpoch(t *testing.T) {
	root, err := RepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	requirements, descriptorDigest, err := LoadCLIMatrix(root)
	if err != nil {
		t.Fatal(err)
	}
	providerVersion, err := loadProviderVersion(root)
	if err != nil {
		t.Fatal(err)
	}
	reports := make([]Report, 0, len(requirements))
	for _, requirement := range requirements {
		report := completeReport(requirement.Product, requirement.Version, requirement.ProviderAddress)
		report.InstallationSource = DirectRegistryInstall
		report.ProviderBinary.Version = providerVersion
		reports = append(reports, report)
	}
	matrix := MatrixReport{
		Format: MatrixReportFormat, Classification: "supported-cli-fqn-candidate-matrix", PublicationReady: false,
		ReleaseDescriptorSHA256: descriptorDigest, CandidateSetSHA256: candidateSetSHA256(),
		ProviderSchemaSHA256: reports[0].ProviderSchemaSHA256, InstallationSource: DirectRegistryInstall,
		Reports: reports,
	}
	raw, err := json.MarshalIndent(matrix, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	matrixPath := filepath.Join(t.TempDir(), "provider-lifecycle-matrix.json")
	if err := os.WriteFile(matrixPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	readback, canonical, err := BuildRegistryReadback(root, matrixPath, "0123456789abcdef0123456789abcdef01234567")
	if err != nil {
		t.Fatal(err)
	}
	if !readback.PublicationReady || readback.ProviderVersion != providerVersion || len(readback.Installs) != 2 || len(canonical) == 0 {
		t.Fatalf("unexpected current Registry readback: %#v", readback)
	}
}

func TestBuildRegistryReadbackRejectsRetiredProviderEpoch(t *testing.T) {
	root, err := RepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	matrixPath := filepath.Join(root, "admission", "v1", "registry", "provider-lifecycle-matrix.json")
	if _, _, err := BuildRegistryReadback(root, matrixPath, "0123456789abcdef0123456789abcdef01234567"); err == nil {
		t.Fatal("current provider readback accepted the retired provider-v1 matrix")
	}
}
