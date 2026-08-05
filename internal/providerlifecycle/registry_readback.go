package providerlifecycle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const (
	registryReadbackFormat = "takoform.provider-registry-readback@v1"
	maxRegistryMatrixBytes = 16 << 20
)

var releaseCommitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// RegistryReadback is the canonical unsigned subject that binds one published
// provider version to direct Terraform Registry installs by every supported
// CLI. Signing and retention are separate release-workflow responsibilities.
type RegistryReadback struct {
	Format                string            `json:"format"`
	PublicationReady      bool              `json:"publicationReady"`
	ProviderAddress       string            `json:"providerAddress"`
	ProviderVersion       string            `json:"providerVersion"`
	ProviderReleaseTag    string            `json:"providerReleaseTag"`
	ProviderReleaseCommit string            `json:"providerReleaseCommit"`
	CandidateSetSHA256    string            `json:"candidateSetSha256"`
	ProviderSchemaSHA256  string            `json:"providerSchemaSha256"`
	LifecycleMatrixPath   string            `json:"lifecycleMatrixPath"`
	LifecycleMatrixDigest string            `json:"lifecycleMatrixDigest"`
	Installs              []RegistryInstall `json:"installs"`
}

// RegistryInstall identifies the exact provider bytes and schema observed by
// one supported CLI after a direct Registry install.
type RegistryInstall struct {
	Product              string `json:"product"`
	CLIVersion           string `json:"cliVersion"`
	ProviderAddress      string `json:"providerAddress"`
	ProviderVersion      string `json:"providerVersion"`
	ProviderBinarySHA256 string `json:"providerBinarySha256"`
	ProviderSchemaSHA256 string `json:"providerSchemaSha256"`
}

// BuildRegistryReadback validates the current provider epoch and returns its
// canonical unsigned Registry readback. Historical admission evidence uses a
// separate legacy validator and cannot be reissued through this path.
func BuildRegistryReadback(root, matrixFile, providerReleaseCommit string) (RegistryReadback, []byte, error) {
	if !releaseCommitPattern.MatchString(providerReleaseCommit) {
		return RegistryReadback{}, nil, fmt.Errorf("provider release commit must be lowercase 40-hex")
	}
	matrixRaw, err := readRegistryMatrix(matrixFile)
	if err != nil {
		return RegistryReadback{}, nil, err
	}
	var matrix MatrixReport
	if err := decodeRegistryJSON(matrixRaw, &matrix); err != nil {
		return RegistryReadback{}, nil, err
	}
	requirements, descriptorDigest, err := LoadCLIMatrix(root)
	if err != nil {
		return RegistryReadback{}, nil, err
	}
	if err := ValidateRegistryMatrix(matrix, requirements); err != nil {
		return RegistryReadback{}, nil, err
	}
	if matrix.ReleaseDescriptorSHA256 != descriptorDigest {
		return RegistryReadback{}, nil, fmt.Errorf("direct Registry matrix does not bind the current release descriptor")
	}
	providerVersion, err := loadProviderVersion(root)
	if err != nil {
		return RegistryReadback{}, nil, err
	}
	installs := make([]RegistryInstall, 0, len(matrix.Reports))
	for _, report := range matrix.Reports {
		if report.ProviderBinary.Version != providerVersion {
			return RegistryReadback{}, nil, fmt.Errorf("%s installed provider version is %q, want %q", report.CLI.Product, report.ProviderBinary.Version, providerVersion)
		}
		installs = append(installs, RegistryInstall{
			Product: report.CLI.Product, CLIVersion: report.CLI.Version, ProviderAddress: report.CLI.ProviderAddress,
			ProviderVersion: providerVersion, ProviderBinarySHA256: report.ProviderBinary.SHA256,
			ProviderSchemaSHA256: report.ProviderSchemaSHA256,
		})
	}
	readback := RegistryReadback{
		Format: registryReadbackFormat, PublicationReady: true, ProviderAddress: CanonicalProviderAddress,
		ProviderVersion: providerVersion, ProviderReleaseTag: "v" + providerVersion, ProviderReleaseCommit: providerReleaseCommit,
		CandidateSetSHA256: matrix.CandidateSetSHA256, ProviderSchemaSHA256: matrix.ProviderSchemaSHA256,
		LifecycleMatrixPath: "registry/provider-lifecycle-matrix.json", LifecycleMatrixDigest: formpackage.DigestBytes(matrixRaw),
		Installs: installs,
	}
	raw, err := json.Marshal(readback)
	if err != nil {
		return RegistryReadback{}, nil, err
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		return RegistryReadback{}, nil, err
	}
	return readback, canonical, nil
}

func readRegistryMatrix(filename string) ([]byte, error) {
	handle, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer handle.Close()
	info, err := handle.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxRegistryMatrixBytes {
		return nil, fmt.Errorf("Registry matrix must be a regular file no larger than %d bytes", maxRegistryMatrixBytes)
	}
	return io.ReadAll(io.LimitReader(handle, maxRegistryMatrixBytes+1))
}

func decodeRegistryJSON(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}
