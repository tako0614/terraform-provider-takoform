package edgeformcatalog

import (
	"slices"
	"strings"
	"testing"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

var orderedKinds = []string{
	"ModuleWorker",
	"WorkerBundle",
	"WorkerVersion",
	"WorkerDeployment",
	"WorkerCustomDomain",
	"WorkerCronTrigger",
	"EdgeKVNamespace",
	"ObjectBucket",
	"SQLiteDatabase",
	"AtLeastOnceQueue",
	"QueueConsumer",
}

func TestCatalogValidates(t *testing.T) {
	t.Parallel()
	if err := Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogIsExactElevenFormFamily(t *testing.T) {
	t.Parallel()
	if len(Forms) != len(orderedKinds) {
		t.Fatalf("family has %d forms, want %d", len(Forms), len(orderedKinds))
	}
	for index, form := range Forms {
		if form.Kind != orderedKinds[index] || form.DefinitionVersion != "0.1.0" {
			t.Fatalf("form[%d] = %s@%s", index, form.Kind, form.DefinitionVersion)
		}
	}
	if Family.APIVersion() != "edge.forms.takoform.com/v1alpha1" {
		t.Fatalf("family apiVersion = %q", Family.APIVersion())
	}
}

func TestCatalogHasReviewedSemanticFields(t *testing.T) {
	t.Parallel()
	want := map[string][]string{
		"ModuleWorker": {},
		"WorkerBundle": {"mainModule", "modules"},
		"WorkerVersion": {
			"bucketBindings", "bundle", "compatibilityDate", "compatibilityFlags",
			"handlers", "kvBindings", "queueProducerBindings", "requiredSensitiveVars",
			"serviceBindings", "sqliteBindings", "vars", "worker",
		},
		"WorkerDeployment":   {"versions", "worker"},
		"WorkerCustomDomain": {"hostname", "worker"},
		"WorkerCronTrigger":  {"cron", "worker"},
		"EdgeKVNamespace":    {},
		"ObjectBucket":       {},
		"SQLiteDatabase":     {},
		"AtLeastOnceQueue":   {"deliveryDelaySeconds", "messageRetentionSeconds"},
		"QueueConsumer": {
			"deadLetterQueue", "maxBatchSize", "maxBatchTimeoutSeconds", "maxConcurrency",
			"maxRetries", "queue", "retryDelaySeconds", "worker",
		},
	}
	for _, form := range Forms {
		got := make([]string, 0, len(form.Fields))
		for _, field := range form.Fields {
			got = append(got, field.Wire)
		}
		slices.Sort(got)
		if !slices.Equal(got, want[form.Kind]) {
			t.Errorf("%s fields = %v, want %v", form.Kind, got, want[form.Kind])
		}
	}
}

func TestRoleRules(t *testing.T) {
	t.Parallel()
	wantRoles := map[string]model.Role{
		"ModuleWorker":       model.RoleIdentity,
		"WorkerBundle":       model.RoleRevision,
		"WorkerVersion":      model.RoleRevision,
		"WorkerDeployment":   model.RoleDeployment,
		"WorkerCustomDomain": model.RoleAttachment,
		"WorkerCronTrigger":  model.RoleAttachment,
		"EdgeKVNamespace":    model.RoleIdentity,
		"ObjectBucket":       model.RoleIdentity,
		"SQLiteDatabase":     model.RoleIdentity,
		"AtLeastOnceQueue":   model.RoleIdentity,
		"QueueConsumer":      model.RoleAttachment,
	}
	for _, form := range Forms {
		if form.Role != wantRoles[form.Kind] {
			t.Errorf("%s role = %s, want %s", form.Kind, form.Role, wantRoles[form.Kind])
		}
		capabilities := form.LifecycleCapabilities()
		if form.Role == model.RoleRevision {
			if slices.Contains(capabilities, "update") || slices.Contains(capabilities, "refresh") {
				t.Errorf("%s is a revision but declares update/refresh: %v", form.Kind, capabilities)
			}
		}
		for _, field := range form.Fields {
			if field.Kind == model.KindBindingList && form.Role != model.RoleRevision {
				t.Errorf("%s carries binding list %s outside the revision role", form.Kind, field.Wire)
			}
		}
		if len(form.AcceptedBindings) > 0 && form.Role != model.RoleRevision {
			t.Errorf("%s accepts bindings outside the revision role", form.Kind)
		}
	}
}

func TestOnlyWorkerVersionAcceptsBindings(t *testing.T) {
	t.Parallel()
	for _, form := range Forms {
		if form.Kind == "WorkerVersion" {
			if len(form.AcceptedBindings) != 5 {
				t.Errorf("WorkerVersion accepts %d bindings, want 5", len(form.AcceptedBindings))
			}
			continue
		}
		if len(form.AcceptedBindings) != 0 {
			t.Errorf("%s unexpectedly accepts bindings", form.Kind)
		}
	}
}

func TestProvidedInterfaceAssignments(t *testing.T) {
	t.Parallel()
	want := map[string]string{
		"EdgeKVNamespace":  "edge.kv",
		"ObjectBucket":     "edge.objects",
		"SQLiteDatabase":   "edge.sql",
		"AtLeastOnceQueue": "edge.queue",
		"WorkerVersion":    "worker.service",
	}
	for _, form := range Forms {
		wantInterface, expects := want[form.Kind]
		if !expects {
			if len(form.ProvidedInterfaces) != 0 {
				t.Errorf("%s unexpectedly provides interfaces", form.Kind)
			}
			continue
		}
		if len(form.ProvidedInterfaces) != 1 || form.ProvidedInterfaces[0].Name != wantInterface {
			t.Errorf("%s provides %v, want %s", form.Kind, form.ProvidedInterfaces, wantInterface)
		}
	}
}

func TestNoVendorNamesInRenderedOutputs(t *testing.T) {
	t.Parallel()
	var rendered []string
	forms, err := RenderForms()
	if err != nil {
		t.Fatal(err)
	}
	for _, form := range forms {
		rendered = append(rendered, form.DefinitionJSON)
	}
	interfaces, err := RenderInterfaces()
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := RenderBindings()
	if err != nil {
		t.Fatal(err)
	}
	for _, contract := range append(interfaces, bindings...) {
		rendered = append(rendered, contract.DefinitionJSON)
	}
	for _, text := range rendered {
		lowered := strings.ToLower(text)
		for _, token := range []string{"cloudflare", "wrangler", "workers.dev"} {
			if strings.Contains(lowered, token) {
				t.Fatalf("rendered output names a vendor token %q", token)
			}
		}
	}
}
