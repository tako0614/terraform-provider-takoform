package standardforms

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/indexedsql"
)

func TestCommittedCandidateSetVerifies(t *testing.T) {
	t.Parallel()
	if err := Verify(filepath.Join("..", "..")); err != nil {
		t.Fatal(err)
	}
}

func TestIndexedSchemaClosureRejectsPackagePathTraversal(t *testing.T) {
	t.Parallel()
	packageRoot := filepath.Join("..", "..", "conformance", "form-package-v1", "positive", "standard", "sql-database-v2")
	descriptor := indexedsql.InterfaceDescriptor()
	schemas := descriptor.Document["schemas"].(map[string]any)
	schemas["request"].(map[string]any)["packagePath"] = "../request.schema.json"
	if err := verifyIndexedSchemaClosure(packageRoot, descriptor); err == nil || !strings.Contains(err.Error(), "path traversal") {
		t.Fatalf("schema closure error = %v, want path traversal rejection", err)
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
	if current.DefinitionVersion != "1.0.1" || current.PackageVersion != "1.0.1" || published.DefinitionVersion != "1.0.1" || published.PackageVersion != "1.0.1" {
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
	if edgeSource["artifactUrl"] != "https://github.com/tako0614/takosumi/releases/download/standard-form-runtime-v1.0.3/edge-worker.mjs" || edgeSource["artifactSha256"] != "281b77f65f6258e56d0468a580b1f67baf9f4d71891c2f7259ce24c47bf7d67e" {
		t.Fatalf("EdgeWorker runtime identity drift: %#v", edgeSource)
	}
	workflow, _ := canonicalDesired("DurableWorkflow")
	workflowSource := workflow["source"].(map[string]any)
	if workflowSource["artifactRef"] != "standard-form-runtime/v1.0.3/durable-workflow.mjs" || workflowSource["artifactSha256"] != "8712e09089276b497669472eddc0aa425c6fa2bf766037f7351690a3517d5ac5" {
		t.Fatalf("DurableWorkflow runtime identity drift: %#v", workflowSource)
	}
	container, _ := canonicalDesired("ContainerService")
	if container["image"] != "docker.io/library/nginx@sha256:845b5424415de5f77dd5753cbb7c1be8bd8e44cc81f20f9705783a02f8848317" {
		t.Fatalf("ContainerService OCI identity drift: %#v", container["image"])
	}
	for _, kind := range []string{"VectorIndex", "DurableWorkflow", "ContainerService", "StatefulActorNamespace"} {
		desired, _ := canonicalDesired(kind)
		if _, present := desired["connections"]; present {
			t.Fatalf("%s canonical fixture retains an optional unsupported connection", kind)
		}
	}
	edgeConnections := edge["connections"].(map[string]any)
	edgeAssets := edgeConnections["ASSETS"].(map[string]any)
	if edgeAssets["resource"] != "ObjectBucket/edge-assets" || edgeAssets["projection"] != "object.binding.v1" {
		t.Fatalf("EdgeWorker portable ObjectBucket projection drift: %#v", edgeAssets)
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

func TestCurrentCandidateOwnsPortableRuntimeInterfaceDescriptors(t *testing.T) {
	t.Parallel()
	expected := map[string]string{
		"EdgeWorker":             "http.request",
		"ObjectBucket":           "object.storage",
		"KVStore":                "keyvalue.store",
		"SQLDatabase":            "sql.query",
		"Queue":                  "queue.messages",
		"VectorIndex":            "vector.query",
		"DurableWorkflow":        "workflow.invoke",
		"ContainerService":       "http.request",
		"StatefulActorNamespace": "actor.invoke",
	}
	for _, spec := range Specs {
		descriptors := standardInterfaceDescriptors(spec.Kind)
		name, exposesRuntimeSurface := expected[spec.Kind]
		if !exposesRuntimeSurface {
			if spec.Kind != "Schedule" || len(descriptors) != 0 {
				t.Fatalf("%s descriptor audit is not explicit", spec.Kind)
			}
			continue
		}
		if len(descriptors) != 1 || descriptors[0].Name != name || descriptors[0].Version != "1" || !descriptors[0].Required {
			t.Fatalf("%s portable descriptor = %#v", spec.Kind, descriptors)
		}
		if strings.Contains(strings.ToLower(descriptors[0].Name), "takosumi") {
			t.Fatalf("%s descriptor leaks a host identity: %s", spec.Kind, descriptors[0].Name)
		}
		for _, input := range descriptors[0].Inputs {
			if !formpackage.PortableInterfaceInputSource(input.Source) {
				t.Fatalf("%s descriptor input is not portable: %#v", spec.Kind, input)
			}
		}
	}
	if len(expected) != len(Specs)-1 {
		t.Fatalf("portable descriptor audit covers %d of %d Forms", len(expected), len(Specs))
	}
	sql := standardInterfaceDescriptors("SQLDatabase")[0]
	if len(sql.Inputs) != 3 || sql.Inputs[2].Name != "engine" || sql.Inputs[2].Pointer != "/engine" {
		t.Fatalf("SQLDatabase query descriptor cannot resolve engine: %#v", sql.Inputs)
	}
}

func TestCurrentCandidatePinsExactTenPackageAndFixtureDigests(t *testing.T) {
	t.Parallel()
	type digests struct {
		packageDigest  string
		desiredDigest  string
		negativeDigest string
	}
	expected := map[string]digests{
		"EdgeWorker":             {"sha256:63e87939660c898837290f0cc8e64e0098106eb4e7cba97a984762d2beaaabb3", "sha256:c1d71aacbfe5df62aabbb22f911304bdb1450c4b898aa7f194a3eaa8a372fbda", "sha256:5bbb54f28e9b30c814443d128698b59e1663db54a01746b6efccd79a1b396ea6"},
		"ObjectBucket":           {"sha256:00f3ed05da56cc795a42043112933ef00e97b531ed123e0c2df3cce70bd1e391", "sha256:02eebf9060e28c799c296f40f0220db4ddbb6fef4da8df32e49983ffee521ae7", "sha256:2fd0b298855cbf98ac4c7844a248ccf7d5261c16af4d74f4c9a0e1c6e713ef11"},
		"KVStore":                {"sha256:964c0bdc7445440f93f2ee6318eb9bf6356f8d749d12a7eb904e90141e312dca", "sha256:bc9df908d5b4083274c41d02491f483808e46c20e945fb0ff5a87faf3ce0e2fa", "sha256:a6e7889fd52a8a3a4fb8a65daf3ffc0bbb35921961cc3c74493037af80f96011"},
		"SQLDatabase":            {"sha256:714a6c3de551598788b25e208783c08b7e8059f10fb96873f5a34b321075c401", "sha256:f4da055dfc9240132a2a8e2a39712df8412210664ce8998a8ce917a7e2e30f51", "sha256:cc9820f9b619e94666879914fc228cc6985b03cb48bbf26c7d2350815a6bc100"},
		"Queue":                  {"sha256:609958d46da6f74f3a29e0940fb628e9c455c353d41d9457791c5859360ad8cc", "sha256:3b5f3a61516403c6d837fa75b36f1b5110663ecac1e3b5c0278f1ef496338df6", "sha256:577bdc1f86bd7329baf3f421263fbc99074e8f8bd684565a5f6d00d5b0bfa0be"},
		"VectorIndex":            {"sha256:adffdb7d630eb44b03c8f4cd4a8ef2295e66318ea81b1fa88be6dfdd20dd429b", "sha256:2bbb58be0ffc411da933428645c0e2f7d79ecc91e31bf06bfc58d3d731825790", "sha256:f077b30f08cca546a8fbff2191ead29348cae75f01d22d3fbd365e0331ee984c"},
		"DurableWorkflow":        {"sha256:05db4bd34b3361eb933ff284f40c1dd17a9f4025c5b5a2ade56a838996a69b90", "sha256:8b0132ff6438b9480c877ee282261da5146c06b6fde80c53ec84101cd17d2018", "sha256:497ab9292ce482d1c8145e0859fff8943a6b5d80e0e1973915a7b62cea5289bf"},
		"ContainerService":       {"sha256:5adfb7dd11a3f462bb2bad94ba9abbcb097c92afa0987d9cc1fa0783d00f3b05", "sha256:182c83da931b52573aedad21294eac9934e2c0c1f42420b00a5e6ac7390c6c6c", "sha256:6808753cc5a0be4bfcf8023dfedfe8d1bb0d8f605c679837db326cc27daf19ed"},
		"StatefulActorNamespace": {"sha256:d6bd34aac538485161ff9045c8a8f9d8dc6193f28f1667cc7bef84a85dbf85d7", "sha256:3a16f86ec2dc943f53c8a2aa389632e66038b6203d8aec96b634da50be13a4e2", "sha256:dc5d84f65fcf623fea1b669f28c1b6bc0263b1a630b61a8b882aad771521cbf3"},
		"Schedule":               {"sha256:ebf03b9e2d90b4fe0444e6dfe824758637639a79e0e267616258e7b28e8d690e", "sha256:a26cb8a1c4c1ddc5c15804eadddad19af27bd9e8c22d0d35d6be0541bef1c1b7", "sha256:76f747035c2587b273e244b5e7cb9ae5f4e10b7237f8cb7b0f1873b98688ba1b"},
	}

	root := filepath.Join("..", "..")
	var inventory Inventory
	if err := readJSON(filepath.Join(root, "forms", "standard-package-set.json"), &inventory); err != nil {
		t.Fatal(err)
	}
	if len(expected) != 10 || len(inventory.Packages) != len(expected) {
		t.Fatalf("exact candidate closure is not ten packages: expected=%d actual=%d", len(expected), len(inventory.Packages))
	}
	for _, entry := range inventory.Packages {
		want, ok := expected[entry.Kind]
		if !ok {
			t.Fatalf("unexpected candidate kind %s", entry.Kind)
		}
		if entry.PackageDigest != want.packageDigest {
			t.Fatalf("%s package digest = %s, want %s", entry.Kind, entry.PackageDigest, want.packageDigest)
		}
		for name, digest := range map[string]string{"desired.json": want.desiredDigest, "negative.json": want.negativeDigest} {
			raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(entry.Path), "fixtures", name))
			if err != nil {
				t.Fatal(err)
			}
			if got := formpackage.DigestBytes(raw); got != digest {
				t.Fatalf("%s %s digest = %s, want %s", entry.Kind, name, got, digest)
			}
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
