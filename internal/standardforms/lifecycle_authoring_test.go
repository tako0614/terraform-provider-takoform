package standardforms

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

func TestLifecycleAuthorityAcceptsCompleteProposal(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	copyLifecycleSchemaForTest(t, filepath.Join("..", ".."), root)
	writeLifecycleTestFile(t, root, "proposals/example.md")
	writeLifecycleAuthorityForTest(t, root, `[],`, validLifecycleProposalJSON())

	authority, err := readProjectLifecycleAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(authority.Proposals) != 1 || authority.Proposals[0].ID != "p-example" || len(authority.CurrentForms) != 0 {
		t.Fatalf("accepted Proposal authority = %#v", authority)
	}
}

func TestLifecycleAuthorityRejectsEmptyProposalDocument(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	copyLifecycleSchemaForTest(t, filepath.Join("..", ".."), root)
	writeLifecycleTestFile(t, root, "proposals/example.md")
	if err := os.WriteFile(filepath.Join(root, "proposals", "example.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	writeLifecycleAuthorityForTest(t, root, `[],`, validLifecycleProposalJSON())

	_, err := readProjectLifecycleAuthority(root)
	if err == nil || !strings.Contains(err.Error(), "must not be empty") {
		t.Fatalf("empty Proposal document error = %v", err)
	}
}

func TestLifecycleAuthorityAcceptsCompleteExperimentalForm(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeExperimentalLifecycleFixture(t, root, true)

	authority, err := readProjectLifecycleAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(authority.CurrentForms) != 1 || authority.CurrentForms[0].State != "experimental" || authority.CurrentForms[0].FormRef.DefinitionVersion != "0.1.0" {
		t.Fatalf("accepted Experimental authority = %#v", authority.CurrentForms)
	}
}

func TestLegacyInventoryAndCurrentLifecyclePackagesCoexistWithoutAuthorityOverlap(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeExperimentalLifecycleFixture(t, root, true)
	authority, err := readProjectLifecycleAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	legacy := publishedReleaseSource{
		AdmissionGeneration: "v1",
		ReleaseID:           releaseIDForKind("LegacyExample"),
		ArtifactID:          "1.0.0",
		Tag:                 "forms/" + releaseIDForKind("LegacyExample") + "/v1.0.0",
		FormRef: formpackage.FormRef{
			APIVersion:        formpackage.FormAPIVersion,
			Kind:              "LegacyExample",
			DefinitionVersion: "1.0.0",
			SchemaDigest:      "sha256:" + strings.Repeat("1", 64),
		},
		PackageDigest: "sha256:" + strings.Repeat("2", 64),
		SourcePath:    "forms/releases/" + releaseIDForKind("LegacyExample") + "/1.0.0",
	}
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(legacy.SourcePath)), 0o755); err != nil {
		t.Fatal(err)
	}
	published := map[string]publishedReleaseSource{
		publishedReleaseKey(legacy.ReleaseID, legacy.ArtifactID): legacy,
	}
	authority.Legacy.ReleaseSourceInventory.Count = len(published)
	authority.Legacy.ReleaseSourceInventory.Digest = legacyInventoryDigestForTest(published)
	if err := verifyLegacyReleaseInventory(root, authority, published); err != nil {
		t.Fatalf("separate Legacy and current lifecycle sources were rejected: %v", err)
	}

	current, err := currentLifecycleReleaseIdentities(root, authority)
	if err != nil || len(current) != 1 {
		t.Fatalf("current identities = %#v, err=%v", current, err)
	}
	overlap := publishedReleaseSource{
		AdmissionGeneration: "v1",
		ReleaseID:           current[0].ReleaseID,
		ArtifactID:          current[0].ArtifactID,
		Tag:                 current[0].Tag,
		FormRef:             current[0].FormRef,
		PackageDigest:       current[0].PackageDigest,
		SourcePath:          current[0].SourcePath,
	}
	overlapping := map[string]publishedReleaseSource{
		publishedReleaseKey(overlap.ReleaseID, overlap.ArtifactID): overlap,
	}
	authority.Legacy.ReleaseSourceInventory.Count = len(overlapping)
	authority.Legacy.ReleaseSourceInventory.Digest = legacyInventoryDigestForTest(overlapping)
	err = verifyLegacyReleaseInventory(root, authority, overlapping)
	if err == nil || !strings.Contains(err.Error(), "reuses immutable Legacy tag") {
		t.Fatalf("current/Legacy authority overlap error = %v", err)
	}
}

