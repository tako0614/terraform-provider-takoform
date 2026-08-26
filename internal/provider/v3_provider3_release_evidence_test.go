package provider

import (
	"bytes"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const v3Provider3ReleaseEvidencePath = "testdata/v3-provider3-release-evidence.json"

type v3Provider3ReleaseEvidence struct {
	Format          string                    `json:"format"`
	ProviderVersion string                    `json:"providerVersion"`
	Tag             string                    `json:"tag"`
	TagObject       string                    `json:"tagObject"`
	Commit          string                    `json:"commit"`
	GitHubRelease   v3Provider3GitHubRelease  `json:"githubRelease"`
	Registry        v3Provider3RegistryProof  `json:"registry"`
	Assets          []v3Provider3ReleaseAsset `json:"assets"`
}

type v3Provider3GitHubRelease struct {
	ID          int64  `json:"id"`
	URL         string `json:"url"`
	Immutable   bool   `json:"immutable"`
	PublishedAt string `json:"publishedAt"`
}

type v3Provider3RegistryProof struct {
	ProviderAddress       string   `json:"providerAddress"`
	Protocol              string   `json:"protocol"`
	Platforms             []string `json:"platforms"`
	LinuxAMD64SHA256      string   `json:"linuxAmd64Sha256"`
	InstalledSchemaSHA256 string   `json:"installedSchemaSha256"`
	ResourceSchemaCount   int      `json:"resourceSchemaCount"`
}

type v3Provider3ReleaseAsset struct {
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	ContentType string `json:"contentType"`
	Digest      string `json:"digest"`
}

// TestV3Provider3ReleaseEvidenceLocksImmutableAssets joins the source/tag,
// immutable GitHub Release assets, Registry readback and installed Provider
// schema. It is an offline lock of a live readback, not a replacement for the
// owning release workflow or a claim that candidate Form Packages are
// published.
func TestV3Provider3ReleaseEvidenceLocksImmutableAssets(t *testing.T) {
	evidence := readV3Provider3ReleaseEvidence(t)
	if evidence.Format != "takoform.provider3-release-evidence@v1" ||
		evidence.ProviderVersion != "3.0.0" || evidence.Tag != "v3.0.0" ||
		evidence.TagObject != "2c0f879b6e38d9995a4f5a4853a44a22c6aaa50a" ||
		evidence.Commit != "a225cfa7c84aa551981cc8ad56c9a281fa6e051a" {
		t.Fatalf("Provider 3 source identity drifted: %#v", evidence)
	}
	if evidence.GitHubRelease.ID != 376051280 ||
		evidence.GitHubRelease.URL != "https://github.com/tako0614/terraform-provider-takoform/releases/tag/v3.0.0" ||
		!evidence.GitHubRelease.Immutable ||
		evidence.GitHubRelease.PublishedAt != "2026-08-24T23:44:13Z" {
		t.Fatalf("Provider 3 GitHub Release identity drifted: %#v", evidence.GitHubRelease)
	}
	if evidence.Registry.ProviderAddress != "registry.terraform.io/tako0614/takoform" ||
		evidence.Registry.Protocol != "6.0" || evidence.Registry.ResourceSchemaCount != 31 ||
		evidence.Registry.InstalledSchemaSHA256 != "0dc07fc814386e51f66745cf1c482438c050a5c14c197c1778988cab46184da6" {
		t.Fatalf("Provider 3 Registry/install proof drifted: %#v", evidence.Registry)
	}
	wantPlatforms := []string{"darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64", "windows_amd64"}
	if strings.Join(evidence.Registry.Platforms, ",") != strings.Join(wantPlatforms, ",") {
		t.Fatalf("Provider 3 platforms = %v, want %v", evidence.Registry.Platforms, wantPlatforms)
	}

	names := make([]string, 0, len(evidence.Assets))
	seen := make(map[string]struct{}, len(evidence.Assets))
	var binaryArchives, sboms, other int
	var linuxAMD64Digest string
	for _, asset := range evidence.Assets {
		if asset.Name == "" || asset.Size <= 0 || asset.ContentType != "application/octet-stream" || !formpackage.ValidDigest(asset.Digest) {
			t.Fatalf("invalid Provider 3 release asset: %#v", asset)
		}
		if _, duplicate := seen[asset.Name]; duplicate {
			t.Fatalf("duplicate Provider 3 release asset %q", asset.Name)
		}
		seen[asset.Name] = struct{}{}
		names = append(names, asset.Name)
		switch {
		case strings.HasSuffix(asset.Name, ".zip"):
			binaryArchives++
		case strings.HasSuffix(asset.Name, ".zip.spdx.json"):
			sboms++
		default:
			other++
		}
		if asset.Name == "terraform-provider-takoform_3.0.0_linux_amd64.zip" {
			linuxAMD64Digest = asset.Digest
		}
	}
	ordered := append([]string(nil), names...)
	sort.Strings(ordered)
	if strings.Join(names, "\n") != strings.Join(ordered, "\n") {
		t.Fatalf("Provider 3 release assets are not in stable name order: %v", names)
	}
	if len(evidence.Assets) != 15 || binaryArchives != 5 || sboms != 5 || other != 5 {
		t.Fatalf("Provider 3 asset closure = total %d, archives %d, SBOMs %d, other %d; want 15/5/5/5", len(evidence.Assets), binaryArchives, sboms, other)
	}
	if linuxAMD64Digest != "sha256:"+evidence.Registry.LinuxAMD64SHA256 {
		t.Fatalf("GitHub/Registry linux_amd64 digest mismatch: %q != sha256:%s", linuxAMD64Digest, evidence.Registry.LinuxAMD64SHA256)
	}

	ledger := readV3ProviderReleaseLedger(t)
	var release *v3ProviderReleaseLedgerEntry
	for index := range ledger.Entries {
		if ledger.Entries[index].Version == evidence.ProviderVersion {
			release = &ledger.Entries[index]
			break
		}
	}
	if release == nil {
		t.Fatal("Provider 3.0.0 is absent from the provider release identity ledger")
	}
	if release.Tag != evidence.Tag || release.TagObject != evidence.TagObject || release.Commit != evidence.Commit ||
		release.RegistryReadback.ProviderAddress != evidence.Registry.ProviderAddress ||
		release.RegistryReadback.GitHubRelease != evidence.GitHubRelease ||
		release.RegistryReadback.Registry.Protocol != evidence.Registry.Protocol ||
		release.RegistryReadback.Registry.LinuxAMD64SHA256 != evidence.Registry.LinuxAMD64SHA256 ||
		release.RegistryReadback.Installation.SchemaSHA256 != evidence.Registry.InstalledSchemaSHA256 ||
		release.RegistryReadback.Installation.ResourceSchemaCount != evidence.Registry.ResourceSchemaCount {
		t.Fatalf("Provider 3 release evidence disagrees with release ledger: evidence=%#v ledger=%#v", evidence, *release)
	}
	if golden := readV3Provider3Golden(t); golden.ResourceCount != evidence.Registry.ResourceSchemaCount {
		t.Fatalf("Provider 3 source golden has %d resources, installed schema reports %d", golden.ResourceCount, evidence.Registry.ResourceSchemaCount)
	}
}

func readV3Provider3ReleaseEvidence(t *testing.T) v3Provider3ReleaseEvidence {
	t.Helper()
	raw, err := os.ReadFile(v3Provider3ReleaseEvidencePath)
	if err != nil {
		t.Fatalf("read %s: %v", v3Provider3ReleaseEvidencePath, err)
	}
	document := bytes.TrimSuffix(raw, []byte("\n"))
	canonical, err := formpackage.Canonicalize(document)
	if err != nil {
		t.Fatalf("canonicalize %s: %v", v3Provider3ReleaseEvidencePath, err)
	}
	if !bytes.Equal(document, canonical) || len(raw)-len(document) > 1 {
		t.Fatalf("%s is not RFC 8785 canonical JSON with at most one final newline", v3Provider3ReleaseEvidencePath)
	}
	var evidence v3Provider3ReleaseEvidence
	if err := json.Unmarshal(raw, &evidence); err != nil {
		t.Fatalf("decode %s: %v", v3Provider3ReleaseEvidencePath, err)
	}
	return evidence
}

type v3ProviderReleaseLedger struct {
	Entries []v3ProviderReleaseLedgerEntry `json:"entries"`
}

type v3ProviderReleaseLedgerEntry struct {
	Version          string                     `json:"version"`
	Tag              string                     `json:"tag"`
	TagObject        string                     `json:"tagObject"`
	Commit           string                     `json:"commit"`
	RegistryReadback v3ProviderRegistryReadback `json:"registryReadback"`
}

type v3ProviderRegistryReadback struct {
	ProviderAddress string                   `json:"providerAddress"`
	GitHubRelease   v3Provider3GitHubRelease `json:"githubRelease"`
	Registry        struct {
		Protocol         string `json:"protocol"`
		LinuxAMD64SHA256 string `json:"linuxAmd64Sha256"`
	} `json:"registry"`
	Installation struct {
		SchemaSHA256        string `json:"schemaSha256"`
		ResourceSchemaCount int    `json:"resourceSchemaCount"`
	} `json:"installation"`
}

func readV3ProviderReleaseLedger(t *testing.T) v3ProviderReleaseLedger {
	t.Helper()
	raw, err := os.ReadFile("../../release/provider-release-identities.json")
	if err != nil {
		t.Fatalf("read Provider release identity ledger: %v", err)
	}
	var ledger v3ProviderReleaseLedger
	if err := json.Unmarshal(raw, &ledger); err != nil {
		t.Fatalf("decode Provider release identity ledger: %v", err)
	}
	return ledger
}
