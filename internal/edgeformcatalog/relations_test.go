package edgeformcatalog

import (
	"reflect"
	"sort"
	"testing"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// wantRelations is the complete cross-resource relation table of the Edge
// Platform Family, written out by hand. Relations are DERIVED from each Form's
// desired schema, so this table is the independent statement of what the
// catalog declares: a reference added, removed, or retargeted without a
// deliberate edit here fails the family.
//
// Every entry is "<pointer> -> <target kind>" with "*" standing for an array
// element, plus the Binding contract when the reference is a typed binding and
// "-" when it is a plain cross-resource reference.
var wantRelations = map[string][]string{
	"ModuleWorker": nil,
	"WorkerBundle": nil,
	"WorkerVersion": {
		"/bucketBindings/*/resource -> ObjectBucket [module-worker.object-bucket] optional",
		"/bundle -> WorkerBundle [-] required",
		"/kvBindings/*/resource -> EdgeKVNamespace [module-worker.edge-kv] optional",
		"/queueProducerBindings/*/resource -> AtLeastOnceQueue [module-worker.queue-producer] optional",
		"/serviceBindings/*/resource -> ModuleWorker [module-worker.service] optional",
		"/sqliteBindings/*/resource -> SQLiteDatabase [module-worker.sqlite] optional",
		"/worker -> ModuleWorker [-] required",
	},
	"WorkerDeployment": {
		"/versions/*/workerVersion -> WorkerVersion [-] required",
		"/worker -> ModuleWorker [-] required",
	},
	"WorkerCustomDomain": {
		"/worker -> ModuleWorker [-] required",
	},
	"WorkerCronTrigger": {
		"/worker -> ModuleWorker [-] required",
	},
	"EdgeKVNamespace":  nil,
	"ObjectBucket":     nil,
	"SQLiteDatabase":   nil,
	"AtLeastOnceQueue": nil,
	"QueueConsumer": {
		"/deadLetterQueue -> AtLeastOnceQueue [-] optional",
		"/queue -> AtLeastOnceQueue [-] required",
		"/worker -> ModuleWorker [-] required",
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
	return relation.Pointer + " -> " + relation.TargetKind + " [" + binding + "] " + requiredness
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
		relations, err := model.DeriveRelations(form.DesiredSchema())
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
		relations, err := model.DeriveRelations(form.DesiredSchema())
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
	if totalDerived != 14 {
		t.Fatalf("family derives %d relations, want the 14 in the relation table", totalDerived)
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
