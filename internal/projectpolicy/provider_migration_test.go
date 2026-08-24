package projectpolicy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blang/semver"
)

func readJSONFile(t *testing.T, path string, into any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

type providerMigrationAudit struct {
	Format string `json:"format"`
	From   struct {
		ProviderVersion           string `json:"providerVersion"`
		ProviderTag               string `json:"providerTag"`
		SourceCommit              string `json:"sourceCommit"`
		CandidateRefsSHA256       string `json:"candidateRefsSha256"`
		FormResourceCount         int    `json:"formResourceCount"`
		TotalResourceCount        int    `json:"totalResourceCount"`
		WritableInterfaceResource bool   `json:"writableInterfaceResource"`
		OpenTofuProviderAddress   string `json:"openTofuProviderAddress"`
	} `json:"from"`
	To struct {
		ProviderVersion             string `json:"providerVersion"`
		ProviderTag                 string `json:"providerTag"`
		CandidateRefsSHA256         string `json:"candidateRefsSha256"`
		FormResourceCount           int    `json:"formResourceCount"`
		TotalResourceCount          int    `json:"totalResourceCount"`
		ResourceSchemaVersion       int64  `json:"resourceSchemaVersion"`
		WritableInterfaceResource   bool   `json:"writableInterfaceResource"`
		ReadOnlyInterfaceDataSource bool   `json:"readOnlyInterfaceDataSource"`
		CanonicalProviderAddress    string `json:"canonicalProviderAddress"`
	} `json:"to"`
	ExactFormRefs struct {
		FromKindCount            int      `json:"fromKindCount"`
		ToKindCount              int      `json:"toKindCount"`
		CommonKindCount          int      `json:"commonKindCount"`
		ChangedCommonExactRefs   int      `json:"changedCommonExactRefs"`
		UnchangedCommonExactRefs int      `json:"unchangedCommonExactRefs"`
		RemovedKinds             []string `json:"removedKinds"`
		AddedKinds               []string `json:"addedKinds"`
	} `json:"exactFormRefs"`
	StateBoundary struct {
		DirectUpgradeSupported                  bool     `json:"directUpgradeSupported"`
		AutomaticStateTransformationImplemented bool     `json:"automaticStateTransformationImplemented"`
		VersionZeroUpgradeBehavior              string   `json:"versionZeroUpgradeBehavior"`
		ExactIdentityFields                     []string `json:"exactIdentityFields"`
		MissingOrMismatchedIdentityBehavior     string   `json:"missingOrMismatchedIdentityBehavior"`
		CurrentExactNotFoundBehavior            string   `json:"currentExactNotFoundBehavior"`
	} `json:"stateBoundary"`
	ProviderAddressBoundary struct {
		AutomaticAlias                            bool   `json:"automaticAlias"`
		ReplaceOnlyWhenOldAddressIsPresentInState bool   `json:"replaceOnlyWhenOldAddressIsPresentInState"`
		ReplacementCommand                        string `json:"replacementCommand"`
	} `json:"providerAddressBoundary"`
}

func TestCurrentV2MigrationGuideTracksReleaseDescriptor(t *testing.T) {
	root := repositoryRoot(t)
	var release releaseDescriptor
	readStrictJSON(t, filepath.Join(root, "release", "version.json"), &release)

	guideRaw, err := os.ReadFile(filepath.Join(root, "release", "migrations", "v1-to-v2.md"))
	if err != nil {
		t.Fatal(err)
	}
	currentTarget := fmt.Sprintf(
		"v%s is a stable release target whose descriptor remains `candidate-only`",
		release.Version,
	)
	if !strings.Contains(string(guideRaw), currentTarget) {
		t.Fatalf(
			"v1-to-v2 migration guide does not track release/version.json target %s",
			release.Version,
		)
	}
}

func TestV021ToV1MigrationBoundaryStaysFailClosed(t *testing.T) {
	root := repositoryRoot(t)
	var audit providerMigrationAudit
	readStrictJSON(t, filepath.Join(root, "release", "migrations", "v0.2.1-to-v1.0.1.json"), &audit)

	const (
		oldAddress                = "registry.opentofu.org/tako0614/takoform"
		newAddress                = "registry.terraform.io/tako0614/takoform"
		targetCandidateRefsSHA256 = "sha256:d54b838930aabeeac611dab5ec43a89b17512b652964492980f264c80dce5910"
	)
	if audit.Format != "takoform.provider-migration-audit@v1" ||
		audit.From.ProviderVersion != "0.2.1" || audit.From.ProviderTag != "v0.2.1" ||
		audit.From.SourceCommit != "8f408793d39d928f3cc91fe2a3ad3a92e60d5749" ||
		audit.From.CandidateRefsSHA256 != "sha256:f7008ebab73bcdc13261697be5e133c9bd624f6492f1901594dcefca6a91854d" ||
		audit.From.FormResourceCount != 34 || audit.From.TotalResourceCount != 35 ||
		!audit.From.WritableInterfaceResource || audit.From.OpenTofuProviderAddress != oldAddress {
		t.Fatalf("unexpected v0.2.1 migration source: %#v", audit.From)
	}
	if audit.To.ProviderVersion != "1.0.1" || audit.To.ProviderTag != "v1.0.1" ||
		audit.To.CandidateRefsSHA256 != targetCandidateRefsSHA256 ||
		audit.To.FormResourceCount != 34 || audit.To.TotalResourceCount != 34 ||
		audit.To.ResourceSchemaVersion != 1 || audit.To.WritableInterfaceResource ||
		!audit.To.ReadOnlyInterfaceDataSource || audit.To.CanonicalProviderAddress != newAddress {
		t.Fatalf("unexpected provider-v1 migration target: %#v", audit.To)
	}
	if audit.ExactFormRefs.FromKindCount != 34 || audit.ExactFormRefs.ToKindCount != 34 ||
		audit.ExactFormRefs.CommonKindCount != 33 ||
		audit.ExactFormRefs.ChangedCommonExactRefs != 33 ||
		audit.ExactFormRefs.UnchangedCommonExactRefs != 0 ||
		strings.Join(audit.ExactFormRefs.RemovedKinds, ",") != "HttpService" ||
		strings.Join(audit.ExactFormRefs.AddedKinds, ",") != "EdgeWorker" {
		t.Fatalf("exact FormRef transition was weakened: %#v", audit.ExactFormRefs)
	}

	wantIdentityFields := []string{
		"form_api_version",
		"form_kind",
		"form_definition_version",
		"form_schema_digest",
		"form_package_digest",
	}
	if audit.StateBoundary.DirectUpgradeSupported || audit.StateBoundary.AutomaticStateTransformationImplemented ||
		audit.StateBoundary.VersionZeroUpgradeBehavior != "diagnostic-only-no-state-output-no-resource-lifecycle-request" ||
		strings.Join(audit.StateBoundary.ExactIdentityFields, ",") != strings.Join(wantIdentityFields, ",") ||
		audit.StateBoundary.MissingOrMismatchedIdentityBehavior != "diagnostic-before-resource-lifecycle-request-state-retained" ||
		audit.StateBoundary.CurrentExactNotFoundBehavior != "remove-current-state" {
		t.Fatalf("provider-v1 state boundary is not fail-closed: %#v", audit.StateBoundary)
	}
	wantCommand := "tofu state replace-provider " + oldAddress + " " + newAddress
	if audit.ProviderAddressBoundary.AutomaticAlias ||
		!audit.ProviderAddressBoundary.ReplaceOnlyWhenOldAddressIsPresentInState ||
		audit.ProviderAddressBoundary.ReplacementCommand != wantCommand {
		t.Fatalf("provider-address transition is unsafe: %#v", audit.ProviderAddressBoundary)
	}

	// The recorded target digest is a fact about the v1.0.1 boundary, not a
	// claim about today's tree — the Legacy candidate refs it hashed were
	// withdrawn with their epoch (decision 0042), so only the recorded shape
	// is checked here.
	if !strings.HasPrefix(audit.To.CandidateRefsSHA256, "sha256:") ||
		len(audit.To.CandidateRefsSHA256) != len("sha256:")+64 {
		t.Fatalf("migration target candidate digest is malformed: %s", audit.To.CandidateRefsSHA256)
	}

	var release releaseDescriptor
	readStrictJSON(t, filepath.Join(root, "release", "version.json"), &release)
	currentVersion, err := semver.Parse(release.Version)
	if err != nil {
		t.Fatalf("parse current provider version: %v", err)
	}
	migrationTarget, err := semver.Parse(audit.To.ProviderVersion)
	if err != nil {
		t.Fatalf("parse migration target provider version: %v", err)
	}
	if release.Tag != "v"+release.Version ||
		currentVersion.LT(migrationTarget) ||
		release.ProviderAddress != audit.To.CanonicalProviderAddress {
		t.Fatalf(
			"current release %s (%s) does not retain the migration target address/history %s (%s)",
			release.Version,
			release.ProviderAddress,
			audit.To.ProviderVersion,
			audit.To.CanonicalProviderAddress,
		)
	}

	guideRaw, err := os.ReadFile(filepath.Join(root, "release", "migrations", "v0.2.1-to-v1.0.1.md"))
	if err != nil {
		t.Fatal(err)
	}
	guide := string(guideRaw)
	for _, required := range []string{
		"not an in-place provider upgrade",
		"none of the old identities is unchanged",
		"receive `404`",
		"diagnostic-only rejection handler",
		"This guide applies only when the state was written by provider `v0.2.1`",
		"cannot be safely refreshed through `v0.2.1`",
		"makes no Resource lifecycle request",
		"can delete the adopted record",
		"tofu state pull",
		"tofu state replace-provider",
		oldAddress,
		newAddress,
		"separate state/work directory",
		"Do not run `plan`, `apply`, `refresh`, or `import`",
	} {
		if !strings.Contains(guide, required) {
			t.Errorf("migration guide omits required boundary %q", required)
		}
	}

	localPublisherRaw, err := os.ReadFile(filepath.Join(root, "scripts", "release-deploy.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	localPublisher := string(localPublisherRaw)
	providerPublishStart := strings.Index(localPublisher, "function providerPublish(")
	providerPublishEnd := strings.Index(localPublisher, "\nfunction providerRecoveryMutationFence(")
	if providerPublishStart < 0 || providerPublishEnd <= providerPublishStart {
		t.Fatal("cannot locate owner-local provider publication body")
	}
	providerPublish := localPublisher[providerPublishStart:providerPublishEnd]
	if !strings.Contains(providerPublish, "body:") ||
		!strings.Contains(providerPublish, "Breaking upgrade from provider v1") ||
		!strings.Contains(providerPublish, "forms.takoform.com/v1alpha2") ||
		!strings.Contains(providerPublish, "release/migrations/v1-to-v2.md") {
		t.Fatal("owner-local provider publication does not link the breaking v1-to-v2 migration guide")
	}

	resourceDocs, err := filepath.Glob(filepath.Join(root, "docs", "resources", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	var currentIndex struct {
		Format   string `json:"format"`
		Families []struct {
			Group     string `json:"group"`
			FormCount int    `json:"formCount"`
		} `json:"families"`
	}
	readJSONFile(t, filepath.Join(root, "forms", "candidates", "current-family-index.json"), &currentIndex)
	if currentIndex.Format != "takoform.current-family-index@v1" || len(currentIndex.Families) != 8 {
		t.Fatalf("current family index = %q/%d families, want v1/8", currentIndex.Format, len(currentIndex.Families))
	}
	wantResourceDocs := 0
	edgeForms := 0
	for _, family := range currentIndex.Families {
		wantResourceDocs += family.FormCount
		if family.Group == "edge.forms.takoform.com" {
			edgeForms = family.FormCount
		}
	}
	if wantResourceDocs != 31 || edgeForms != 16 {
		t.Fatalf("current family index = %d total/%d Edge Forms, want 31/16", wantResourceDocs, edgeForms)
	}
	// Every Form in the exact eight-family current Provider 3 projection, and
	// nothing else. The Edge family contributes sixteen of the thirty-one;
	// ObjectBucket is retained only in the immutable v1beta1/Provider 2.1.1
	// history, the generic exact-FormRef carrier was withdrawn (decision 0021),
	// and the nine retained provider-v2 docs were withdrawn with their epoch
	// (decision 0042). Every resource document describes a current Form and
	// must state the exact Form identity fields the state boundary fences on.
	if len(resourceDocs) != wantResourceDocs {
		t.Fatalf("current resource docs = %d, want %d", len(resourceDocs), wantResourceDocs)
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "resources", "object_bucket.md")); !os.IsNotExist(err) {
		t.Fatal("current Provider 3 docs must not expose ObjectBucket")
	}
	for _, filename := range resourceDocs {
		raw, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		for _, field := range wantIdentityFields {
			if !strings.Contains(string(raw), "`"+field+"`") {
				t.Errorf("%s omits exact state identity field %s", filename, field)
			}
		}
	}
}

// The v1alpha2 epoch's nine resources were withdrawn with no successors
// (decision 0042), so the release that removes them from the published
// surface is a major. This holds the descriptor to that: it may name the
// published 2.1.1, or the 3.x major the migration boundary requires — never a
// 2.x successor cut by habit.
func TestNextReleaseAfterTheWithdrawalIsAMajor(t *testing.T) {
	root := repositoryRoot(t)
	descriptor := struct {
		Version string `json:"version"`
	}{}
	readJSONFile(t, filepath.Join(root, "release", "version.json"), &descriptor)
	version, err := semver.Parse(descriptor.Version)
	if err != nil {
		t.Fatalf("release/version.json version %q is not SemVer: %v", descriptor.Version, err)
	}
	if descriptor.Version != "2.1.1" && version.Major < 3 {
		t.Fatalf(
			"release/version.json names %s; after the v1alpha2 withdrawal the descriptor stays at the published 2.1.1 until the owner assigns a 3.x major (release/migrations/v2-to-v3.md)",
			descriptor.Version,
		)
	}

	migration := readText(t, filepath.Join(root, "release", "migrations", "v2-to-v3.md"))
	for _, required := range []string{
		"`3.0.0`",
		"takoform_edge_worker",
		"takoform_relational_database",
		"takoform_object_bucket",
		"takoform_key_value_store",
		"takoform_queue",
		"takoform_schedule",
		"takoform_container_service",
		"takoform_stateful_entity",
		"takoform_vector_index",
		"state rm",
		"= 2.1.1",
	} {
		if !strings.Contains(migration, required) {
			t.Errorf("release/migrations/v2-to-v3.md no longer states %q; the migration contract must keep naming the withdrawn resources and every path an operator has", required)
		}
	}
}