func legacyInventoryDigestForTest(published map[string]publishedReleaseSource) string {
	keys := make([]string, 0, len(published))
	for key := range published {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var inventory strings.Builder
	for _, key := range keys {
		source := published[key]
		fmt.Fprintf(
			&inventory,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			source.ReleaseID,
			source.ArtifactID,
			source.FormRef.APIVersion,
			source.FormRef.Kind,
			source.FormRef.DefinitionVersion,
			source.FormRef.SchemaDigest,
			source.PackageDigest,
			source.SourcePath,
		)
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(inventory.String())))
}

func TestLifecycleAuthorityRejectsExperimentalPackageWithoutNegativeFixture(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeExperimentalLifecycleFixture(t, root, false)

	_, err := readProjectLifecycleAuthority(root)
	if err == nil || !strings.Contains(err.Error(), "positive and negative conformance fixtures") {
		t.Fatalf("missing Experimental negative fixture error = %v", err)
	}
}

func TestLifecycleAuthorityRejectsMalformedAuthoringSchema(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeLifecycleTestFile(t, root, "proposals/example.md")
	writeLifecycleAuthorityForTest(t, root, `[],`, validLifecycleProposalJSON())
	if err := os.WriteFile(filepath.Join(root, "forms", "lifecycle.schema.json"), []byte(`{"type":`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := readProjectLifecycleAuthority(root)
	if err == nil || !strings.Contains(err.Error(), "project lifecycle authoring schema") {
		t.Fatalf("malformed lifecycle authoring schema error = %v", err)
	}
}

func TestLifecycleAuthorityRejectsProposalDocumentThroughSymlinkedDirectory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	copyLifecycleSchemaForTest(t, filepath.Join("..", ".."), root)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "example.md"), []byte("outside evidence\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "proposals")); err != nil {
		t.Fatal(err)
	}
	writeLifecycleAuthorityForTest(t, root, `[],`, validLifecycleProposalJSON())

	_, err := readProjectLifecycleAuthority(root)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlinked Proposal directory error = %v", err)
	}
}

