package formpublication_test

import (
	"encoding/base32"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/admissionrelease"
	"github.com/tako0614/terraform-provider-takoform/internal/formpublication"
)

const exactTestPackageCount = 34

type structureFixture struct {
	root       string
	candidates admissionrelease.CandidateSet
	set        formpublication.Set
}

type testReleasePlan struct {
	Format     string               `json:"format"`
	Generation string               `json:"generation"`
	Repository string               `json:"repository"`
	Note       string               `json:"note"`
	Releases   []testPlannedRelease `json:"releases"`
}

type testPlannedRelease struct {
	Kind          string              `json:"kind"`
	Slug          string              `json:"slug"`
	ReleaseID     string              `json:"releaseId"`
	Version       string              `json:"version"`
	Tag           string              `json:"tag"`
	SourcePath    string              `json:"sourcePath"`
	FormRef       formpackage.FormRef `json:"formRef"`
	PackageDigest string              `json:"packageDigest"`
}

func TestVerifyStructureAcceptsExactThirtyFourReleaseClosure(t *testing.T) {
	fixture := newStructureFixture(t)

	set, err := formpublication.VerifyStructure(fixture.root, fixture.candidates)
	if err != nil {
		t.Fatalf("verify exact publication closure: %v", err)
	}
	if len(set.Entries) != exactTestPackageCount {
		t.Fatalf("verified entries = %d, want %d", len(set.Entries), exactTestPackageCount)
	}
}

