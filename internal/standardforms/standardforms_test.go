package standardforms

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/admissioncheckpoint"
	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
)

func TestCommittedCandidateSetVerifies(t *testing.T) {
	t.Parallel()
	if err := Verify(filepath.Join("..", "..")); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyRejectsMissingProjectLifecycleAuthority(t *testing.T) {
	t.Parallel()
	err := Verify(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "project lifecycle authority") {
		t.Fatalf("missing lifecycle authority error = %v", err)
	}
}

func TestVerifyRejectsAStableProjectClaim(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, filepath.FromSlash(projectLifecyclePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"format":"takoform.form-lifecycle@v2","projectStatus":"stable"}`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	err := Verify(root)
	if err == nil || !strings.Contains(err.Error(), `projectStatus is "stable", want experimental`) {
		t.Fatalf("stable project claim error = %v", err)
	}
}

func TestVerifyRejectsAnApprovedLifecycleState(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, filepath.FromSlash(projectLifecyclePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{
  "format":"takoform.form-lifecycle@v2",
  "projectStatus":"experimental",
  "currentEpoch":"forms.takoform.com/v1alpha2",
  "states":["proposal","experimental","stable","approved","legacy"],
  "legacy":{},
  "currentForms":[],
  "proposals":[]
}`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	err := Verify(root)
	if err == nil || !strings.Contains(err.Error(), "states must be exactly proposal, experimental, stable, legacy") {
		t.Fatalf("approved lifecycle state error = %v", err)
	}
}

func TestVerifyRejectsUnknownLifecycleFormat(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, filepath.FromSlash(projectLifecyclePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{
  "format":"takoform.form-lifecycle@v3",
  "projectStatus":"experimental",
  "currentEpoch":"forms.takoform.com/v1alpha2",
  "states":["proposal","experimental","stable","legacy"],
  "legacy":{},
  "currentForms":[],
  "proposals":[]
}`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	err := Verify(root)
	if err == nil || !strings.Contains(err.Error(), `format is "takoform.form-lifecycle@v3", want takoform.form-lifecycle@v2`) {
		t.Fatalf("unknown lifecycle format error = %v", err)
	}
}

func TestVerifyRejectsUnpinnedLegacyReleaseSources(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, filepath.FromSlash(projectLifecyclePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{
  "format":"takoform.form-lifecycle@v2",
  "projectStatus":"experimental",
  "currentEpoch":"forms.takoform.com/v1alpha2",
  "states":["proposal","experimental","stable","legacy"],
  "legacy":{
    "apiVersion":"forms.takoform.com/v1alpha1",
    "epochDecision":"spec/decisions/0006-v1alpha2-restarts-form-lines.md",
    "decision":"spec/decisions/0004-takoform-is-an-experimental-specification.md",
    "releaseSources":"forms/releases",
    "newCreatePolicy":"host-policy",
    "retainedCapabilities":["read","observe","delete","recovery","migration"]
  },
  "currentForms":[],
  "proposals":[]
}`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	err := Verify(root)
	if err == nil || !strings.Contains(err.Error(), "legacy release source inventory pin is required") {
		t.Fatalf("unpinned legacy release source error = %v", err)
	}
}

func TestVerifyRejectsLegacyReleaseInventoryDigestDrift(t *testing.T) {
	t.Parallel()
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "repository")
	clone := exec.Command("git", "clone", "--quiet", "--local", "--no-hardlinks", repositoryRoot, root)
	if output, err := clone.CombinedOutput(); err != nil {
		t.Fatalf("clone lifecycle fixture repository: %v\n%s", err, output)
	}
	if err := os.Remove(filepath.Join(root, "forms", "admission-candidate-set.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	lifecycleRaw, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(projectLifecyclePath)))
	if err != nil {
		t.Fatal(err)
	}
	lifecycleRaw = bytes.ReplaceAll(
		lifecycleRaw,
		[]byte("sha256:292495fe4190d077eb993da0e79c31fd856ee62332096eb5397ec615a17a90f4"),
		[]byte("sha256:0000000000000000000000000000000000000000000000000000000000000000"),
	)
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(projectLifecyclePath)), lifecycleRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	syncLifecycleFixtureData(t, repositoryRoot, root)
	err = Verify(root)
	if err == nil || !strings.Contains(err.Error(), "legacy release source inventory digest drift") {
		t.Fatalf("legacy inventory digest error = %v", err)
	}
}

func TestVerifyRejectsAnUnaccountedReleaseSource(t *testing.T) {
	t.Parallel()
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "repository")
	clone := exec.Command("git", "clone", "--quiet", "--local", "--no-hardlinks", repositoryRoot, root)
	if output, err := clone.CombinedOutput(); err != nil {
		t.Fatalf("clone lifecycle fixture repository: %v\n%s", err, output)
	}
	if err := os.Remove(filepath.Join(root, "forms", "admission-candidate-set.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	lifecycleRaw, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(projectLifecyclePath)))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(projectLifecyclePath)), lifecycleRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	lifecycleSchemaRaw, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(projectLifecycleSchemaPath)))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(projectLifecycleSchemaPath)), lifecycleSchemaRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	syncLifecycleFixtureData(t, repositoryRoot, root)
	unaccounted := filepath.Join(root, "forms", "releases", "k-unaccounted", "0.1.0")
	if err := os.MkdirAll(unaccounted, 0o755); err != nil {
		t.Fatal(err)
	}
	err = Verify(root)
	if err == nil || !strings.Contains(err.Error(), "release source is outside both Legacy inventory and current lifecycle authority") {
		t.Fatalf("unaccounted release source error = %v", err)
	}
}

// syncLifecycleFixtureData overlays mutable fixture data on a local Git clone.
// The clone supplies immutable historical tag objects while the test exercises
// the current worktree's ledger and Legacy inventory, including while either
// file has an uncommitted edit.
func syncLifecycleFixtureData(t *testing.T, sourceRoot, targetRoot string) {
	t.Helper()
	copyLifecycleSchemaForTest(t, sourceRoot, targetRoot)
	targetProposals := filepath.Join(targetRoot, "proposals")
	if err := os.RemoveAll(targetProposals); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(targetProposals, os.DirFS(filepath.Join(sourceRoot, "proposals"))); err != nil {
		t.Fatal(err)
	}
	epochDecision := filepath.FromSlash("spec/decisions/0006-v1alpha2-restarts-form-lines.md")
	epochDecisionRaw, err := os.ReadFile(filepath.Join(sourceRoot, epochDecision))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetRoot, epochDecision), epochDecisionRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	ledger, err := os.ReadFile(filepath.Join(sourceRoot, filepath.FromSlash(admissioncheckpoint.IdentityLedgerPath)))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetRoot, filepath.FromSlash(admissioncheckpoint.IdentityLedgerPath)), ledger, 0o644); err != nil {
		t.Fatal(err)
	}
	targetReleases := filepath.Join(targetRoot, "forms", "releases")
	if err := os.RemoveAll(targetReleases); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(targetReleases, os.DirFS(filepath.Join(sourceRoot, "forms", "releases"))); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyRequiresEveryHistoricalAdmissionRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, filepath.FromSlash(projectLifecyclePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{
  "format":"takoform.form-lifecycle@v2",
  "projectStatus":"experimental",
  "currentEpoch":"forms.takoform.com/v1alpha2",
  "states":["proposal","experimental","stable","legacy"],
  "legacy":{
    "apiVersion":"forms.takoform.com/v1alpha1",
    "epochDecision":"spec/decisions/0006-v1alpha2-restarts-form-lines.md",
    "decision":"spec/decisions/0004-takoform-is-an-experimental-specification.md",
    "releaseSources":"forms/releases",
    "releaseSourceInventory":{"format":"takoform.legacy-release-inventory@v1","count":71,"digest":"sha256:292495fe4190d077eb993da0e79c31fd856ee62332096eb5397ec615a17a90f4"},
    "historicalAdmissionRoots":["admission/v1","admission/v4"],
    "newCreatePolicy":"host-policy",
    "retainedCapabilities":["read","observe","delete","recovery","migration"]
  },
  "currentForms":[],
  "proposals":[]
}`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	err := Verify(root)
	if err == nil || !strings.Contains(err.Error(), "historicalAdmissionRoots must be exactly admission/v1, admission/v3, admission/v4") {
		t.Fatalf("historical admission roots error = %v", err)
	}
}

func TestVerifyRejectsLifecycleAuthorityThatAllowsLegacyCreates(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, filepath.FromSlash(projectLifecyclePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{
  "format":"takoform.form-lifecycle@v2",
  "projectStatus":"experimental",
  "currentEpoch":"forms.takoform.com/v1alpha2",
  "states":["proposal","experimental","stable","legacy"],
  "legacy":{
    "apiVersion":"forms.takoform.com/v1alpha1",
    "epochDecision":"spec/decisions/0006-v1alpha2-restarts-form-lines.md",
    "decision":"spec/decisions/0004-takoform-is-an-experimental-specification.md",
    "releaseSources":"forms/releases",
    "releaseSourceInventory":{"format":"takoform.legacy-release-inventory@v1","count":71,"digest":"sha256:292495fe4190d077eb993da0e79c31fd856ee62332096eb5397ec615a17a90f4"},
    "historicalAdmissionRoots":["admission/v1","admission/v3","admission/v4"],
    "newCreatePolicy":"allowed",
    "retainedCapabilities":["read","observe","delete","recovery","migration"]
  },
  "currentForms":[],
  "proposals":[]
}`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	err := Verify(root)
	if err == nil || !strings.Contains(err.Error(), `legacy newCreatePolicy is "allowed", want host-policy`) {
		t.Fatalf("legacy create policy error = %v", err)
	}
}

func TestVerifyRequiresLegacyRecoveryCapabilities(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, filepath.FromSlash(projectLifecyclePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{
  "format":"takoform.form-lifecycle@v2",
  "projectStatus":"experimental",
  "currentEpoch":"forms.takoform.com/v1alpha2",
  "states":["proposal","experimental","stable","legacy"],
  "legacy":{
    "apiVersion":"forms.takoform.com/v1alpha1",
    "epochDecision":"spec/decisions/0006-v1alpha2-restarts-form-lines.md",
    "decision":"spec/decisions/0004-takoform-is-an-experimental-specification.md",
    "releaseSources":"forms/releases",
    "releaseSourceInventory":{"format":"takoform.legacy-release-inventory@v1","count":71,"digest":"sha256:292495fe4190d077eb993da0e79c31fd856ee62332096eb5397ec615a17a90f4"},
    "historicalAdmissionRoots":["admission/v1","admission/v3","admission/v4"],
    "newCreatePolicy":"host-policy",
    "retainedCapabilities":["read","observe","delete","migration"]
  },
  "currentForms":[],
  "proposals":[]
}`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	err := Verify(root)
	if err == nil || !strings.Contains(err.Error(), "retainedCapabilities must be exactly read, observe, delete, recovery, migration") {
		t.Fatalf("legacy recovery capability error = %v", err)
	}
}

func TestVerifyPinsTheLifecycleDecision(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, filepath.FromSlash(projectLifecyclePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{
  "format":"takoform.form-lifecycle@v2",
  "projectStatus":"experimental",
  "currentEpoch":"forms.takoform.com/v1alpha2",
  "states":["proposal","experimental","stable","legacy"],
  "legacy":{
    "apiVersion":"forms.takoform.com/v1alpha1",
    "epochDecision":"spec/decisions/0006-v1alpha2-restarts-form-lines.md",
    "decision":"spec/decisions/0001-provider-v1-keeps-form-versions-independent.md",
    "releaseSources":"forms/releases",
    "releaseSourceInventory":{"format":"takoform.legacy-release-inventory@v1","count":71,"digest":"sha256:292495fe4190d077eb993da0e79c31fd856ee62332096eb5397ec615a17a90f4"},
    "historicalAdmissionRoots":["admission/v1","admission/v3","admission/v4"],
    "newCreatePolicy":"host-policy",
    "retainedCapabilities":["read","observe","delete","recovery","migration"]
  },
  "currentForms":[],
  "proposals":[]
}`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	err := Verify(root)
	if err == nil || !strings.Contains(err.Error(), "legacy decision must be spec/decisions/0004-takoform-is-an-experimental-specification.md") {
		t.Fatalf("lifecycle decision error = %v", err)
	}
}

func TestVerifyRejectsCentralAdmissionCandidateAuthority(t *testing.T) {
	t.Parallel()
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "repository")
	clone := exec.Command("git", "clone", "--quiet", "--local", "--no-hardlinks", repositoryRoot, root)
	if output, err := clone.CombinedOutput(); err != nil {
		t.Fatalf("clone lifecycle fixture repository: %v\n%s", err, output)
	}
	lifecycleRaw, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(projectLifecyclePath)))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(projectLifecyclePath)), lifecycleRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	syncLifecycleFixtureData(t, repositoryRoot, root)
	candidatePath := filepath.Join(root, "forms", "admission-candidate-set.json")
	if err := os.WriteFile(candidatePath, []byte(`{"format":"obsolete-central-authority"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	err = Verify(root)
	if err == nil || !strings.Contains(err.Error(), "central admission candidate authority must not exist") {
		t.Fatalf("central admission candidate authority error = %v", err)
	}
}

func TestVerifyRejectsAProposalThatClaimsAFormRef(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, filepath.FromSlash(projectLifecyclePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{
  "format":"takoform.form-lifecycle@v2",
  "projectStatus":"experimental",
  "currentEpoch":"forms.takoform.com/v1alpha2",
  "states":["proposal","experimental","stable","legacy"],
  "legacy":{
    "apiVersion":"forms.takoform.com/v1alpha1",
    "epochDecision":"spec/decisions/0006-v1alpha2-restarts-form-lines.md",
    "decision":"spec/decisions/0004-takoform-is-an-experimental-specification.md",
    "releaseSources":"forms/releases",
    "releaseSourceInventory":{"format":"takoform.legacy-release-inventory@v1","count":71,"digest":"sha256:292495fe4190d077eb993da0e79c31fd856ee62332096eb5397ec615a17a90f4"},
    "historicalAdmissionRoots":["admission/v1","admission/v3","admission/v4"],
    "newCreatePolicy":"host-policy",
    "retainedCapabilities":["read","observe","delete","recovery","migration"]
  },
  "currentForms":[],
  "proposals":[{"id":"p-example","formRef":{"kind":"NotYetAForm"}}]
}`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	err := Verify(root)
	if err == nil || !strings.Contains(err.Error(), `unknown field "formRef"`) {
		t.Fatalf("proposal FormRef authority error = %v", err)
	}
}

func TestVerifyRequiresCompleteOrderedProposalPriorArt(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeLifecycleTestFile(t, root, "proposals/example.md")
	writeLifecycleAuthorityForTest(t, root, `[],`, `[
    {
      "id":"p-example",
      "document":"proposals/example.md",
      "owner":"maintainer:example",
      "consumer":"consumer:example",
      "intendedHosts":["host:example"],
      "workload":"example workload",
      "portableBoundary":"portable desired state only",
      "portableFields":["name"],
      "hostDecisions":["placement"],
      "lifecycleRisks":{"replacement":"reviewed","dataLoss":"reviewed","delete":"reviewed","import":"reviewed","drift":"reviewed"},
      "securityBoundary":{"credentials":"external","network":"host-owned","artifacts":"digest-pinned","secrets":"excluded"},
      "priorArt":[],
      "existingAbstractionGap":"existing APIs do not expose this boundary"
    }
  ]`)
	err := Verify(root)
	if err == nil || !strings.Contains(err.Error(), "must review exactly OCCI, CIMI, TOSCA, Kubernetes/Crossplane, and Terraform/OpenTofu") {
		t.Fatalf("proposal prior-art error = %v", err)
	}
}

func TestVerifyRejectsDirectProposalToStableTransition(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeLifecycleTestFile(t, root, "proposals/example.md")
	writeLifecycleTestFile(t, root, "decisions/proposal.md")
	writeLifecycleTestFile(t, root, "decisions/stable.md")
	writeLifecycleAuthorityForTest(t, root, `[
    {
      "proposalId":"p-example",
      "state":"stable",
      "owner":"maintainer:example",
      "formRef":{"apiVersion":"forms.takoform.com/v1alpha2","kind":"Example","definitionVersion":"1.0.0","schemaDigest":"sha256:0000000000000000000000000000000000000000000000000000000000000000"},
      "packageDigest":"sha256:0000000000000000000000000000000000000000000000000000000000000000",
      "packagePath":"forms/releases/k-example/1.0.0",
      "history":[{"state":"proposal","decision":"decisions/proposal.md"},{"state":"stable","decision":"decisions/stable.md"}],
      "evidence":{}
    }
  ],`, validLifecycleProposalJSON())
	err := Verify(root)
	if err == nil || !strings.Contains(err.Error(), "history must be exactly proposal, experimental, stable") {
		t.Fatalf("direct Proposal-to-Stable error = %v", err)
	}
}

func TestVerifyRequiresExperimentalFormsToUseZeroMajor(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeLifecycleTestFile(t, root, "proposals/example.md")
	writeLifecycleAuthorityForTest(t, root, `[
    {
      "proposalId":"p-example",
      "state":"experimental",
      "owner":"maintainer:example",
      "formRef":{"apiVersion":"forms.takoform.com/v1alpha2","kind":"Example","definitionVersion":"1.0.0","schemaDigest":"sha256:0000000000000000000000000000000000000000000000000000000000000000"},
      "packageDigest":"sha256:0000000000000000000000000000000000000000000000000000000000000000",
      "packagePath":"forms/releases/k-example/1.0.0",
      "history":[],
      "evidence":{}
    }
  ],`, validLifecycleProposalJSON())
	err := Verify(root)
	if err == nil || !strings.Contains(err.Error(), "Experimental Form must use a 0.x definitionVersion") {
		t.Fatalf("Experimental major-version error = %v", err)
	}
}

func writeLifecycleAuthorityForTest(t *testing.T, root, currentForms, proposals string) {
	t.Helper()
	raw := []byte(`{
  "format":"takoform.form-lifecycle@v2",
  "projectStatus":"experimental",
  "currentEpoch":"forms.takoform.com/v1alpha2",
  "states":["proposal","experimental","stable","legacy"],
  "legacy":{
    "apiVersion":"forms.takoform.com/v1alpha1",
    "epochDecision":"spec/decisions/0006-v1alpha2-restarts-form-lines.md",
    "decision":"spec/decisions/0004-takoform-is-an-experimental-specification.md",
    "releaseSources":"forms/releases",
    "releaseSourceInventory":{"format":"takoform.legacy-release-inventory@v1","count":71,"digest":"sha256:292495fe4190d077eb993da0e79c31fd856ee62332096eb5397ec615a17a90f4"},
    "historicalAdmissionRoots":["admission/v1","admission/v3","admission/v4"],
    "newCreatePolicy":"host-policy",
    "retainedCapabilities":["read","observe","delete","recovery","migration"]
  },
  "currentForms":` + currentForms + `
  "proposals":` + proposals + `
}`)
	path := filepath.Join(root, filepath.FromSlash(projectLifecyclePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeLifecycleTestFile(t *testing.T, root, relativePath string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("test evidence\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func validLifecycleProposalJSON() string {
	return `[
    {
      "id":"p-example",
      "document":"proposals/example.md",
      "owner":"maintainer:example",
      "consumer":"consumer:example",
      "intendedHosts":["host:example"],
      "workload":"example workload",
      "portableBoundary":"portable desired state only",
      "portableFields":["name"],
      "hostDecisions":["placement"],
      "lifecycleRisks":{"replacement":"reviewed","dataLoss":"reviewed","delete":"reviewed","import":"reviewed","drift":"reviewed"},
      "securityBoundary":{"credentials":"external","network":"host-owned","artifacts":"digest-pinned","secrets":"excluded"},
      "priorArt":[
        {"name":"OCCI","applicability":"applicable","finding":"reviewed"},
        {"name":"CIMI","applicability":"applicable","finding":"reviewed"},
        {"name":"TOSCA","applicability":"not-applicable","finding":"reviewed"},
        {"name":"Kubernetes/Crossplane","applicability":"applicable","finding":"reviewed"},
        {"name":"Terraform/OpenTofu","applicability":"applicable","finding":"reviewed"}
      ],
      "existingAbstractionGap":"existing APIs do not expose this boundary"
    }
  ]`
}

func TestReleaseSourceRequiresExactReviewedFixtureBytes(t *testing.T) {
	t.Parallel()
	fixtureRoot := filepath.Join("..", "..", "conformance", "form-package-v1", "positive", "standard", "object-bucket")
	releaseRoot := filepath.Join(t.TempDir(), "release")
	if err := os.CopyFS(releaseRoot, os.DirFS(fixtureRoot)); err != nil {
		t.Fatal(err)
	}
	report, err := formpackage.VerifyDirectory(fixtureRoot)
	if err != nil {
		t.Fatal(err)
	}
	entry := InventoryEntry{Kind: "ObjectBucket", FormRef: report.FormRef, PackageDigest: report.PackageDigest}
	if err := verifyReleaseSource(fixtureRoot, releaseRoot, entry); err != nil {
		t.Fatalf("exact release source rejected: %v", err)
	}
	indexPath := filepath.Join(releaseRoot, formpackage.PackageIndexFilename)
	indexRaw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, append(indexRaw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyReleaseSource(fixtureRoot, releaseRoot, entry); err == nil || !strings.Contains(err.Error(), "package-index.json bytes differ") {
		t.Fatalf("non-exact release source error = %v", err)
	}
}

func TestPublishedPackageSetVerifiesIndependentlyOfAdmission(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	if err := VerifyPublishedPackageSet(root); err != nil {
		t.Fatal(err)
	}
	var retired RetiredInventory
	if err := readJSON(filepath.Join(root, filepath.FromSlash(RetiredInventoryPath)), &retired); err != nil {
		t.Fatal(err)
	}
	var published struct {
		DefinitionVersion string `json:"definitionVersion"`
		PackageVersion    string `json:"packageVersion"`
	}
	if err := readJSON(filepath.Join(root, "admission", "v1", "published-package-set.json"), &published); err != nil {
		t.Fatal(err)
	}
	if retired.DefinitionVersion != published.DefinitionVersion || retired.PackageVersion != published.PackageVersion {
		t.Fatalf("retired inventory drifted from published evidence: retired=%s/%s published=%s/%s",
			retired.DefinitionVersion, retired.PackageVersion, published.DefinitionVersion, published.PackageVersion)
	}
	if len(retired.Packages) != len(RetiredKinds) {
		t.Fatalf("retired inventory holds %d packages, want %d", len(retired.Packages), len(RetiredKinds))
	}
	// The rebuilt Forms must not silently reuse a published identity.
	published_ids := map[string]struct{}{}
	for _, entry := range retired.Packages {
		published_ids[entry.FormRef.Kind+"@"+entry.FormRef.DefinitionVersion] = struct{}{}
	}
	var active Inventory
	if err := readJSON(filepath.Join(root, "forms", "standard-package-set.json"), &active); err != nil {
		t.Fatal(err)
	}
	for _, entry := range active.Packages {
		if _, clash := published_ids[entry.FormRef.Kind+"@"+entry.FormRef.DefinitionVersion]; clash {
			t.Fatalf("%s reuses a published definition version", entry.FormRef.Kind)
		}
	}
}

func TestLegacyPortableCandidateSetRetainsAllThirtyFourIdentities(t *testing.T) {
	t.Parallel()
	set, err := LegacyPortableCandidateSet(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if set.Generation != "portable-v1" || len(set.Entries) != 34 {
		t.Fatalf("unexpected Legacy portable-v1 set: generation=%q entries=%d", set.Generation, len(set.Entries))
	}
	kinds := make(map[string]bool, len(set.Entries))
	for _, entry := range set.Entries {
		kinds[entry.Kind] = true
	}
	if !kinds["EdgeWorker"] || kinds["HttpService"] {
		t.Fatalf("Legacy portable-v1 set does not carry the EdgeWorker identity: %#v", kinds)
	}
}

func TestFormSurfaceRegenerationExcludesStandaloneInterfaceResource(t *testing.T) {
	t.Parallel()
	if _, ok := declaredResourceTypes()["takoform_interface"]; ok {
		t.Fatal("Form surface regeneration retained the forbidden standalone interface resource")
	}
}

func TestRetainedGaCoreV1PackageSetAuthenticatesExactLiveReadback(t *testing.T) {
	t.Parallel()
	if err := VerifyRetainedGaCoreV1PublishedPackageSet(filepath.Join("..", "..")); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyGaCoreV2AdmissionEvidenceAuthenticatesItsPublishedIdentity(t *testing.T) {
	if err := verifyHistoricalAdmissionAssignments(filepath.Join("..", "..")); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyPublishedPackageSetAuthenticatesAllThirtyFourIdentities(t *testing.T) {
	t.Parallel()
	set, err := LegacyPublishedPackageSet(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if set.Generation != "portable-v1" ||
		set.PublicationStatus != "published-immutable" ||
		set.AdmissionStatus != "external-required" ||
		len(set.Entries) != 34 {
		t.Fatalf("unexpected current published package set: %#v", set)
	}
}

// TestEveryDeclaredFormHasMaterializableFixtures proves the generated
// fixtures are real input rather than placeholders, for every declared Form.
func TestEveryDeclaredFormHasMaterializableFixtures(t *testing.T) {
	t.Parallel()
	if err := VerifyMaterializableCandidate(filepath.Join("..", "..")); err != nil {
		t.Fatal(err)
	}
	for _, kind := range formcatalog.Kinds {
		desired := kind.CanonicalDesired()
		if desired["name"] == "" {
			t.Fatalf("%s canonical fixture has no name", kind.Kind)
		}
		if _, err := kind.NegativeDesired(); err != nil {
			t.Fatalf("%s has no rejectable counter-example: %v", kind.Kind, err)
		}
		if kind.Artifact {
			source, ok := desired["source"].(map[string]any)
			if !ok || len(source) == 0 {
				t.Fatalf("%s declares an artifact source with no fixture", kind.Kind)
			}
		}
		if kind.Connections == formcatalog.ConnectionsRequired {
			if _, ok := desired["connections"].(map[string]any); !ok {
				t.Fatalf("%s requires a connection but its fixture declares none", kind.Kind)
			}
		}
	}
}

// TestDeclaredFormsOwnPortableRuntimeInterfaceDescriptors proves interface
// declarations stay portable: open names, non-secret documents, and inputs
// resolved only through the Form's own outputs.
func TestDeclaredFormsOwnPortableRuntimeInterfaceDescriptors(t *testing.T) {
	t.Parallel()
	declaredNames := map[string]struct{}{}
	for _, kind := range formcatalog.Kinds {
		descriptors := kind.InterfaceDescriptors()
		if len(descriptors) != len(kind.Interfaces) {
			t.Fatalf("%s descriptor count = %d, want %d", kind.Kind, len(descriptors), len(kind.Interfaces))
		}
		for _, descriptor := range descriptors {
			declaredNames[descriptor.Name] = struct{}{}
			if descriptor.Version != "1" || !descriptor.Required || descriptor.Document == nil {
				t.Fatalf("%s portable descriptor = %#v", kind.Kind, descriptor)
			}
			if strings.Contains(strings.ToLower(descriptor.Name), "takosumi") {
				t.Fatalf("%s descriptor leaks a host identity: %s", kind.Kind, descriptor.Name)
			}
			for _, input := range descriptor.Inputs {
				if !formpackage.PortableInterfaceInputSource(input.Source) {
					t.Fatalf("%s descriptor input is not portable: %#v", kind.Kind, input)
				}
			}
		}
	}
	if len(declaredNames) < 10 {
		t.Fatalf("the portable Form set declares only %d distinct runtime interfaces", len(declaredNames))
	}
}

// TestInventoryCoversEveryDeclaredFormExactlyOnce proves the generated
// inventory is the catalogue, with no dropped or duplicated Form.
func TestInventoryCoversEveryDeclaredFormExactlyOnce(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	var inventory Inventory
	if err := readJSON(filepath.Join(root, "forms", "standard-package-set.json"), &inventory); err != nil {
		t.Fatal(err)
	}
	if len(inventory.Packages) != len(formcatalog.Kinds) {
		t.Fatalf("inventory holds %d packages, want %d declared Forms", len(inventory.Packages), len(formcatalog.Kinds))
	}
	seen := map[string]struct{}{}
	for _, entry := range inventory.Packages {
		declared, ok := formcatalog.ByKind(entry.Kind)
		if !ok {
			t.Fatalf("inventory holds undeclared kind %s", entry.Kind)
		}
		if _, duplicate := seen[entry.Kind]; duplicate {
			t.Fatalf("inventory duplicates %s", entry.Kind)
		}
		seen[entry.Kind] = struct{}{}
		if entry.FormRef.DefinitionVersion != declared.Version() {
			t.Fatalf("%s definition version = %s, want %s", entry.Kind, entry.FormRef.DefinitionVersion, declared.Version())
		}
		if entry.AdmissionStatus != "external-required" {
			t.Fatalf("%s claims admission status %q", entry.Kind, entry.AdmissionStatus)
		}
		report, err := formpackage.VerifyDirectory(filepath.Join(root, filepath.FromSlash(entry.Path)))
		if err != nil {
			t.Fatalf("%s package: %v", entry.Kind, err)
		}
		if report.PackageDigest != entry.PackageDigest {
			t.Fatalf("%s package digest drift", entry.Kind)
		}
	}
}

func TestCandidatePublicationDoesNotActivateStandardForms(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	inventoryPath := filepath.Join(root, "forms", "standard-package-set.json")
	before, err := os.ReadFile(inventoryPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyCandidatePublication(root); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(inventoryPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("candidate publication gate mutated the standard package inventory")
	}
	var inventory Inventory
	if err := readJSON(inventoryPath, &inventory); err != nil {
		t.Fatal(err)
	}
	if inventory.AdmissionStatus != "external-required" || inventory.PublicationReady {
		t.Fatalf("candidate publication changed admission truth: status=%q ready=%v", inventory.AdmissionStatus, inventory.PublicationReady)
	}
	for _, entry := range inventory.Packages {
		if entry.AdmissionStatus != "external-required" {
			t.Fatalf("candidate publication admitted %s", entry.Kind)
		}
	}
}
