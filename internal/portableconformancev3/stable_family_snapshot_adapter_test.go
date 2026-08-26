package portableconformancev3

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stableTableWitnessFixture(t *testing.T) (string, string, stableFamilyCorpus, stableCandidateSet) {
	t.Helper()
	root, err := repositoryRootForContract(".")
	if err != nil {
		t.Fatal(err)
	}
	corpusPath := filepath.Join(root, "conformance", "takoform-v1", "families", "table.forms.takoform.com.json")
	var corpus stableFamilyCorpus
	if _, err := stableReadStrict(corpusPath, &corpus); err != nil {
		t.Fatal(err)
	}
	candidatePath, err := stableResolve(root, corpusPath, corpus.CandidateSet.Path)
	if err != nil {
		t.Fatal(err)
	}
	var set stableCandidateSet
	if _, err := stableReadStrict(candidatePath, &set); err != nil {
		t.Fatal(err)
	}
	return root, corpusPath, corpus, set
}

func TestStableRunCurrentNonEdgeFamilyWitnessCompilesSnapshotAndRunsLifecycle(t *testing.T) {
	root, corpusPath, corpus, set := stableTableWitnessFixture(t)
	if err := stableRunCurrentNonEdgeFamilyWitness(root, corpusPath, corpus, set); err != nil {
		t.Fatalf("table Snapshot-backed lifecycle witness: %v", err)
	}
}

func TestStableRunCurrentNonEdgeFamilyWitnessRejectsWrongExactRef(t *testing.T) {
	root, corpusPath, corpus, set := stableTableWitnessFixture(t)
	set.Forms[0].FormRef.SchemaDigest = "sha256:" + strings.Repeat("0", 64)
	if err := stableRunCurrentNonEdgeFamilyWitness(root, corpusPath, corpus, set); err == nil || !strings.Contains(err.Error(), "FormRef") {
		t.Fatalf("wrong exact FormRef was accepted: %v", err)
	}
}

func TestStableRunCurrentNonEdgeFamilyWitnessRejectsLifecycleCapabilityDrift(t *testing.T) {
	root, corpusPath, corpus, set := stableTableWitnessFixture(t)
	probe := corpus.RunnerInput["table"]
	probe.LifecycleCapabilities = append([]string(nil), probe.LifecycleCapabilities[:len(probe.LifecycleCapabilities)-1]...)
	corpus.RunnerInput["table"] = probe
	if err := stableRunCurrentNonEdgeFamilyWitness(root, corpusPath, corpus, set); err == nil || !strings.Contains(err.Error(), "lifecycle capabilities drifted") {
		t.Fatalf("lifecycle capability drift was accepted: %v", err)
	}
}

func TestStableCurrentFamilySnapshotAdapterIsDataOnly(t *testing.T) {
	raw, err := os.ReadFile("stable_family_snapshot_adapter.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"ReferenceHost", "v3Runner", "stableGeneric", "currentformregistry", "edgeformcatalog", "functionformcatalog", "tableformcatalog",
	} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("Snapshot family adapter contains forbidden implementation dependency %q", forbidden)
		}
	}
}
