package portableconformancev3

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
)

const (
	stableLegacyCoverageFormat         = "takoform.legacy-portable-coverage@v1"
	stableLegacyCoverageOwner          = "edge-family-concrete-host"
	stableLegacyCoverageRepositoryPath = "conformance/takoform-v1/family-host/edge/coverage-ledger.json"
	stableLegacyEdgeContractPath       = "portable-host/contract.json"
	stableLegacyEdgeContractSHA256     = "1c4281e54ea986ec9cfacc74ef5d384142b620a63fb19331b514eb1f2f31def7"
	stableLegacyRehomeRepositoryPath   = "conformance/takoform-v1/family-host/edge/rehome-ledger.json"
	stableLegacyRehomeSHA256           = "0aa8a9b1e6293d02ab1e0022d38033c7cf887819f4b37e59d59e4cb9ffab7b25"

	stableLegacyOwnerGenericHost      = "generic-host"
	stableLegacyOwnerFamilySemantic   = "family-semantic"
	stableLegacyOwnerInterfaceRuntime = "interface-runtime"
	stableLegacyOwnerComposition      = "composition"
	stableLegacyOwnerConcreteHost     = "concrete-host"
)

type stableLegacyPortableCoverage struct {
	Check            string   `json:"check"`
	Owner            string   `json:"owner"`
	EdgeAdapterCheck string   `json:"edgeAdapterCheck"`
	NeutralChecks    []string `json:"neutralChecks,omitempty"`
}

type stableLegacyCoverageLedger struct {
	Format     string                         `json:"format"`
	Owner      string                         `json:"owner"`
	Contract   stableDigestRecord             `json:"contract"`
	CheckCount int                            `json:"checkCount"`
	Coverage   []stableLegacyPortableCoverage `json:"coverage"`
}

type stableLegacyRehomeEntry struct {
	OldPath string `json:"oldPath"`
	NewPath string `json:"newPath"`
	SHA256  string `json:"sha256"`
}

type stableLegacyRehomeLedger struct {
	Format    string                    `json:"format"`
	FileCount int                       `json:"fileCount"`
	Files     []stableLegacyRehomeEntry `json:"files"`
}

func stableLegacyExpectedChecks() []string {
	return append(append([]string(nil), stableRequiredChecks...), "class-holder-rules-enforced")
}

// stableVerifyLegacyPortableCoverage proves that moving the old stable-v1
// 125-check adapter out of generic-host did not delete or silently rename one
// check. The adapter still executes its complete matrix, but it is owned and
// loaded as Edge family/concrete-Host evidence rather than generic evidence.
func stableVerifyLegacyPortableCoverage(ctx context.Context, repositoryRoot string) error {
	if err := stableVerifyLegacyEdgeRehome(repositoryRoot); err != nil {
		return err
	}
	ledgerPath := filepath.Join(repositoryRoot, filepath.FromSlash(stableLegacyCoverageRepositoryPath))
	var ledger stableLegacyCoverageLedger
	if _, err := stableReadStrict(ledgerPath, &ledger); err != nil {
		return err
	}
	want := stableLegacyExpectedChecks()
	if ledger.Format != stableLegacyCoverageFormat || ledger.Owner != stableLegacyCoverageOwner ||
		ledger.Contract.Path != stableLegacyEdgeContractPath || ledger.CheckCount != 125 ||
		ledger.Contract.SHA256 != stableLegacyEdgeContractSHA256 ||
		len(ledger.Coverage) != len(want) || len(want) != 125 {
		return errors.New("stable legacy portable coverage ledger identity drifted")
	}
	neutral := make(map[string]bool, len(stableGenericRequiredChecks))
	for _, check := range stableGenericRequiredChecks {
		neutral[check] = true
	}
	for index, expected := range want {
		entry := ledger.Coverage[index]
		if entry.Check != expected || entry.EdgeAdapterCheck != expected ||
			entry.Owner != stableLegacyCheckOwner(expected) {
			return fmt.Errorf("stable legacy portable coverage[%d] identity or owner drifted", index)
		}
		for evidenceIndex, check := range entry.NeutralChecks {
			if !neutral[check] || (evidenceIndex > 0 && entry.NeutralChecks[evidenceIndex-1] >= check) {
				return fmt.Errorf("stable legacy check %s names invalid neutral evidence %s", expected, check)
			}
		}
	}

	contractPath, err := stableResolve(repositoryRoot, ledgerPath, ledger.Contract.Path)
	if err != nil {
		return err
	}
	if _, err := stableVerifyDigest(contractPath, stableLegacyEdgeContractSHA256); err != nil {
		return err
	}
	contract, err := Verify(filepath.Dir(contractPath))
	if err != nil {
		return fmt.Errorf("verify stable Edge family Host adapter: %w", err)
	}
	if !reflect.DeepEqual(contract.RequiredRunnerChecks, want) {
		return errors.New("stable Edge family Host adapter no longer carries the old 125-check matrix")
	}
	report, err := SelfTest(ctx, contract)
	if err != nil {
		return fmt.Errorf("stable Edge family Host adapter: %w", err)
	}
	if report.Status != "passed" || !reflect.DeepEqual(report.Checks, want) {
		return errors.New("stable Edge family Host adapter did not execute the old 125-check matrix")
	}
	return nil
}