func copyLifecycleSchemaForTest(t *testing.T, sourceRoot, destinationRoot string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(sourceRoot, filepath.FromSlash(projectLifecycleSchemaPath)))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(destinationRoot, filepath.FromSlash(projectLifecycleSchemaPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeExperimentalLifecycleFixture(t *testing.T, root string, includeNegative bool) {
	t.Helper()
	copyLifecycleSchemaForTest(t, filepath.Join("..", ".."), root)
	for _, relativePath := range []string{
		"proposals/example.md",
		"decisions/example-proposal.md",
		"decisions/example-experimental.md",
		"evidence/fixtures.md",
		"evidence/host.md",
		"evidence/consumer.md",
		"evidence/known-limitations.md",
		"evidence/compatibility.md",
		"evidence/migration.md",
		"evidence/security-review.md",
		"evidence/documentation.md",
		"evidence/publication-plan.md",
	} {
		writeLifecycleTestFile(t, root, relativePath)
	}

	stagingPath := filepath.Join(root, "form-package-authoring")
	report := writeExperimentalFormPackage(t, stagingPath, includeNegative)
	indexRaw, err := os.ReadFile(filepath.Join(stagingPath, formpackage.PackageIndexFilename))
	if err != nil {
		t.Fatal(err)
	}
	index, err := formpackage.ValidatePackageIndex(indexRaw)
	if err != nil {
		t.Fatal(err)
	}
	locator, err := formpackage.PublicationLocatorFor(index, report.PackageDigest)
	if err != nil {
		t.Fatal(err)
	}
	packagePath := locator.SourcePath
	packageRoot := filepath.Join(root, filepath.FromSlash(packagePath))
	if err := os.MkdirAll(filepath.Dir(packageRoot), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(stagingPath, packageRoot); err != nil {
		t.Fatal(err)
	}
	authority := map[string]any{
		"format":        "takoform.form-lifecycle@v2",
		"projectStatus": "experimental",
		"currentEpoch":  formpackage.CurrentFormAPIVersion,
		"states":        []string{"proposal", "experimental", "stable", "legacy"},
		"legacy": map[string]any{
			"apiVersion":               formpackage.LegacyFormAPIVersion,
			"decision":                 "spec/decisions/0004-takoform-is-an-experimental-specification.md",
			"epochDecision":            "spec/decisions/0006-v1alpha2-restarts-form-lines.md",
			"releaseSources":           "forms/releases",
			"releaseSourceInventory":   map[string]any{"format": "takoform.legacy-release-inventory@v1", "count": 71, "digest": "sha256:292495fe4190d077eb993da0e79c31fd856ee62332096eb5397ec615a17a90f4"},
			"historicalAdmissionRoots": []string{"admission/v1", "admission/v3", "admission/v4"},
			"newCreatePolicy":          "host-policy",
			"retainedCapabilities":     []string{"read", "observe", "delete", "recovery", "migration"},
		},
		"proposals": []any{map[string]any{
			"id":               "p-example",
			"document":         "proposals/example.md",
			"owner":            "maintainer:example",
			"consumer":         "consumer:example",
			"intendedHosts":    []string{"host:example"},
			"workload":         "example workload",
			"portableBoundary": "portable desired state only",
			"portableFields":   []string{"name"},
			"hostDecisions":    []string{"placement"},
			"lifecycleRisks":   map[string]any{"replacement": "reviewed", "dataLoss": "reviewed", "delete": "reviewed", "import": "reviewed", "drift": "reviewed"},
			"securityBoundary": map[string]any{"credentials": "external", "network": "host-owned", "artifacts": "digest-pinned", "secrets": "excluded"},
			"priorArt": []any{
				map[string]any{"name": "OCCI", "applicability": "applicable", "finding": "reviewed"},
				map[string]any{"name": "CIMI", "applicability": "applicable", "finding": "reviewed"},
				map[string]any{"name": "TOSCA", "applicability": "not-applicable", "finding": "reviewed"},
				map[string]any{"name": "Kubernetes/Crossplane", "applicability": "applicable", "finding": "reviewed"},
				map[string]any{"name": "Terraform/OpenTofu", "applicability": "applicable", "finding": "reviewed"},
			},
			"existingAbstractionGap": "existing APIs do not expose this boundary",
		}},
		"currentForms": []any{map[string]any{
			"proposalId":    "p-example",
			"state":         "experimental",
			"owner":         "maintainer:example",
			"formRef":       report.FormRef,
			"packageDigest": report.PackageDigest,
			"packagePath":   packagePath,
			"history": []any{
				map[string]any{"state": "proposal", "decision": "decisions/example-proposal.md"},
				map[string]any{"state": "experimental", "decision": "decisions/example-experimental.md"},
			},
			"evidence": map[string]any{
				"definition":          packagePath + "/definition.json",
				"fixtures":            "evidence/fixtures.md",
				"hostImplementations": []any{map[string]any{"subject": "host:example", "maintainer": "maintainer:host", "evidence": "evidence/host.md"}},
				"realConsumers":       []any{map[string]any{"subject": "consumer:example", "evidence": "evidence/consumer.md"}},
				"knownLimitations":    "evidence/known-limitations.md",
				"compatibility":       "evidence/compatibility.md",
				"migration":           "evidence/migration.md",
				"securityReview":      "evidence/security-review.md",
				"documentation":       "evidence/documentation.md",
				"publicationPlan":     "evidence/publication-plan.md",
			},
		}},
	}
	writeLifecycleJSONForTest(t, filepath.Join(root, filepath.FromSlash(projectLifecyclePath)), authority)
}

func writeExperimentalFormPackage(t *testing.T, packageRoot string, includeNegative bool) formpackage.VerificationReport {
	t.Helper()
	desiredRaw := []byte(`{"name":"example"}`)
	negativeRaw := []byte(`{}`)
	definition := map[string]any{
		"apiVersion":        formpackage.CurrentFormAPIVersion,
		"kind":              "Example",
		"definitionVersion": "0.1.0",
		"title":             "Example Experimental Form",
		"desiredSchema": map[string]any{
			"$schema":              "https://json-schema.org/draft/2020-12/schema",
			"type":                 "object",
			"additionalProperties": false,
			"required":             []string{"name"},
			"properties":           map[string]any{"name": map[string]any{"type": "string", "minLength": 1}},
		},
		"observedSchema": map[string]any{
			"$schema":              "https://json-schema.org/draft/2020-12/schema",
			"type":                 "object",
			"additionalProperties": false,
			"properties":           map[string]any{},
		},
		"lifecycleCapabilities": []string{"create", "read", "update", "delete", "import", "observe", "refresh", "drift"},
		"conformanceFixtures":   []any{map[string]any{"name": "basic", "desiredPath": "fixtures/desired.json"}},
	}
	if includeNegative {
		definition["negativeConformanceFixtures"] = []any{map[string]any{
			"name": "missing-name", "stage": "desired", "inputPath": "fixtures/negative-missing-name.json", "expectedFailure": "schema_validation_failed",
		}}
	}
	definitionRaw := canonicalLifecycleTestJSON(t, definition)
	files := []any{
		lifecyclePackageFile("definition.json", formpackage.DefinitionMediaType, definitionRaw),
		lifecyclePackageFile("fixtures/desired.json", "application/json", desiredRaw),
	}
	if includeNegative {
		files = append(files, lifecyclePackageFile("fixtures/negative-missing-name.json", "application/json", negativeRaw))
	}
	index := map[string]any{
		"apiVersion": formpackage.CurrentPackageAPIVersion,
		"kind":       formpackage.PackageKind,
		"formRef": map[string]any{
			"apiVersion":        formpackage.CurrentFormAPIVersion,
			"kind":              "Example",
			"definitionVersion": "0.1.0",
			"schemaDigest":      formpackage.DigestBytes(definitionRaw),
		},
		"definitionPath": "definition.json",
		"files":          files,
	}
	writeLifecycleBytesForTest(t, filepath.Join(packageRoot, "definition.json"), definitionRaw)
	writeLifecycleBytesForTest(t, filepath.Join(packageRoot, "fixtures", "desired.json"), desiredRaw)
	if includeNegative {
		writeLifecycleBytesForTest(t, filepath.Join(packageRoot, "fixtures", "negative-missing-name.json"), negativeRaw)
	}
	writeLifecycleBytesForTest(t, filepath.Join(packageRoot, formpackage.PackageIndexFilename), canonicalLifecycleTestJSON(t, index))
	report, err := formpackage.VerifyDirectory(packageRoot)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func lifecyclePackageFile(path, mediaType string, raw []byte) map[string]any {
	return map[string]any{"path": path, "mediaType": mediaType, "size": len(raw), "digest": formpackage.DigestBytes(raw)}
}

func canonicalLifecycleTestJSON(t *testing.T, value any) []byte {
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

func writeLifecycleJSONForTest(t *testing.T, path string, value any) {
	t.Helper()
	writeLifecycleBytesForTest(t, path, canonicalLifecycleTestJSON(t, value))
}

func writeLifecycleBytesForTest(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}
