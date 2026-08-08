package edgeformcatalog

import (
	"reflect"
	"sort"
	"testing"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// desiredSchemaFor renders one catalog Form's desired schema with its
// target-contract annotations resolved exactly as the generation pipeline
// resolves them.
func desiredSchemaFor(t *testing.T, form model.Form) map[string]any {
	t.Helper()
	schema, err := form.DesiredSchema(newTargetContractResolver())
	if err != nil {
		t.Fatalf("%s desired schema: %v", form.Kind, err)
	}
	return schema
}

// wantRelations is the complete cross-resource relation table of the Edge
// Platform Family, written out by hand. Relations are DERIVED from each Form's
// desired schema, so this table is the independent statement of what the
// catalog declares: a reference added, removed, or retargeted without a
// deliberate edit here fails the family.
//
// Every entry is "<pointer> -> <target kind>" with "*" standing for an array
// element, plus the Binding contract when the reference is a typed binding and
// "-" when it is a plain cross-resource reference, the requiredness, and the
// TARGET CONTRACT the reference states: "exact-form" when the relation depends
// on the target's exact desired contract, "iface:<name>@<version>" when it
// depends only on an Interface the target provides (decision 0022).
//
// The target contract belongs in this table for the same reason the target kind
// does: it is a normative statement about what each relation requires, and a
// silent flip from one to the other would either refuse a legitimate target or
// admit one whose contract no longer answers.
var wantRelations = map[string][]string{
	"ModuleWorker": nil,
	"WorkerBundle": nil,
	"WorkerVersion": {
		"/bucketBindings/*/resource -> ObjectBucket [module-worker.object-bucket] optional iface:edge.objects@1.0.0",
		"/bundle -> WorkerBundle [-] required exact-form",
		"/kvBindings/*/resource -> EdgeKVNamespace [module-worker.edge-kv] optional iface:edge.kv@1.0.0",
		"/queueProducerBindings/*/resource -> AtLeastOnceQueue [module-worker.queue-producer] optional iface:edge.queue@1.0.0",
		"/serviceBindings/*/resource -> ModuleWorker [module-worker.service] optional iface:worker.service@1.0.0",
		"/sqliteBindings/*/resource -> SQLiteDatabase [module-worker.sqlite] optional iface:edge.sql@1.0.0",
		"/worker -> ModuleWorker [-] required iface:worker.runtime@1.0.0",
	},
	"WorkerDeployment": {
		"/versions/*/workerVersion -> WorkerVersion [-] required exact-form",
		"/worker -> ModuleWorker [-] required exact-form",
	},
	"WorkerCustomDomain": {
		"/worker -> ModuleWorker [-] required iface:worker.runtime@1.0.0",
	},
	"WorkerEndpoint": {
		"/worker -> ModuleWorker [-] required iface:worker.runtime@1.0.0",
	},
	"WorkerCronTrigger": {
		"/worker -> ModuleWorker [-] required iface:worker.runtime@1.0.0",
	},
	"EdgeKVNamespace":  nil,
	"ObjectBucket":     nil,
	"SQLiteDatabase":   nil,
	"AtLeastOnceQueue": nil,
	"QueueConsumer": {
		"/deadLetterQueue -> AtLeastOnceQueue [-] optional iface:edge.queue@1.0.0",
		"/queue -> AtLeastOnceQueue [-] required iface:edge.queue@1.0.0",
		"/worker -> ModuleWorker [-] required iface:worker.runtime@1.0.0",
	},
}

func renderRelation(relation model.Relation) string {
	binding := relation.Binding
	if binding == "" {
		binding = "-"
	}
	requiredness := "optional"
	if relation.Required {
		requiredness = "required"
	}
	contract := "none"
	switch {
	case len(relation.TargetFormRefs) > 0:
		contract = "exact-form"
	case relation.RequiredInterface != nil:
		contract = "iface:" + relation.RequiredInterface.Name + "@" + relation.RequiredInterface.Version
	}
	return relation.Pointer + " -> " + relation.TargetKind + " [" + binding + "] " + requiredness + " " + contract
}

// TestDerivedRelationsMatchTheCatalog proves the derivation over every Form:
// the pointer set, the target kinds, the binding contracts, and requiredness
// are exactly the table above, and every target group is the family group.
func TestDerivedRelationsMatchTheCatalog(t *testing.T) {
	t.Parallel()
	if len(Forms) != len(wantRelations) {
		t.Fatalf("catalog has %d Forms, the relation table lists %d", len(Forms), len(wantRelations))
	}
	for _, form := range Forms {
		want, listed := wantRelations[form.Kind]
		if !listed {
			t.Fatalf("Form %s has no relation-table entry", form.Kind)
		}
		relations, err := model.DeriveRelations(desiredSchemaFor(t, form))
		if err != nil {
			t.Fatalf("%s: %v", form.Kind, err)
		}
		got := make([]string, 0, len(relations))
		for _, relation := range relations {
			if relation.TargetAPIVersion != Family.APIVersion() {
				t.Errorf("%s relation %s targets group %q", form.Kind, relation.Pointer, relation.TargetAPIVersion)
			}
			got = append(got, renderRelation(relation))
		}
		if len(got) == 0 {
			got = nil
		}
		sorted := append([]string(nil), want...)
		sort.Strings(sorted)
		if !reflect.DeepEqual(got, sorted) {
			t.Errorf("%s relations =\n%v\nwant\n%v", form.Kind, got, sorted)
		}
	}
}

// TestEveryDeclaredReferenceIsDerived is the regression guard for the defect
// this work fixed: the old host recognized a reference only inside a member
// literally named "resource", so only bindings were ever seen. Counting the
// declared reference fields against the derived relations proves nothing is
// invisible any more.
func TestEveryDeclaredReferenceIsDerived(t *testing.T) {
	t.Parallel()
	totalDeclared, totalDerived, bindings := 0, 0, 0
	for _, form := range Forms {
		relations, err := model.DeriveRelations(desiredSchemaFor(t, form))
		if err != nil {
			t.Fatalf("%s: %v", form.Kind, err)
		}
		totalDeclared += declaredReferenceCount(form.Fields)
		totalDerived += len(relations)
		for _, relation := range relations {
			if relation.IsBinding() {
				bindings++
			}
		}
	}
	if totalDeclared != totalDerived {
		t.Fatalf("catalog declares %d reference fields but derives %d relations", totalDeclared, totalDerived)
	}
	if totalDerived != 15 {
		t.Fatalf("family derives %d relations, want the 15 in the relation table", totalDerived)
	}
	// Five of the fourteen are typed bindings. Every other relation was
	// completely unvalidated before this lane learned to derive them.
	if bindings != 5 {
		t.Fatalf("family derives %d binding relations, want 5", bindings)
	}
}

// TestBindingContractsHoldAcrossTheCatalog proves the authoring side of the
// rules a host verifies at apply time: it must fail when a target Form stops
// providing the Interface a binding requires.
func TestBindingContractsHoldAcrossTheCatalog(t *testing.T) {
	if err := validateBindingContracts(); err != nil {
		t.Fatalf("catalog binding contracts: %v", err)
	}
}