func stableVerifyLegacyEdgeRehome(repositoryRoot string) error {
	ledgerPath := filepath.Join(repositoryRoot, filepath.FromSlash(stableLegacyRehomeRepositoryPath))
	var ledger stableLegacyRehomeLedger
	raw, err := stableReadStrict(ledgerPath, &ledger)
	if err != nil {
		return err
	}
	if stableDigest(raw) != stableLegacyRehomeSHA256 || ledger.Format != "takoform.edge-host-rehome@v1" ||
		ledger.FileCount != 28 || len(ledger.Files) != 28 {
		return errors.New("stable Edge Host rehome ledger identity or hard-pinned digest drifted")
	}
	const oldRoot = "conformance/takoform-v1/generic-host/portable-host/"
	const newRoot = "conformance/takoform-v1/family-host/edge/portable-host/"
	contractSeen := false
	for index, entry := range ledger.Files {
		if index > 0 && ledger.Files[index-1].NewPath >= entry.NewPath {
			return errors.New("stable Edge Host rehome ledger paths are not sorted and unique")
		}
		if len(entry.OldPath) <= len(oldRoot) || len(entry.NewPath) <= len(newRoot) ||
			entry.OldPath[:len(oldRoot)] != oldRoot || entry.NewPath[:len(newRoot)] != newRoot ||
			entry.OldPath[len(oldRoot):] != entry.NewPath[len(newRoot):] {
			return fmt.Errorf("stable Edge Host rehome[%d] is not an exact old-to-new path mapping", index)
		}
		oldPath, err := stableResolve(repositoryRoot, ledgerPath, entry.OldPath)
		if err != nil {
			return err
		}
		newPath, err := stableResolve(repositoryRoot, ledgerPath, entry.NewPath)
		if err != nil {
			return err
		}
		oldRaw, err := stableVerifyDigest(oldPath, entry.SHA256)
		if err != nil {
			return err
		}
		newRaw, err := stableVerifyDigest(newPath, entry.SHA256)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(oldRaw, newRaw) {
			return fmt.Errorf("stable Edge Host rehome %s changed bytes", entry.NewPath)
		}
		if entry.NewPath == newRoot+"contract.json" {
			if entry.SHA256 != stableLegacyEdgeContractSHA256 {
				return errors.New("stable Edge Host rehome contract digest drifted from the pre-move hard pin")
			}
			contractSeen = true
		}
	}
	if !contractSeen {
		return errors.New("stable Edge Host rehome ledger omitted contract.json")
	}
	return nil
}

func stableLegacyCheckOwner(check string) string {
	switch check {
	case "module-worker-runtime-contract-advertised",
		"undeclared-runtime-handler-rejected",
		"declared-handler-not-exported-rejected",
		"edge-interface-contracts-advertised",
		"support-profiles-present":
		return stableLegacyOwnerInterfaceRuntime
	case "static-asset-spa-paths",
		"sqlite-migration-ledger-readiness",
		"artifact-manifest-reject-list",
		"artifact-manifest-kind-exclusive",
		"manifest-reference-is-not-a-capability",
		"cron-grammar-enforced",
		"queue-single-consumer-enforced",
		"custom-domain-hostname-canonicalized",
		"custom-domain-hostname-claim-unique",
		"custom-domain-hostname-claim-stops-at-the-tenant",
		"dead-letter-cycle-rejected",
		"attachment-claim-decided-on-import",
		"attachment-claim-revalidated-at-commit",
		"custom-domain-u-label-refused",
		"bundle-main-module-is-loadable",
		"class-holder-rules-enforced":
		return stableLegacyOwnerFamilySemantic
	case "deployment-weight-sum-enforced",
		"binding-target-missing-404-before-mutation",
		"dependency-in-use-on-bound-target-delete",
		"relation-target-missing-rejected",
		"relation-target-deletion-blocked",
		"relation-incarnation-change-detected",
		"relation-reapply-repins",
		"binding-contract-verified",
		"artifact-then-bundle-apply",
		"artifact-retention-while-referenced",
		"deployment-single-active-per-worker",
		"deployment-version-ownership",
		"deployment-version-duplicate-rejected",
		"attachment-requires-active-deployment",
		"handler-gated-attachments",
		"binding-name-collision-rejected",
		"deployment-change-preserves-dependents",
		"deployment-delete-blocked-by-dependent",
		"deployment-delete-blocked-by-inbound-binding",
		"dependent-revision-advances-with-rendering",
		"delete-fence-survives-derived-rendering",
		"worker-endpoint-address-is-host-assigned",
		"worker-endpoint-single-per-worker",
		"worker-endpoint-follows-the-active-deployment",
		"worker-endpoint-address-is-stable-for-its-uid",
		"relation-target-form-ref-verified",
		"relation-target-interface-verified",
		"relation-pin-records-target-form-ref",
		"relation-resolution-is-tenant-scoped":
		return stableLegacyOwnerComposition
	default:
		if genericContainsString(stableGenericRequiredChecks, check) {
			return stableLegacyOwnerGenericHost
		}
		return stableLegacyOwnerConcreteHost
	}
}
