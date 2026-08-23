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
	"StaticAssetBundle",
	"WorkerVersion",
	"WorkerDeployment",
	"WorkerCustomDomain",
	"WorkerEndpoint",
	"WorkerCronTrigger",
	"EdgeKVNamespace",
	"ObjectBucket",
	"SQLiteDatabase",
	"SQLiteMigrationSet",
	"SQLiteMigrationApplication",
	"AtLeastOnceQueue",
	"QueueConsumer",
	"DurableWorkflow",
	"ActorNamespace",
}

func TestCatalogValidates(t *testing.T) {
	t.Parallel()
	if err := Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogIsExactSeventeenFormFamily(t *testing.T) {
	t.Parallel()
	if len(Forms) != len(orderedKinds) {
		t.Fatalf("family has %d forms, want %d", len(Forms), len(orderedKinds))
	}
	// Worker Version alone is off the generation line: its contract gained
	// the sealed externalServices slot list in Beta 2, so it is not the
	// v1beta1 contract re-identified (decision 0046). Spelling the exception
	// out here means a second Form drifting off the line fails this test.
	wantVersions := map[string]string{"WorkerVersion": "0.2.0"}
	for index, form := range Forms {
		want := wantVersions[form.Kind]
		if want == "" {
			want = "0.1.0"
		}
		if form.Kind != orderedKinds[index] || form.DefinitionVersion != want {
			t.Fatalf("form[%d] = %s@%s, want %s@%s", index, form.Kind, form.DefinitionVersion, orderedKinds[index], want)
		}
	}
	if Family.APIVersion() != "edge.forms.takoform.com/v1beta2" {
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
		"WorkerBundle":      {"manifestDigest"},
		"StaticAssetBundle": {"manifestDigest"},
		// No compatibilityDate and no compatibilityFlags: the runtime is fixed by
		// the exact worker.runtime contract the Module Worker identity provides,
		// not by a token this project has no behavior registry to interpret
		// (decision 0019).
		"WorkerVersion": {
			"actorBindings", "assets", "bucketBindings", "bundle", "externalServices", "handlers", "kvBindings",
			"queueProducerBindings", "requiredSensitiveVars", "serviceBindings", "sqliteBindings", "vars",
			"worker", "workflowBindings",
		},
		"WorkerDeployment":   {"versions", "worker"},
		"WorkerCustomDomain": {"hostname", "worker"},
		// Reachability is the whole request; the address is the answer, so the
		// worker reference is the ONLY desired member (decision 0024).
		"WorkerEndpoint":             {"worker"},
		"WorkerCronTrigger":          {"cron", "worker"},
		"EdgeKVNamespace":            {},
		"ObjectBucket":               {},
		"SQLiteDatabase":             {},
		"SQLiteMigrationSet":         {"manifestDigest"},
		"SQLiteMigrationApplication": {"database", "migrationSet"},
		"AtLeastOnceQueue":           {"deliveryDelaySeconds", "messageRetentionSeconds"},
		"QueueConsumer": {
			"deadLetterQueue", "maxBatchSize", "maxBatchTimeoutSeconds", "maxConcurrency",
			"maxRetries", "queue", "retryDelaySeconds", "worker",
		},
		// Both carry only the worker and the class. Everything else about a
		// workflow or an actor — the execution model, the storage, the alarm —
		// is the exact Interface the identity provides, and what runs is
		// whatever the worker's deployment selects.
		"DurableWorkflow": {"className", "worker"},
		"ActorNamespace":  {"className", "worker"},
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
		"ModuleWorker":               model.RoleIdentity,
		"WorkerBundle":               model.RoleRevision,
		"StaticAssetBundle":          model.RoleRevision,
		"WorkerVersion":              model.RoleRevision,
		"WorkerDeployment":           model.RoleDeployment,
		"WorkerCustomDomain":         model.RoleAttachment,
		"WorkerEndpoint":             model.RoleAttachment,
		"WorkerCronTrigger":          model.RoleAttachment,
		"EdgeKVNamespace":            model.RoleIdentity,
		"ObjectBucket":               model.RoleIdentity,
		"SQLiteDatabase":             model.RoleIdentity,
		"SQLiteMigrationSet":         model.RoleRevision,
		"SQLiteMigrationApplication": model.RoleAttachment,
		"AtLeastOnceQueue":           model.RoleIdentity,
		"QueueConsumer":              model.RoleAttachment,
		"DurableWorkflow":            model.RoleIdentity,
		"ActorNamespace":             model.RoleIdentity,
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
// advertises refresh in the v1beta1 lane.
func TestLifecycleCapabilityTable(t *testing.T) {
	t.Parallel()
	base := []string{"create", "read", "delete", "import", "observe"}
	withUpdate := []string{"create", "read", "update", "delete", "import", "observe"}
	want := map[string][]string{
		"ModuleWorker":               base,
		"WorkerBundle":               base,
		"StaticAssetBundle":          base,
		"WorkerVersion":              base,
		"WorkerDeployment":           withUpdate,
		"WorkerCustomDomain":         base,
		"WorkerEndpoint":             base,
		"WorkerCronTrigger":          withUpdate,
		"EdgeKVNamespace":            base,
		"ObjectBucket":               base,
		"SQLiteDatabase":             base,
		"SQLiteMigrationSet":         base,
		"SQLiteMigrationApplication": base,
		"AtLeastOnceQueue":           withUpdate,
		"QueueConsumer":              withUpdate,
		// Both members' desired fields are immutable, so neither can advertise
		// an in-place update: a change of worker or class is a replacement.
		"DurableWorkflow": base,
		"ActorNamespace":  base,
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
			t.Errorf("%s declares refresh; the v1beta1 lane has no refresh capability", form.Kind)
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
	if !slices.Equal(exempt, []string{"WorkerVersion/assets", "QueueConsumer/deadLetterQueue"}) {
		t.Errorf("absence-is-semantics fields = %v, want exactly the reviewed WorkerVersion/assets and QueueConsumer/deadLetterQueue", exempt)
	}
}

func TestEdgeAppBetaDesiredStateIsClosedAndArtifactBacked(t *testing.T) {
	t.Parallel()

	assets, ok := ByKind("StaticAssetBundle")
	if !ok {
		t.Fatal("StaticAssetBundle is not declared")
	}
	migrations, ok := ByKind("SQLiteMigrationSet")
	if !ok {
		t.Fatal("SQLiteMigrationSet is not declared")
	}
	for _, form := range []model.Form{assets, migrations} {
		if form.Role != model.RoleRevision {
			t.Errorf("%s role = %s, want revision", form.Kind, form.Role)
		}
		if len(form.Fields) != 1 || form.Fields[0].Wire != "manifestDigest" ||
			form.Fields[0].Pattern != model.PatternCanonicalSHA256 {
			t.Errorf("%s desired fields = %#v, want only canonical manifestDigest", form.Kind, form.Fields)
		}
	}

	version, ok := ByKind("WorkerVersion")
	if !ok {
		t.Fatal("WorkerVersion is not declared")
	}
	var assetField *model.Field
	for index := range version.Fields {
		if version.Fields[index].Wire == "assets" {
			assetField = &version.Fields[index]
			break
		}
	}
	if assetField == nil {
		t.Fatal("WorkerVersion has no assets field")
	}
	if assetField.Kind != model.KindObject || !assetField.AbsenceIsSemantic {
		t.Fatalf("WorkerVersion assets = %#v, want optional semantic object", *assetField)
	}
	wantMembers := map[string]model.FieldKind{
		"bundle":           model.KindResourceRef,
		"runWorkerFirst":   model.KindBoolean,
		"notFoundHandling": model.KindStringEnum,
	}
	for _, member := range assetField.Fields {
		if !member.Required {
			t.Errorf("assets.%s is optional; every present assets object must be complete", member.Wire)
		}
		if wantMembers[member.Wire] != member.Kind {
			t.Errorf("assets.%s kind = %s, want %s", member.Wire, member.Kind, wantMembers[member.Wire])
		}
		delete(wantMembers, member.Wire)
		if member.Wire == "notFoundHandling" && !slices.Equal(member.Enum, []string{"none", "single_page_application"}) {
			t.Errorf("assets.notFoundHandling enum = %v", member.Enum)
		}
	}
	if len(wantMembers) != 0 {
		t.Errorf("WorkerVersion assets is missing members %v", wantMembers)
	}

	application, ok := ByKind("SQLiteMigrationApplication")
	if !ok {
		t.Fatal("SQLiteMigrationApplication is not declared")
	}
	if application.Role != model.RoleAttachment || application.DeclaresUpdate() {
		t.Errorf("SQLiteMigrationApplication role/capability = %s/%v", application.Role, application.LifecycleCapabilities())
	}
	for _, field := range application.Fields {
		if field.Kind != model.KindResourceRef || !field.Required || !field.Immutable || !field.Target.ExactForm {
			t.Errorf("SQLiteMigrationApplication.%s is not an immutable exact resource relation: %#v", field.Wire, field)
		}
	}
}

// TestDeclaredDefaultsReachTheDesiredSchema proves the default travels where a
// host can see it: inside the Form Definition's own desiredSchema.
func TestDeclaredDefaultsReachTheDesiredSchema(t *testing.T) {
	t.Parallel()
	for _, form := range Forms {
		properties, _ := desiredSchemaFor(t, form)["properties"].(map[string]any)
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
			if len(form.AcceptedBindings) != 7 {
				t.Errorf("WorkerVersion accepts %d bindings, want 7", len(form.AcceptedBindings))
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
	want := map[string][]string{
		"EdgeKVNamespace":  {"edge.kv"},
		"ObjectBucket":     {"edge.objects"},
		"SQLiteDatabase":   {"edge.sql"},
		"AtLeastOnceQueue": {"edge.queue"},
		// The worker identity carries both directions of its exact contracts.
		// worker.runtime is the ES Module Worker ABI the Form claims to fix, so
		// a host that supports ModuleWorker implements it at that exact digest
		// (decision 0019). worker.service belongs to the IDENTITY too: the
		// module-worker.service binding lists ModuleWorker in its
		// allowedTargetForms, and a host verifies that the resolved target's
		// Form provides the binding's targetInterface.
		"ModuleWorker": {"worker.runtime", "worker.service"},
		// Each holds its contract on the IDENTITY for the same reason
		// ModuleWorker holds worker.runtime: the identity is what a host
		// implements, and the class a Worker Version exports is the code that
		// fills it.
		"DurableWorkflow": {"worker.workflow"},
		"ActorNamespace":  {"worker.actor"},
	}
	for _, form := range Forms {
		wantInterfaces, expects := want[form.Kind]
		if !expects {
			if len(form.ProvidedInterfaces) != 0 {
				t.Errorf("%s unexpectedly provides interfaces", form.Kind)
			}
			continue
		}
		got := make([]string, 0, len(form.ProvidedInterfaces))
		for _, provided := range form.ProvidedInterfaces {
			got = append(got, provided.Name)
		}
		if !slices.Equal(got, wantInterfaces) {
			t.Errorf("%s provides %v, want %v", form.Kind, got, wantInterfaces)
		}
	}
}

// TestModuleWorkerABIIsAnExactContract proves the ABI is stated as data rather
// than claimed in prose: the identity provides the runtime contract, the
// contract declares each handler it names as a real operation, and the
// WorkerVersion `handlers` enum is that vocabulary and nothing else. It is the
// authoring-time half of decision 0019; the host-side half is the required
// conformance check.
func TestModuleWorkerABIIsAnExactContract(t *testing.T) {
	t.Parallel()
	handlers, err := RuntimeHandlers()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(handlers, []string{"fetch", "scheduled", "queue"}) {
		t.Fatalf("runtime handler vocabulary = %v", handlers)
	}
	definition, err := interfaceDefinitionByName(WorkerRuntimeInterfaceName)
	if err != nil {
		t.Fatal(err)
	}
	operations := make([]string, 0, len(definition.Operations))
	for _, operation := range definition.Operations {
		operations = append(operations, operation.Name)
	}
	slices.Sort(operations)
	want := []string{
		"environment", "fetch", "globals", "loadModule",
		"queue", "scheduled", "waitUntil",
	}
	if !slices.Equal(operations, want) {
		t.Fatalf("worker.runtime operations = %v, want %v", operations, want)
	}
	if definition.Semantics.Consistency != "read_after_write" ||
		definition.Semantics.Delivery != "at_least_once" ||
		definition.Semantics.Ordering != "none" {
		t.Fatalf("worker.runtime semantics = %+v", definition.Semantics)
	}
	if len(definition.Fixtures) == 0 {
		t.Fatal("the runtime ABI must prove what is provable with fixtures")
	}
	version, known := ByKind("WorkerVersion")
	if !known {
		t.Fatal("WorkerVersion is missing from the catalog")
	}
	for _, field := range version.Fields {
		if field.Wire != "handlers" {
			continue
		}
		if !slices.Equal(field.Enum, handlers) {
			t.Fatalf("WorkerVersion handlers enum = %v, want the ABI vocabulary %v", field.Enum, handlers)
		}
		return
	}
	t.Fatal("WorkerVersion declares no handlers field")
}

// TestNoRuntimeSelectorTokensRemain proves the removal is complete in the bytes
// that ship: no Form, Interface, or Binding Definition of the family names a
// compatibility date or flag. A date is only meaningful against a registry that
// says which behavior each date changes, and this project has none, so the
// field promised portability it could not deliver (decision 0019).
func TestNoRuntimeSelectorTokensRemain(t *testing.T) {
	t.Parallel()
	forms, err := RenderForms()
	if err != nil {
		t.Fatal(err)
	}
	rendered := make([]string, 0, len(forms)+16)
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
		for _, token := range []string{"compatibilitydate", "compatibility_date", "compatibilityflags", "compatibility_flags", "nodejs_compat"} {
			if strings.Contains(lowered, token) {
				t.Fatalf("rendered output still carries the runtime selector token %q", token)
			}
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

// TestAnAssignedHostnameIsCanonicalAndAgreesWithItsURL holds the two published
// members of an assigned address to one grammar.
//
// The desired-state hostname grammar deliberately admits the spellings DNS
// treats as one name, because a host canonicalizes what an author wrote. An
// assigned value has no earlier spelling to preserve, so reusing that grammar
// for an output would let a host publish "a.example." — a name its own
// canonical form forbids — while the url member, which carries no optional
// dot, could not be built from it. A contract whose two members cannot both be
// satisfied by one address is not a contract a host can conform to.
func TestAnAssignedHostnameIsCanonicalAndAgreesWithItsURL(t *testing.T) {
	for _, form := range Forms {
		hostname, hasHostname := outputPattern(form, "hostname")
		if !hasHostname {
			continue
		}
		if hostname != model.PatternCanonicalHostname {
			t.Errorf("%s publishes an assigned hostname on the authored grammar %q; an assigned name is canonical", form.Kind, hostname)
		}
		url, hasURL := outputPattern(form, "url")
		if !hasURL {
			continue
		}
		// The url is exactly https:// + the hostname + /, so its pattern is the
		// hostname's with the anchors moved outward. Comparing the strings says
		// that in the one place a divergence could hide.
		want := `^https://` + strings.TrimSuffix(strings.TrimPrefix(hostname, `^`), `$`) + `/$`
		if url != want {
			t.Errorf("%s url pattern %q does not admit exactly the hostnames its hostname pattern does; want %q", form.Kind, url, want)
		}
	}
}

func outputPattern(form model.Form, wire string) (string, bool) {
	for _, output := range form.Outputs {
		if output.Wire == wire {
			return output.Pattern, output.Pattern != ""
		}
	}
	return "", false
}
