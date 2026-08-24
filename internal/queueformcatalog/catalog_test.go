package queueformcatalog

import (
	"slices"
	"strings"
	"testing"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

func TestCatalogValidates(t *testing.T) {
	if err := Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogIdentityAndPullQueueFields(t *testing.T) {
	if Family.APIVersion() != "queue.forms.takoform.com" {
		t.Fatalf("family apiVersion = %q, want versionless queue family", Family.APIVersion())
	}
	if len(Forms) != 1 || Forms[0].Kind != "PullQueue" || Forms[0].Role != model.RoleIdentity {
		t.Fatalf("forms = %#v, want one PullQueue identity", Forms)
	}
	form := Forms[0]
	if form.DefinitionVersion != "0.1.0" {
		t.Fatalf("definition version = %q, want 0.1.0", form.DefinitionVersion)
	}
	if form.RequiresHostAPI != currentHostAPI {
		t.Fatalf("requiresHostApi = %q, want %q for acyclic relation support", form.RequiresHostAPI, currentHostAPI)
	}
	var names []string
	for _, field := range form.Fields {
		names = append(names, field.Wire)
	}
	wantNames := []string{"messageRetentionSeconds", "defaultVisibilityTimeoutSeconds", "receiveWaitBoundSeconds", "deadLetter"}
	if !slices.Equal(names, wantNames) {
		t.Fatalf("fields = %v, want %v", names, wantNames)
	}
	wantBounds := map[string][2]int64{
		"messageRetentionSeconds":         {60, 1209600},
		"defaultVisibilityTimeoutSeconds": {0, 43200},
		"receiveWaitBoundSeconds":         {0, 20},
	}
	for _, field := range form.Fields[:3] {
		if field.Min == nil || field.Max == nil || *field.Min != wantBounds[field.Wire][0] || *field.Max != wantBounds[field.Wire][1] || !field.Required {
			t.Fatalf("%s bounds/required = %#v, want %v..%v required", field.Wire, field, wantBounds[field.Wire][0], wantBounds[field.Wire][1])
		}
	}
	deadLetter := form.Fields[3]
	if deadLetter.Required || !deadLetter.AbsenceIsSemantic || len(deadLetter.Fields) != 2 {
		t.Fatalf("deadLetter = %#v, want optional semantic object with two members", deadLetter)
	}
	queue := deadLetter.Fields[0]
	if queue.Wire != "queue" || queue.ResourceTarget == nil {
		t.Fatalf("deadLetter.queue = %#v, want explicit ResourceTarget", queue)
	}
	target := *queue.ResourceTarget
	if target.Group != Family.APIVersion() || target.Kind != "PullQueue" {
		t.Fatalf("deadLetter.queue target = %#v, want queue.forms.takoform.com/PullQueue", target)
	}
	if target.Contract.Interface == nil || target.Contract.Interface.Name != QueuePullInterfaceName || target.Contract.Interface.Version != "1.0.0" {
		t.Fatalf("deadLetter.queue contract = %#v, want queue.pull@1.0.0", target.Contract)
	}
	if queue.TargetKind != "" || queue.Target.Declared() {
		t.Fatalf("deadLetter.queue leaks retained/provider target spelling: %#v", queue)
	}
	maxReceiveCount := deadLetter.Fields[1]
	if maxReceiveCount.Wire != "maxReceiveCount" || maxReceiveCount.Min == nil || maxReceiveCount.Max == nil || *maxReceiveCount.Min != 1 || *maxReceiveCount.Max != 1000 {
		t.Fatalf("maxReceiveCount = %#v, want 1..1000", maxReceiveCount)
	}
	constraints := form.Constraints()
	if len(constraints) != 1 || constraints[0].Kind != model.ConstraintAcyclic || constraints[0].Reference != "/deadLetter/queue" {
		t.Fatalf("constraints = %#v, want acyclic /deadLetter/queue", constraints)
	}
}

func TestRenderedFormCarriesInterfaceAndAcyclicConstraint(t *testing.T) {
	rendered, err := RenderForms()
	if err != nil {
		t.Fatal(err)
	}
	if len(rendered) != 1 {
		t.Fatalf("rendered = %d, want one Form", len(rendered))
	}
	item := rendered[0]
	if item.Definition.APIVersion != Family.APIVersion() || item.Definition.Kind != "PullQueue" || item.Definition.DefinitionVersion != "0.1.0" {
		t.Fatalf("rendered identity = %s/%s@%s", item.Definition.APIVersion, item.Definition.Kind, item.Definition.DefinitionVersion)
	}
	if _, ok := item.Fixtures["desired.json"]; !ok {
		t.Fatal("rendered Form has no canonical desired fixture")
	}
	if len(item.Definition.ProvidedInterfaces) != 1 {
		t.Fatalf("providedInterfaces = %#v, want queue.pull", item.Definition.ProvidedInterfaces)
	}
	provided := item.Definition.ProvidedInterfaces[0]
	if provided.APIVersion != InterfaceAPIVersion || provided.Name != QueuePullInterfaceName || provided.Version != "1.0.0" || !strings.HasPrefix(provided.SchemaDigest, "sha256:") {
		t.Fatalf("provided interface = %#v", provided)
	}
	if len(item.Definition.Constraints) != 1 || item.Definition.Constraints[0].Kind != string(model.ConstraintAcyclic) || item.Definition.Constraints[0].Reference != "/deadLetter/queue" {
		t.Fatalf("rendered constraints = %#v", item.Definition.Constraints)
	}
	properties := item.Definition.DesiredSchema["properties"].(map[string]any)
	deadLetter := properties["deadLetter"].(map[string]any)
	deadLetterProperties := deadLetter["properties"].(map[string]any)
	queue := deadLetterProperties["queue"].(map[string]any)
	queueProperties := queue["properties"].(map[string]any)
	if got := queueProperties["apiVersion"].(map[string]any)["const"]; got != Family.APIVersion() {
		t.Fatalf("queue apiVersion const = %v", got)
	}
	if got := queueProperties["kind"].(map[string]any)["const"]; got != "PullQueue" {
		t.Fatalf("queue kind const = %v", got)
	}
	required := queue[model.RequiredInterfaceAnnotationKey].(map[string]any)
	if required["name"] != QueuePullInterfaceName || required["version"] != "1.0.0" || required["apiVersion"] != InterfaceAPIVersion {
		t.Fatalf("queue required interface = %#v", required)
	}
	if _, exact := queue[model.TargetFormRefsAnnotationKey]; exact {
		t.Fatal("self dead-letter queue must not embed an impossible self exact FormRef")
	}
}

func TestQueueInterfaceAndExactResolver(t *testing.T) {
	definitions := InterfaceDefinitions()
	if err := ValidateInterfaceDefinitions(definitions); err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 1 || definitions[0].Name != QueuePullInterfaceName || definitions[0].Version != "1.0.0" {
		t.Fatalf("interface definitions = %#v", definitions)
	}
	wantOperations := []string{"send", "receive", "delete", "changeVisibility"}
	var operations []string
	for _, operation := range definitions[0].Operations {
		operations = append(operations, operation.Name)
	}
	if !slices.Equal(operations, wantOperations) {
		t.Fatalf("queue.pull operations = %v, want %v", operations, wantOperations)
	}
	rendered, err := RenderInterfaces()
	if err != nil {
		t.Fatal(err)
	}
	if len(rendered) != 1 || !strings.HasPrefix(rendered[0].SchemaDigest, "sha256:") {
		t.Fatalf("rendered interfaces = %#v", rendered)
	}
	iface, err := InterfaceRefFor(QueuePullInterfaceName, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if iface.SchemaDigest != rendered[0].SchemaDigest {
		t.Fatalf("interface digest = %s, rendered = %s", iface.SchemaDigest, rendered[0].SchemaDigest)
	}

	resolver := NewTargetResolver()
	resolved, err := resolver.ResolveResourceTarget(*pullQueueTarget())
	if err != nil {
		t.Fatal(err)
	}
	if resolved.RequiredInterface == nil || resolved.RequiredInterface.Name != QueuePullInterfaceName || resolved.RequiredInterface.SchemaDigest != iface.SchemaDigest {
		t.Fatalf("resolved queue target = %#v", resolved)
	}
	for name, target := range map[string]model.ResourceTarget{
		"wrong group": {Group: "topic.forms.takoform.com", Kind: "PullQueue", Contract: pullQueueTarget().Contract},
		"wrong kind":  {Group: Family.APIVersion(), Kind: "Topic", Contract: pullQueueTarget().Contract},
		"wrong interface": {Group: Family.APIVersion(), Kind: "PullQueue", Contract: model.TargetContract{
			Interface: &model.InterfaceRefSource{Name: "topic.publish", Version: "1.0.0"},
		}},
		"self exact form": {Group: Family.APIVersion(), Kind: "PullQueue", Contract: model.TargetContract{ExactForm: true}},
	} {
		name, target := name, target
		t.Run(name, func(t *testing.T) {
			if _, err := resolver.ResolveResourceTarget(target); err == nil {
				t.Fatalf("ResolveResourceTarget(%s) succeeded", name)
			}
		})
	}

	refs, err := resolver.TargetFormRefs("PullQueue")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].APIVersion != Family.APIVersion() || refs[0].Kind != "PullQueue" || refs[0].DefinitionVersion != "0.1.0" || !strings.HasPrefix(refs[0].SchemaDigest, "sha256:") {
		t.Fatalf("concrete PullQueue FormRef = %#v", refs)
	}
	relations, err := resolver.ResolveExactFormRelations(refs[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(relations) != 1 || relations[0].Pointer != "/deadLetter/queue" || relations[0].RequiredInterface == nil || relations[0].RequiredInterface.Name != QueuePullInterfaceName {
		t.Fatalf("resolved queue relations = %#v", relations)
	}
}

func TestOptionalQueueFieldsDeclareMeaning(t *testing.T) {
	for _, form := range Forms {
		for _, field := range form.Fields {
			if !field.Required && field.Default == nil && !field.AbsenceIsSemantic {
				t.Errorf("%s.%s is optional without Default or AbsenceIsSemantic", form.Kind, field.Wire)
			}
		}
	}
}
