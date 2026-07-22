package admissionmaterial

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		"ref: ${{ inputs.snapshot_commit }}",
		"test \"$(git rev-parse HEAD^{tree})\" = \"${SNAPSHOT_TREE}\"",
		"git merge-base --is-ancestor \"${SNAPSHOT_COMMIT}\" \"${current_main}\"",
		"git rev-parse \"${SNAPSHOT_COMMIT}:${HOST_CANDIDATE_PATH}\"",
		"git rev-parse \"${SNAPSHOT_COMMIT}:${PROVIDER_CANDIDATE_PATH}\"",
		"--trusted-root admission/v1/trust/trusted-root.json",
		"--host-source-commit \"${HOST_SOURCE_COMMIT}\"",
		"--host-takoform-source-commit \"${HOST_TAKOFORM_SOURCE_COMMIT}\"",
		"--provider-source-commit \"${PROVIDER_SOURCE_COMMIT}\"",
		"--host-run-id \"${HOST_RUN_ID}\"",
		"--provider-run-id \"${PROVIDER_RUN_ID}\"",
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
	if strings.Contains(jobs[1], "actions/checkout@") || strings.Contains(jobs[1], "contents: read") || strings.Contains(jobs[1], "contents: write") {
		t.Fatal("signer regained source checkout or repository content authority")
	}
	if strings.Contains(jobs[1], "gh release") || strings.Contains(jobs[1], "git tag") {
		t.Fatal("evidence signer regained publication authority")
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
