package projectpolicy

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

	candidateRaw, err := os.ReadFile(filepath.Join(root, "internal", "formregistry", "candidate-refs.json"))
	if err != nil {
		t.Fatal(err)
	}
	currentDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(candidateRaw))
	if audit.To.CandidateRefsSHA256 != currentDigest {
		t.Fatalf("migration target candidate digest = %s, current set = %s", audit.To.CandidateRefsSHA256, currentDigest)
	}
	var candidates map[string]json.RawMessage
	if err := json.Unmarshal(candidateRaw, &candidates); err != nil {
		t.Fatal(err)
	}
	if len(candidates) != audit.To.FormResourceCount {
		t.Fatalf("migration target count = %d, current candidate count = %d", audit.To.FormResourceCount, len(candidates))
	}

	var release releaseDescriptor
	readStrictJSON(t, filepath.Join(root, "release", "version.json"), &release)
	if release.Version != audit.To.ProviderVersion || release.Tag != audit.To.ProviderTag ||
		release.ProviderAddress != audit.To.CanonicalProviderAddress {
		t.Fatalf("migration target differs from release descriptor")
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
	providerPublishEnd := strings.Index(localPublisher, "\nfunction providerReadback(")
	if providerPublishStart < 0 || providerPublishEnd <= providerPublishStart {
		t.Fatal("cannot locate owner-local provider publication body")
	}
	providerPublish := localPublisher[providerPublishStart:providerPublishEnd]
	if !strings.Contains(providerPublish, "body:") ||
		!strings.Contains(providerPublish, "Breaking upgrade from v0.2.1") ||
		!strings.Contains(providerPublish, "release/migrations/v0.2.1-to-v1.0.1.md") {
		t.Fatal("owner-local provider publication does not link the breaking v0.2.1-to-v1.0.1 migration guide")
	}

	resourceDocs, err := filepath.Glob(filepath.Join(root, "docs", "resources", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(resourceDocs) != audit.To.FormResourceCount {
		t.Fatalf("resource docs = %d, want %d", len(resourceDocs), audit.To.FormResourceCount)
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
