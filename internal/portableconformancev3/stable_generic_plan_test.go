package portableconformancev3

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStableGenericPlanRunsThroughIndependentMemoryAndHTTPAdapters(t *testing.T) {
	seed, plan := loadStableGenericPlanTestFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	completed, err := stableRunGenericPlan(
		ctx, seed, plan, stableGenericRequiredChecks,
		genericMemoryFactory, genericHTTPFactory,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(completed) != len(stableGenericRequiredChecks) {
		t.Fatalf("typed plan completed %d checks, want %d", len(completed), len(stableGenericRequiredChecks))
	}
	for _, check := range stableGenericRequiredChecks {
		if !completed[check] {
			t.Fatalf("typed plan did not complete %q", check)
		}
	}
}

func loadStableGenericPlanTestFixture(t *testing.T) (genericPlanSeed, genericPlan) {
	t.Helper()
	repositoryRoot, err := repositoryRootForContract(".")
	if err != nil {
		t.Fatal(err)
	}
	corpusPath := filepath.Join(repositoryRoot, "conformance", "takoform-v1", "generic.json")
	var corpus stableGenericCorpus
	if _, err := stableReadStrict(corpusPath, &corpus); err != nil {
		t.Fatal(err)
	}
	compiled, _, err := stableCompileGenericSnapshots(repositoryRoot, corpusPath, corpus)
	if err != nil {
		t.Fatal(err)
	}
	external := compiled[corpus.ExternalHostProbe.Snapshot]
	if external.snapshot == nil {
		t.Fatal("generic test fixture has no external Snapshot")
	}
	contract, err := stableGenericHostContract(corpus, corpus.ExternalHostProbe, external.snapshot)
	if err != nil {
		t.Fatal(err)
	}
	seed := genericPlanSeed{
		Snapshot: external.snapshot, Contract: contract,
		Probe: corpus.ExternalHostProbe, Artifact: corpus.ExternalHostProbe.ArtifactTransport,
	}
	plan, err := stableBuildGenericPlan(seed)
	if err != nil {
		t.Fatal(err)
	}
	return seed, plan
}

func TestStableGenericPlanDetectsMemoryTransitionDivergence(t *testing.T) {
	seed, plan := loadStableGenericPlanTestFixture(t)
	brokenMemory := func(seed genericPlanSeed) (genericPlanAdapter, func(), error) {
		adapter, err := newGenericMemoryAdapter(seed)
		if err != nil {
			return nil, nil, err
		}
		adapter.faults.keepGenerationOnUpdate = true
		return adapter, nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	_, err := stableRunGenericPlan(ctx, seed, plan, stableGenericRequiredChecks, brokenMemory, genericHTTPFactory)
	if err == nil || !strings.Contains(err.Error(), "apply-primary-update") {
		t.Fatalf("generic plan did not catch a broken memory generation transition: %v", err)
	}
}

func TestStableGenericPlanDetectsHTTPHostTransitionDivergence(t *testing.T) {
	seed, plan := loadStableGenericPlanTestFixture(t)
	brokenHTTP := func(seed genericPlanSeed) (genericPlanAdapter, func(), error) {
		return genericHTTPFactoryWithHost(seed, func(host *ReferenceHost) {
			host.genericPlanFaultKeepGenerationOnUpdate = true
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	_, err := stableRunGenericPlan(ctx, seed, plan, stableGenericRequiredChecks, genericMemoryFactory, brokenHTTP)
	if err == nil || !strings.Contains(err.Error(), "apply-primary-update") {
		t.Fatalf("generic plan did not catch a broken HTTP Host generation transition: %v", err)
	}
}

func TestStableGenericPlanDetectsHTTPDeclaredConstraintDivergence(t *testing.T) {
	seed, plan := loadStableGenericPlanTestFixture(t)
	tests := []struct {
		constraint string
		caseID     string
	}{
		{constraint: "sum", caseID: "constraint-sum-invalid"},
		{constraint: "claim", caseID: "constraint-claim-alternate-conflict"},
		{constraint: "hostAssigned", caseID: "constraint-host-assigned-substitute"},
		{constraint: "exclusive", caseID: "constraint-lease-second-exact-form"},
	}
	for _, test := range tests {
		t.Run(test.constraint, func(t *testing.T) {
			brokenHTTP := func(seed genericPlanSeed) (genericPlanAdapter, func(), error) {
				return genericHTTPFactoryWithHost(seed, func(host *ReferenceHost) {
					host.genericPlanFaultDeclaredConstraint = test.constraint
				})
			}
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			_, err := stableRunGenericPlan(
				ctx, seed, plan, stableGenericRequiredChecks, genericMemoryFactory, brokenHTTP,
			)
			if err == nil || !strings.Contains(err.Error(), test.caseID) {
				t.Fatalf("generic plan did not catch broken %s semantics at %s: %v", test.constraint, test.caseID, err)
			}
		})
	}
}
