package admissionmaterial

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/internal/admissionrelease"
)

func TestPrepareOutputPathRejectsRepositoryAndExistingPaths(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	repository := filepath.Join(parent, "repository")
	if err := os.Mkdir(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareOutputPath(repository, filepath.Join(repository, "material")); err == nil || !strings.Contains(err.Error(), "outside the repository") {
		t.Fatalf("repository-contained output error = %v", err)
	}
	link := filepath.Join(parent, "repository-link")
	if err := os.Symlink(repository, link); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareOutputPath(repository, filepath.Join(link, "material")); err == nil || !strings.Contains(err.Error(), "outside the repository") {
		t.Fatalf("symlinked repository-contained output error = %v", err)
	}
	existing := filepath.Join(parent, "existing")
	if err := os.Mkdir(existing, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareOutputPath(repository, existing); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing output error = %v", err)
	}

	want := filepath.Join(parent, "new-material")
	got, err := prepareOutputPath(repository, want)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestBuildRejectsUnboundSourceAndWorkflowRunIdentitiesBeforeReadingArtifacts(t *testing.T) {
	t.Parallel()
	validCommit := strings.Repeat("a", 40)
	tests := []struct {
		name    string
		options BuildOptions
		want    string
	}{
		{
			name:    "invalid host source commit",
			options: BuildOptions{SourceCommit: validCommit, HostSourceCommit: "main", HostTakoformSourceCommit: validCommit, ProviderSourceCommit: validCommit, HostWorkflowRunID: "1", ProviderWorkflowRunID: "2"},
			want:    "host source commit",
		},
		{
			name:    "missing host workflow run id",
			options: BuildOptions{SourceCommit: validCommit, HostSourceCommit: validCommit, HostTakoformSourceCommit: validCommit, ProviderSourceCommit: validCommit, ProviderWorkflowRunID: "2"},
			want:    "host workflow run id",
		},
		{
			name:    "missing host Takoform source commit",
			options: BuildOptions{SourceCommit: validCommit, HostSourceCommit: validCommit, ProviderSourceCommit: validCommit, HostWorkflowRunID: "1", ProviderWorkflowRunID: "2"},
			want:    "host Takoform source commit",
		},
		{
			name:    "missing provider source commit",
			options: BuildOptions{SourceCommit: validCommit, HostSourceCommit: validCommit, HostTakoformSourceCommit: validCommit, HostWorkflowRunID: "1", ProviderWorkflowRunID: "2"},
			want:    "provider source commit",
		},
		{
			name:    "invalid provider workflow run id",
			options: BuildOptions{SourceCommit: validCommit, HostSourceCommit: validCommit, HostTakoformSourceCommit: validCommit, ProviderSourceCommit: validCommit, HostWorkflowRunID: "1", ProviderWorkflowRunID: "02"},
			want:    "provider workflow run id",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := Build(test.options); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Build error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestAdmissionEvidenceWorkflowBindsExactRunsAndSeparatesSignerAuthority(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "standard-admission-evidence.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(raw)
	for _, required := range []string{
		"permissions: {}",
		"request_id:",
		"run-name: ${{ inputs.request_id }}",
		`[[ ! "$REQUEST_ID" =~ ^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$ ]]`,
		"ref: ${{ inputs.snapshot_commit }}",
		"test \"$(git rev-parse HEAD^{tree})\" = \"${SNAPSHOT_TREE}\"",
		"git merge-base --is-ancestor \"${SNAPSHOT_COMMIT}\" \"${current_main}\"",
		"git rev-parse \"${SNAPSHOT_COMMIT}:${HOST_CANDIDATE_PATH}\"",
		"git rev-parse \"${SNAPSHOT_COMMIT}:${PROVIDER_CANDIDATE_PATH}\"",
		"git rev-parse \"${SNAPSHOT_COMMIT}:${REGISTRY_CANDIDATE_PATH}\"",
		"test \"${HOST_CANDIDATE_PATH}\" = \"admission/v4/candidates/host-report-${ADMISSION_VERSION}-${HOST_SOURCE_COMMIT:0:12}-${HOST_TAKOFORM_SOURCE_COMMIT:0:12}\"",
		"test \"${PROVIDER_CANDIDATE_PATH}\" = \"admission/v4/candidates/provider-report-${ADMISSION_VERSION}-${PROVIDER_SOURCE_COMMIT:0:12}\"",
		"test \"${REGISTRY_CANDIDATE_PATH}\" = \"admission/v4/candidates/registry-readback-${ADMISSION_VERSION}-${PROVIDER_SOURCE_COMMIT:0:12}\"",
		"jq -er '.version' admission/v4/version.json",
		"jq -er '.tag' admission/v4/version.json",
		"--trusted-root admission/v4/trust/trusted-root.json",
		"--host-id \"${HOST_ID}\"",
		"admission/v4/conforming-hosts.json",
		"build-current",
		"--registry-readback \"${REGISTRY_CANDIDATE_PATH}\"",
		"--host-source-commit \"${HOST_SOURCE_COMMIT}\"",
		"--host-takoform-source-commit \"${HOST_TAKOFORM_SOURCE_COMMIT}\"",
		"--provider-source-commit \"${PROVIDER_SOURCE_COMMIT}\"",
		"--host-request-id \"${HOST_REQUEST_ID}\"",
		"--host-run-id \"${HOST_RUN_ID}\"",
		"--host-run-attempt \"${HOST_RUN_ATTEMPT}\"",
		"--host-head-sha \"${HOST_HEAD_SHA}\"",
		"--provider-request-id \"${PROVIDER_REQUEST_ID}\"",
		"--provider-run-id \"${PROVIDER_RUN_ID}\"",
		"--provider-run-attempt \"${PROVIDER_RUN_ATTEMPT}\"",
		"--provider-head-sha \"${PROVIDER_HEAD_SHA}\"",
		"--registry-request-id \"${REGISTRY_REQUEST_ID}\"",
		"--registry-run-id \"${REGISTRY_RUN_ID}\"",
		"--registry-run-attempt \"${REGISTRY_RUN_ATTEMPT}\"",
		"--registry-head-sha \"${REGISTRY_HEAD_SHA}\"",
		"environment: standard-admission-evidence",
		"artifact-ids: ${{ needs.assemble.outputs.artifact_id }}",
		"digest-mismatch: error",
		"id-token: write",
		"standard-admission-evidence-candidate-",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("workflow omits %q", required)
		}
	}
	jobs := strings.Split(workflow, "\n  sign:\n")
	if len(jobs) != 2 {
		t.Fatal("workflow does not contain one isolated sign job")
	}
	if strings.Contains(jobs[0], "id-token: write") {
		t.Fatal("assembly job received OIDC signing authority")
	}
	for _, forbidden := range []string{"TAKOSUMI_ACTIONS_READ_TOKEN", "gh run download", "actions/runs/${HOST_RUN_ID}", "actions/runs/${PROVIDER_RUN_ID}"} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("workflow reintroduced expiring artifact or cross-repository secret dependency %q", forbidden)
		}
	}
	// A portable contract may not name one host as the source of admission.
	// The accepted publisher is reviewed data, not a workflow constant.
	if strings.Contains(workflow, "github.com/tako0614/takosumi") {
		t.Fatal("admission workflow hard-codes a single host repository identity")
	}
	if strings.Contains(jobs[1], "actions/checkout@") || strings.Contains(jobs[1], "contents: read") || strings.Contains(jobs[1], "contents: write") {
		t.Fatal("signer regained source checkout or repository content authority")
	}
	if strings.Contains(jobs[1], "gh release") || strings.Contains(jobs[1], "git tag") {
		t.Fatal("evidence signer regained publication authority")
	}
}

func TestBuildCurrentRequiresRegistryCandidateAndRunBinding(t *testing.T) {
	validCommit := strings.Repeat("a", 40)
	validRequestID := "01234567-89ab-4cde-8f01-23456789abcd"
	options := CurrentBuildOptions{BuildOptions: BuildOptions{
		SourceCommit: validCommit, HostSourceCommit: validCommit,
		HostTakoformSourceCommit: validCommit, ProviderSourceCommit: validCommit,
		HostWorkflowRunID: "1", ProviderWorkflowRunID: "2",
		AdmissionVersion: "1.0.0", HostReports: "host", ProviderReports: "provider",
		OutputDir: "output", HostID: "host",
	},
		HostRequestID: validRequestID, HostWorkflowRunAttempt: "1", HostHeadSHA: validCommit,
		ProviderRequestID: validRequestID, ProviderWorkflowRunAttempt: "1", ProviderHeadSHA: validCommit,
		RegistryRequestID: validRequestID, RegistryWorkflowRunAttempt: "1", RegistryHeadSHA: validCommit,
	}
	if err := BuildCurrent(options); err == nil || !strings.Contains(err.Error(), "registry workflow run id") {
		t.Fatalf("missing registry run binding error = %v", err)
	}
	options.RegistryWorkflowRunID = "3"
	if err := BuildCurrent(options); err == nil || !strings.Contains(err.Error(), "current host, provider, Registry") {
		t.Fatalf("missing registry candidate error = %v", err)
	}
}

func TestCurrentProviderReportsMatchRegistryByExactBinary(t *testing.T) {
	t.Parallel()
	const binaryDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	providers := artifactSet{
		allByKind: map[string]reportArtifact{
			"ObjectBucket": {
				report: admissionrelease.RunnerReport{
					RunnerVersion:        "0.2.1",
					ProviderBinarySHA256: binaryDigest,
				},
			},
		},
		providerBinarySHA256: binaryDigest,
	}
	registry := registryArtifactSet{
		parsed: admissionrelease.ProviderRegistryReadback{
			ProviderVersion:       "0.2.1",
			ProviderReleaseCommit: "89abcdef0123456789abcdef0123456789abcdef",
			Installs: []admissionrelease.RegistryInstall{
				{Product: "OpenTofu", ProviderVersion: "0.2.1", ProviderBinarySHA256: binaryDigest},
				{Product: "Terraform", ProviderVersion: "0.2.1", ProviderBinarySHA256: binaryDigest},
			},
		},
	}
	if err := validateCurrentProviderRegistryIdentity(providers, registry); err != nil {
		t.Fatalf("exact provider binary must permit later evidence tooling commits: %v", err)
	}
	registry.parsed.Installs[0].ProviderBinarySHA256 = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if err := validateCurrentProviderRegistryIdentity(providers, registry); err == nil ||
		!strings.Contains(err.Error(), "binary") {
		t.Fatalf("provider binary substitution error = %v", err)
	}
}

func TestRegistryReadbackWorkflowUsesBothCLIsAndAnIsolatedSigner(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "provider-registry-readback.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(raw)
	for _, required := range []string{
		"permissions: {}",
		"tofu_version: 1.12.1",
		"terraform_version: 1.15.8",
		"render-registry-matrix",
		"admission-readback registry",
		"takoform.provider-registry-readback-candidate@v1",
		"takoform.provider-registry-readback-signed-candidate@v1",
		"request_id:",
		"requestId:process.env.REQUEST_ID",
		"environment: provider-registry-readback",
		"artifact-ids: ${{ needs.generate.outputs.artifact_id }}",
		"digest-mismatch: error",
		"provider-readback.sigstore.json",
		registryIdentity,
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("Registry workflow omits %q", required)
		}
	}
	jobs := strings.Split(workflow, "\n  sign:\n")
	if len(jobs) != 2 {
		t.Fatal("Registry workflow does not contain one isolated sign job")
	}
	if strings.Contains(jobs[0], "id-token: write") {
		t.Fatal("Registry execution job received signing authority")
	}
	if strings.Contains(jobs[1], "contents: write") ||
		strings.Contains(workflow, "gh release") ||
		strings.Contains(workflow, "git push") {
		t.Fatal("Registry evidence workflow gained publication authority")
	}
}

func TestCurrentEvidenceRequestIDRequiresCanonicalUUIDv4(t *testing.T) {
	t.Parallel()
	if !canonicalUUIDv4Pattern.MatchString("01234567-89ab-4cde-8f01-23456789abcd") {
		t.Fatal("canonical lowercase UUIDv4 was rejected")
	}
	for _, invalid := range []string{
		"01234567-89ab-3cde-8f01-23456789abcd",
		"01234567-89ab-4cde-7f01-23456789abcd",
		"01234567-89AB-4CDE-8F01-23456789ABCD",
		"req_latest",
	} {
		if canonicalUUIDv4Pattern.MatchString(invalid) {
			t.Errorf("invalid request id %q was accepted", invalid)
		}
	}
}

func TestVerifyChecksumsRejectsDuplicateEntries(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	payload := []byte("exact payload")
	if err := os.WriteFile(filepath.Join(root, "payload.json"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(payload))
	line := digest + "  payload.json\n"
	if err := os.WriteFile(filepath.Join(root, checksumsName), []byte(line+line), 0o600); err != nil {
		t.Fatal(err)
	}
	err := verifyChecksums(root, map[string]struct{}{checksumsName: {}, "payload.json": {}})
	if err == nil || !strings.Contains(err.Error(), "duplicates") {
		t.Fatalf("duplicate checksum error = %v", err)
	}
}
