package standardforms

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

func TestCommittedCandidateSetVerifies(t *testing.T) {
	t.Parallel()
	if err := Verify(filepath.Join("..", "..")); err != nil {
		t.Fatal(err)
	}
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

func TestAdmissionActivationGateFailsClosedWithoutExternalAdmission(t *testing.T) {
	t.Parallel()
	err := VerifyReleaseReady(filepath.Join("..", ".."))
	if err == nil || !strings.Contains(err.Error(), "missing admission/v1/standard-admission-set.json") {
		t.Fatalf("release gate error = %v", err)
	}
}

func TestPublishedPackageSetVerifiesWithoutAdmittingForms(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	if err := VerifyPublishedPackageSet(root); err != nil {
		t.Fatal(err)
	}
	var current Inventory
	if err := readJSON(filepath.Join(root, "forms", "standard-package-set.json"), &current); err != nil {
		t.Fatal(err)
	}
	var published struct {
		DefinitionVersion string `json:"definitionVersion"`
		PackageVersion    string `json:"packageVersion"`
	}
	if err := readJSON(filepath.Join(root, "admission", "v1", "published-package-set.json"), &published); err != nil {
		t.Fatal(err)
	}
	if current.DefinitionVersion != "1.0.1" || current.PackageVersion != "1.0.1" || published.DefinitionVersion != "1.0.0" || published.PackageVersion != "1.0.0" {
		t.Fatalf("candidate/publication window drift: current=%s/%s published=%s/%s", current.DefinitionVersion, current.PackageVersion, published.DefinitionVersion, published.PackageVersion)
	}
	err := VerifyReleaseReady(root)
	if err == nil || !strings.Contains(err.Error(), "missing admission/v1/standard-admission-set.json") {
		t.Fatalf("published package readback opened admission: %v", err)
	}
}

func TestCurrentCandidatePinsRealRuntimeAndMaterializableDefaults(t *testing.T) {
	t.Parallel()
	edge, err := canonicalDesired("EdgeWorker")
	if err != nil {
		t.Fatal(err)
	}
	edgeSource := edge["source"].(map[string]any)
	if edgeSource["artifactUrl"] != "https://github.com/tako0614/takosumi/releases/download/standard-form-runtime-v1.0.2/edge-worker.mjs" || edgeSource["artifactSha256"] != "281b77f65f6258e56d0468a580b1f67baf9f4d71891c2f7259ce24c47bf7d67e" {
		t.Fatalf("EdgeWorker runtime identity drift: %#v", edgeSource)
	}
	workflow, _ := canonicalDesired("DurableWorkflow")
	workflowSource := workflow["source"].(map[string]any)
	if workflowSource["artifactRef"] != "standard-form-runtime/v1.0.2/durable-workflow.mjs" || workflowSource["artifactSha256"] != "8712e09089276b497669472eddc0aa425c6fa2bf766037f7351690a3517d5ac5" {
		t.Fatalf("DurableWorkflow runtime identity drift: %#v", workflowSource)
	}
	container, _ := canonicalDesired("ContainerService")
	if container["image"] != "docker.io/library/nginx@sha256:845b5424415de5f77dd5753cbb7c1be8bd8e44cc81f20f9705783a02f8848317" {
		t.Fatalf("ContainerService OCI identity drift: %#v", container["image"])
	}
	for _, kind := range []string{"EdgeWorker", "VectorIndex", "DurableWorkflow", "ContainerService", "StatefulActorNamespace"} {
		desired, _ := canonicalDesired(kind)
		if _, present := desired["connections"]; present {
			t.Fatalf("%s canonical fixture retains an optional unsupported connection", kind)
		}
	}
	kv, _ := canonicalDesired("KVStore")
	queue, _ := canonicalDesired("Queue")
	database, _ := canonicalDesired("SQLDatabase")
	if kv["consistency"] != "eventual" || queue["delivery"] != nil || database["migrationsPath"] != nil {
		t.Fatalf("canonical managed-target defaults are not materializable: kv=%#v queue=%#v database=%#v", kv, queue, database)
	}
	schedule, _ := canonicalDesired("Schedule")
	if _, present := schedule["connections"]; !present {
		t.Fatal("Schedule canonical fixture lost its required workflow connection")
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
