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
		// A bundle's portable desired state is exactly the immutable identity of
		// its committed artifact manifest: the manifest, not the Form, describes
		// the modules (spec/artifact-transport, decision 0014).
		"WorkerBundle": {"manifestDigest"},
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
			if slices.Contains(capabilities, "update") {
				t.Errorf("%s is a revision but declares update: %v", form.Kind, capabilities)
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

// TestLifecycleCapabilityTable pins the exact capability set of every family
// member. update is a claim about what an in-place apply can move, so a Form
// with nothing mutable must not advertise it, and no Form of any role
// advertises refresh in the v1alpha3 lane.
func TestLifecycleCapabilityTable(t *testing.T) {
	t.Parallel()
	base := []string{"create", "read", "delete", "import", "observe"}
	withUpdate := []string{"create", "read", "update", "delete", "import", "observe"}
	want := map[string][]string{
		"ModuleWorker":       base,
		"WorkerBundle":       base,
		"WorkerVersion":      base,
		"WorkerDeployment":   withUpdate,
		"WorkerCustomDomain": base,
		"WorkerCronTrigger":  withUpdate,
		"EdgeKVNamespace":    base,
		"ObjectBucket":       base,
		"SQLiteDatabase":     base,
		"AtLeastOnceQueue":   withUpdate,
		"QueueConsumer":      withUpdate,
	}
	if len(want) != len(Forms) {
		t.Fatalf("capability table covers %d forms, the family has %d", len(want), len(Forms))
	}
	for _, form := range Forms {
		got := form.LifecycleCapabilities()
		if !slices.Equal(got, want[form.Kind]) {
			t.Errorf("%s capabilities = %v, want %v", form.Kind, got, want[form.Kind])
		}
		if slices.Contains(got, "refresh") {
			t.Errorf("%s declares refresh; the v1alpha3 lane has no refresh capability", form.Kind)
		}
		if form.Role == model.RoleRevision && slices.Contains(got, "update") {
			t.Errorf("%s is a revision but declares update", form.Kind)
		}
	}
}

// TestEveryOptionalFieldCarriesPortableMeaning is the family-wide statement of
// the boundary rule, and pins the single reviewed absence-is-semantics
// exemption so a second one cannot appear unnoticed.
func TestEveryOptionalFieldCarriesPortableMeaning(t *testing.T) {
	t.Parallel()
	var exempt []string
	for _, form := range Forms {
		for _, field := range form.Fields {
			if field.Required {
				continue
			}
			switch {
			case field.AbsenceIsSemantic:
				exempt = append(exempt, form.Kind+"/"+field.Wire)
			case field.Default == nil:
				t.Errorf("%s optional field %s has no portable meaning when omitted", form.Kind, field.Wire)
			}
		}
	}
	if !slices.Equal(exempt, []string{"QueueConsumer/deadLetterQueue"}) {
		t.Errorf("absence-is-semantics fields = %v, want exactly the reviewed QueueConsumer/deadLetterQueue", exempt)
	}
}

// TestDeclaredDefaultsReachTheDesiredSchema proves the default travels where a
// host can see it: inside the Form Definition's own desiredSchema.
func TestDeclaredDefaultsReachTheDesiredSchema(t *testing.T) {
	t.Parallel()
	for _, form := range Forms {
		properties, _ := form.DesiredSchema()["properties"].(map[string]any)
		for _, field := range form.Fields {
			property, _ := properties[field.Wire].(map[string]any)
			if property == nil {
				t.Fatalf("%s desired schema has no property %s", form.Kind, field.Wire)
			}
			_, declared := property["default"]
			if declared != (field.Default != nil) {
				t.Errorf("%s property %s declares default=%v, want %v", form.Kind, field.Wire, declared, field.Default != nil)
			}
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
