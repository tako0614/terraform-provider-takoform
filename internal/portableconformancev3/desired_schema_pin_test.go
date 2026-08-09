package portableconformancev3

// desired_schema_pin_test.go is the teeth of the served-schema pin
// (spec/decisions/0022, amended): the corpus carries the desired-state contract
// of every Form its probes drive, those bytes ARE the installed Definition's at
// the exact pinned FormRef, and a pin that could not fail a host is refused
// before a run starts.
//
// The host-facing half — a host that serves something else — is
// TestServedDesiredSchemaMustBeThePinnedOne in divergent_host_test.go.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCorpusPinsTheFormsOwnDesiredSchema proves the pinned contract IS the
// Form's, at the exact FormRef the corpus pins.
//
// Carrying it is only honest while it cannot drift from the Definition a host
// installs, which is what this compares — the same obligation decision 0025
// accepted when the corpus took on the output contract, for the same reason:
// the corpus states what no wire surface can be asked to re-derive.
func TestCorpusPinsTheFormsOwnDesiredSchema(t *testing.T) {
	root := corpusRoot(t)
	contract, err := Verify(root)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	repoRoot, err := filepath.Abs(filepath.Join(root, "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := LoadCatalog(repoRoot, contract)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	for _, entry := range probeInventory(&contract.RunnerInput) {
		ref := entry.Probe.Identity.FormRef
		form := catalog.exact(ref)
		if form == nil {
			t.Fatalf("the catalog installs no Form at the pinned identity %s", ref.Kind)
		}
		// Canonical bytes, not decoded values: the corpus decodes its numbers as
		// exact literals and the Definition loader need not, and what has to agree
		// is the CONTRACT rather than one loader's Go types.
		pinned, err := canonicalJSON(entry.Probe.DesiredSchema.Schema)
		if err != nil {
			t.Fatal(err)
		}
		declared, err := canonicalJSON(form.DesiredSchema)
		if err != nil {
			t.Fatal(err)
		}
		if pinned != declared {
			t.Fatalf(
				"the corpus pins a %s desired schema the installed Definition does not declare:\npinned: %s\nform:   %s",
				ref.Kind, pinned, declared,
			)
		}
	}
}

// TestRunnerMaterializesFromThePinnedSchema proves the pin is what the runner
// actually sends against, rather than a document it carries and never reads.
//
// The Form is the one in the lane that declares a default at all. If the runner
// took its defaults from the host, this would still pass against the reference
// host and fail against every host with an opinion — so the assertion is made
// where it is decided: on the runner's own materialization, from a contract no
// host was asked about.
func TestRunnerMaterializesFromThePinnedSchema(t *testing.T) {
	contract, err := Verify(corpusRoot(t))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	runner := &v3Runner{contract: contract}
	runner.pinDesiredSchemas()
	probe := contract.RunnerInput.AtLeastOnceQueue
	if _, written := probe.Desired["deliveryDelaySeconds"]; written {
		t.Fatal("the queue probe writes deliveryDelaySeconds, so nothing here measures a declared default")
	}
	materialized := runner.materialize(probe.Identity.FormRef, probe.Desired)
	delay, present := materialized["deliveryDelaySeconds"]
	if !present {
		t.Fatal("the runner sent a spec with no deliveryDelaySeconds; the pinned default was never materialized")
	}
	if fmt.Sprint(delay) != "0" {
		t.Fatalf("deliveryDelaySeconds materialized as %v, want the Form's declared 0", delay)
	}
	// And an exact ref the corpus pins no schema for materializes nothing rather
	// than borrowing another contract's defaults.
	other := probe.Identity.FormRef
	other.DefinitionVersion = "9.9.9"
	if _, present := runner.materialize(other, probe.Desired)["deliveryDelaySeconds"]; present {
		t.Fatal("an unpinned exact ref materialized another contract's default")
	}
}

// TestVerifyRejectsATamperedDesiredSchemaFixture proves the pin is bytes rather
// than a promise.
func TestVerifyRejectsATamperedDesiredSchemaFixture(t *testing.T) {
	root := t.TempDir()
	copyCorpus(t, root)
	path := filepath.Join(root, "fixtures", "desired-schema-at-least-once-queue.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(root); err == nil || !strings.Contains(err.Error(), "byte digest drifted") {
		t.Fatalf("expected desired-schema fixture digest drift, got %v", err)
	}
}

// TestVerifyRejectsAnOpenDesiredSchemaPin proves the corpus refuses a pin no
// host could fail.
//
// An open object schema admits every unexpected property, so a corpus pinning
// one would hold a host to a contract that permits what the lane's own negative
// fixture exists to refuse — and the runner would materialize against it all the
// same. The refusal has to happen at load, before a run reports a pass it did
// not earn.
func TestVerifyRejectsAnOpenDesiredSchemaPin(t *testing.T) {
	root := t.TempDir()
	copyCorpus(t, root)
	path := filepath.Join(root, "fixtures", "desired-schema-at-least-once-queue.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	schema["additionalProperties"] = true
	opened, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, opened, 0o644); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(opened)
	repinCorpus(t, root, func(input map[string]json.RawMessage) {
		probe := map[string]json.RawMessage{}
		if err := json.Unmarshal(input["atLeastOnceQueue"], &probe); err != nil {
			t.Fatal(err)
		}
		pin, err := json.Marshal(map[string]string{
			"path":   "fixtures/desired-schema-at-least-once-queue.json",
			"sha256": "sha256:" + hex.EncodeToString(digest[:]),
		})
		if err != nil {
			t.Fatal(err)
		}
		probe["desiredSchema"] = pin
		drifted, err := json.Marshal(probe)
		if err != nil {
			t.Fatal(err)
		}
		input["atLeastOnceQueue"] = drifted
	})
	if _, err := Verify(root); err == nil || !strings.Contains(err.Error(), "additionalProperties must be false") {
		t.Fatalf("expected an open desired-schema pin to be refused, got %v", err)
	}
}