func TestVerifyStructureFailsClosedOnClosureDrift(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*testing.T, *structureFixture)
		wantErr string
	}{
		{
			name: "entry omission",
			mutate: func(t *testing.T, fixture *structureFixture) {
				fixture.set.Entries = fixture.set.Entries[:len(fixture.set.Entries)-1]
				writeCanonicalJSON(t, filepath.Join(fixture.root, formpublication.SetFilename), fixture.set)
			},
			wantErr: "want exactly 34",
		},
		{
			name: "asset byte tamper",
			mutate: func(t *testing.T, fixture *structureFixture) {
				entry := fixture.set.Entries[0]
				name := filepath.Join(
					fixture.root, "releases", entry.ReleaseID, entry.Version,
					entry.Assets[0].Name,
				)
				if err := os.WriteFile(name, []byte("tampered bytes"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "byte readback mismatch",
		},
		{
			name: "extra release file",
			mutate: func(t *testing.T, fixture *structureFixture) {
				entry := fixture.set.Entries[0]
				writeTestFile(t, filepath.Join(
					fixture.root, "releases", entry.ReleaseID, entry.Version, "unretained",
				), []byte("extra"))
			},
			wantErr: "extra file",
		},
		{
			name: "extra authority file",
			mutate: func(t *testing.T, fixture *structureFixture) {
				writeTestFile(t, filepath.Join(
					fixture.root, "authority", fixture.set.Entries[0].ToolingCommit, "unretained",
				), []byte("extra"))
			},
			wantErr: "extra file",
		},
		{
			name: "historical authority root substitution",
			mutate: func(t *testing.T, fixture *structureFixture) {
				authorityPath := filepath.Join(
					fixture.root,
					filepath.FromSlash(fixture.set.Entries[0].TrustedRoot.Path),
				)
				raw, err := os.ReadFile(authorityPath)
				if err != nil {
					t.Fatal(err)
				}
				var document map[string]any
				if err := json.Unmarshal(raw, &document); err != nil {
					t.Fatal(err)
				}
				tlogs := document["tlogs"].([]any)
				tlogs[0].(map[string]any)["baseUrl"] = "https://substituted.example"
				substituted := canonicalTestJSON(t, document)
				writeTestFile(t, authorityPath, substituted)
				for index := range fixture.set.Entries {
					fixture.set.Entries[index].TrustedRoot.SHA256 = testDigest(substituted)
				}
				writeCanonicalJSON(
					t, filepath.Join(fixture.root, formpublication.SetFilename), fixture.set,
				)
			},
			wantErr: "differs semantically",
		},
		{
			name: "asset symlink",
			mutate: func(t *testing.T, fixture *structureFixture) {
				entry := fixture.set.Entries[0]
				name := filepath.Join(
					fixture.root, "releases", entry.ReleaseID, entry.Version,
					entry.Assets[0].Name,
				)
				if err := os.Remove(name); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(fixture.root, formpublication.SetFilename), name); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "regular file, not a symlink",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newStructureFixture(t)
			test.mutate(t, &fixture)
			_, err := formpublication.VerifyStructure(fixture.root, fixture.candidates)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestProjectEntriesSelectsExactSubsetAndRejectsSubstitution(t *testing.T) {
	fixture := newStructureFixture(t)
	if _, err := formpublication.VerifyStructure(fixture.root, fixture.candidates); err != nil {
		t.Fatal(err)
	}
	selected := admissionrelease.CandidateSet{
		Generation: "ga-core-v2",
		Entries: append(
			[]admissionrelease.Candidate(nil), fixture.candidates.Entries[:10]...,
		),
	}

	projected, err := formpublication.ProjectEntries(fixture.set, selected)
	if err != nil {
		t.Fatalf("project exact selected subset: %v", err)
	}
	if len(projected) != 10 {
		t.Fatalf("projected entries = %d, want 10", len(projected))
	}
	for _, candidate := range selected.Entries {
		entry, ok := projected[candidate.Kind]
		if !ok || entry.Slug != candidate.Slug ||
			entry.FormRef != candidate.FormRef ||
			entry.PackageDigest != candidate.PackageDigest {
			t.Fatalf("%s projected identity drifted: %+v", candidate.Kind, entry)
		}
	}

	t.Run("selected identity substitution", func(t *testing.T) {
		substituted := selected
		substituted.Entries = append([]admissionrelease.Candidate(nil), selected.Entries...)
		substituted.Entries[0].PackageDigest = testDigest([]byte("substituted"))
		_, err := formpublication.ProjectEntries(fixture.set, substituted)
		if err == nil || !strings.Contains(err.Error(), "identity differs") {
			t.Fatalf("substitution error = %v", err)
		}
	})
	t.Run("selected kind omission", func(t *testing.T) {
		missing := selected
		missing.Entries = append([]admissionrelease.Candidate(nil), selected.Entries...)
		releaseID := testReleaseID("MissingKind")
		missing.Entries[0].Kind = "MissingKind"
		missing.Entries[0].Slug = "missing-kind"
		missing.Entries[0].PackagePath = "forms/releases/" + releaseID + "/1.0.0"
		missing.Entries[0].FormRef.Kind = "MissingKind"
		_, err := formpublication.ProjectEntries(fixture.set, missing)
		if err == nil || !strings.Contains(err.Error(), "absent") {
			t.Fatalf("missing-kind error = %v", err)
		}
	})
	t.Run("duplicate publication kind", func(t *testing.T) {
		set := fixture.set
		set.Entries = append([]formpublication.Entry(nil), fixture.set.Entries...)
		set.Entries[1] = set.Entries[0]
		_, err := formpublication.ProjectEntries(set, selected)
		if err == nil || !strings.Contains(err.Error(), "duplicates kind") {
			t.Fatalf("duplicate-kind error = %v", err)
		}
	})
	t.Run("duplicate tag object", func(t *testing.T) {
		set := fixture.set
		set.Entries = append([]formpublication.Entry(nil), fixture.set.Entries...)
		set.Entries[1].TagObjectOID = set.Entries[0].TagObjectOID
		_, err := formpublication.ProjectEntries(set, selected)
		if err == nil || !strings.Contains(err.Error(), "duplicates tag object") {
			t.Fatalf("duplicate tag-object error = %v", err)
		}
	})
}

func newStructureFixture(t *testing.T) structureFixture {
	t.Helper()
	root := t.TempDir()
	version := "1.0.0"
	toolingCommit := strings.Repeat("a", 40)
	protectedMain := strings.Repeat("b", 40)
	plan := testReleasePlan{
		Format: "takoform.release-plan@v1", Generation: "portable-v1",
		Repository: "tako0614/terraform-provider-takoform",
		Note:       "Synthetic exact publication plan used by the structural verifier test.",
		Releases:   make([]testPlannedRelease, 0, exactTestPackageCount),
	}
	candidates := admissionrelease.CandidateSet{
		Generation: "portable-v1",
		Entries:    make([]admissionrelease.Candidate, 0, exactTestPackageCount),
	}
	set := formpublication.Set{
		Format: formpublication.SetFormat, Generation: "portable-v1",
		Repository:        "tako0614/terraform-provider-takoform",
		PublicationStatus: "published-immutable", AdmissionStatus: "external-required",
		RevocationCheckpointStatus: "external-required",
		GitObjectFormat:            "sha1", ProtectedMainCommit: protectedMain,
		VerificationPolicy: formpublication.VerificationPolicy{
			CertificateIdentity: "https://github.com/tako0614/terraform-provider-takoform/.github/workflows/form-package-release.yml@refs/heads/main",
			OIDCIssuer:          "https://token.actions.githubusercontent.com",
			BundleMediaType:     "application/vnd.dev.sigstore.bundle.v0.3+json",
		},
		Entries: make([]formpublication.Entry, 0, exactTestPackageCount),
	}
	for index := 0; index < exactTestPackageCount; index++ {
		kind := fmt.Sprintf("TestKind%02d", index)
		slug := fmt.Sprintf("test-kind-%02d", index)
		releaseID := testReleaseID(kind)
		sourcePath := filepath.ToSlash(filepath.Join("forms", "releases", releaseID, version))
		base := "takoform-form-" + releaseID + "_" + version
		indexName := base + "_package-index.json"
		names := []string{
			"SHA256SUMS",
			"release-manifest.json",
			base + ".tar.gz",
			indexName,
			base + "_package-index.sigstore.json",
			base + "_provenance.intoto.json",
			base + "_sbom.spdx.json",
		}
		sort.Strings(names)
		assets := make([]formpublication.Asset, 0, len(names))
		packageDigest := ""
		for _, name := range names {
			raw := []byte(kind + " " + name + "\n")
			if name == indexName {
				packageDigest = testDigest(raw)
			}
			assets = append(assets, formpublication.Asset{
				Name: name, SHA256: testDigest(raw), Size: int64(len(raw)),
			})
			writeTestFile(t, filepath.Join(
				root, "releases", releaseID, version, name,
			), raw)
		}
		formRef := formpackage.FormRef{
			APIVersion: formpackage.FormAPIVersion, Kind: kind,
			DefinitionVersion: version,
			SchemaDigest:      testDigest([]byte(kind + " schema")),
		}
		candidate := admissionrelease.Candidate{
			Kind: kind, Slug: slug, PackagePath: sourcePath,
			FormRef: formRef, PackageDigest: packageDigest,
		}
		candidates.Entries = append(candidates.Entries, candidate)
		planned := testPlannedRelease{
			Kind: kind, Slug: slug, ReleaseID: releaseID, Version: version,
			Tag: "forms/" + releaseID + "/v" + version, SourcePath: sourcePath,
			FormRef: formRef, PackageDigest: packageDigest,
		}
		plan.Releases = append(plan.Releases, planned)
		set.Entries = append(set.Entries, formpublication.Entry{
			Kind: kind, ReleaseID: releaseID, Version: version, Tag: planned.Tag,
			SourcePath: sourcePath, FormRef: formRef, PackageDigest: packageDigest,
			TagObjectOID: fmt.Sprintf("%040x", index+1),
			PeeledCommit: toolingCommit, SourceCommit: toolingCommit,
			ToolingCommit:   toolingCommit,
			GitHubReleaseID: fmt.Sprintf("%d", 1000+index),
			PublishedAt:     "2026-07-30T00:00:00Z", Immutable: true,
			Assets: assets,
		})
	}
	planRaw := canonicalTestJSON(t, plan)
	writeTestFile(t, filepath.Join(root, "release-plan.json"), planRaw)
	set.SourcePlan = formpublication.SourcePlan{
		Path: "release-plan.json", SourcePath: "forms/release-plan.json",
		SHA256: testDigest(planRaw),
	}
	trustedRoot := readRepositoryTrustedRoot(t)
	writeTestFile(t, filepath.Join(root, "trust", "trusted-root.json"), trustedRoot)
	set.VerificationPolicy.TrustedRoot = formpublication.SourcePlan{
		Path:       "trust/trusted-root.json",
		SourcePath: "admission/v4/trust/trusted-root.json",
		SHA256:     testDigest(trustedRoot),
	}
	authorityPlanPath := filepath.ToSlash(filepath.Join(
		"authority", toolingCommit, "release-plan.json",
	))
	authorityRootPath := filepath.ToSlash(filepath.Join(
		"authority", toolingCommit, "trusted-root.json",
	))
	writeTestFile(t, filepath.Join(root, filepath.FromSlash(authorityPlanPath)), planRaw)
	writeTestFile(t, filepath.Join(root, filepath.FromSlash(authorityRootPath)), trustedRoot)
	for index := range set.Entries {
		set.Entries[index].ReleasePlan = formpublication.SourcePlan{
			Path: authorityPlanPath, SourcePath: "forms/release-plan.json",
			SHA256: testDigest(planRaw),
		}
		set.Entries[index].TrustedRoot = formpublication.SourcePlan{
			Path:       authorityRootPath,
			SourcePath: "admission/v4/trust/trusted-root.json",
			SHA256:     testDigest(trustedRoot),
		}
	}
	writeCanonicalJSON(t, filepath.Join(root, formpublication.SetFilename), set)
	return structureFixture{root: root, candidates: candidates, set: set}
}

func readRepositoryTrustedRoot(t *testing.T) []byte {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	name := filepath.Join(
		filepath.Dir(current), "..", "..", "admission", "v4", "trust", "trusted-root.json",
	)
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func canonicalTestJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

func writeCanonicalJSON(t *testing.T, name string, value any) {
	t.Helper()
	writeTestFile(t, name, canonicalTestJSON(t, value))
}

func writeTestFile(t *testing.T, name string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func testDigest(raw []byte) string {
	return formpackage.DigestBytes(raw)
}

func testReleaseID(kind string) string {
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(kind))
	return "k-" + strings.ToLower(encoded)
}
