package topicformcatalog

import (
	"slices"
	"strings"
	"testing"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
	"github.com/tako0614/terraform-provider-takoform/internal/queueformcatalog"
)

func TestCatalogValidates(t *testing.T) {
	if err := Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogIdentityAndSubscriptionFields(t *testing.T) {
	if Family.APIVersion() != "topic.forms.takoform.com" {
		t.Fatalf("family apiVersion = %q, want versionless topic family", Family.APIVersion())
	}
	if len(Forms) != 2 || Forms[0].Kind != "Topic" || Forms[0].Role != model.RoleIdentity || Forms[1].Kind != "TopicSubscription" || Forms[1].Role != model.RoleAttachment {
		t.Fatalf("forms = %#v, want Topic identity and TopicSubscription attachment", Forms)
	}
	if Forms[0].DefinitionVersion != "0.1.0" || Forms[1].DefinitionVersion != "0.1.0" {
		t.Fatalf("definition versions = %q, %q", Forms[0].DefinitionVersion, Forms[1].DefinitionVersion)
	}
	if len(Forms[0].Fields) != 0 || len(Forms[0].ProvidedInterfaces) != 1 || Forms[0].ProvidedInterfaces[0].Name != TopicPublishInterfaceName || Forms[0].ProvidedInterfaces[0].Version != "1.0.0" {
		t.Fatalf("Topic declaration = %#v, want no fields and topic.publish@1.0.0", Forms[0])
	}
	form := Forms[1]
	if form.RequiresHostAPI != currentHostAPI {
		t.Fatalf("TopicSubscription requiresHostApi = %q, want %q for nested/constraint support", form.RequiresHostAPI, currentHostAPI)
	}
	var names []string
	for _, field := range form.Fields {
		names = append(names, field.Wire)
	}
	wantNames := []string{"topic", "target", "filterPolicy", "retryDelaySeconds", "maxRetries", "deadLetter"}
	if !slices.Equal(names, wantNames) {
		t.Fatalf("TopicSubscription fields = %v, want %v", names, wantNames)
	}
	topicTarget := form.Fields[0].ResourceTarget
	if topicTarget == nil || topicTarget.Group != Family.APIVersion() || topicTarget.Kind != "Topic" || !topicTarget.Contract.ExactForm || topicTarget.Contract.Interface != nil {
		t.Fatalf("topic target = %#v, want exact Topic Form target", topicTarget)
	}
	queueTarget := form.Fields[1].ResourceTarget
	if queueTarget == nil || queueTarget.Group != queueFamilyGroup || queueTarget.Kind != queueKind || queueTarget.Contract.Interface == nil || queueTarget.Contract.Interface.Name != queuePullInterfaceName || queueTarget.Contract.Interface.Version != "1.0.0" {
		t.Fatalf("queue target = %#v, want queue.pull@1.0.0", queueTarget)
	}
	filter := form.Fields[2]
	if filter.Required || !filter.AbsenceIsSemantic || filter.MinItems != 1 || filter.MaxItems != 16 || filter.MaxProperties != 10 {
		t.Fatalf("filterPolicy = %#v, want optional semantic 1..16 values/10 keys", filter)
	}
	deadLetter := form.Fields[5]
	if deadLetter.Required || !deadLetter.AbsenceIsSemantic || deadLetter.Kind != model.KindResourceRef || deadLetter.ResourceTarget == nil {
		t.Fatalf("deadLetter = %#v, want optional semantic PullQueue reference", deadLetter)
	}
	if got := deadLetter.ResourceTarget; got == nil || got.Group != queueFamilyGroup || got.Kind != queueKind || got.Contract.Interface == nil || got.Contract.Interface.Name != queuePullInterfaceName {
		t.Fatalf("deadLetter target = %#v, want queue.pull@1.0.0", got)
	}
	constraints := form.Constraints()
	if len(constraints) != 2 || constraints[0].Kind != model.ConstraintDistinctPair || constraints[1].Kind != model.ConstraintUniquePair {
		t.Fatalf("constraints = %#v, want distinct and unique pair", constraints)
	}
	if !slices.Equal(constraints[0].References, []string{"/target", "/deadLetter"}) || !slices.Equal(constraints[1].References, []string{"/topic", "/target"}) {
		t.Fatalf("constraints references = %#v, want target/deadLetter and topic/target", constraints)
	}
}

func TestOptionalTopicFieldsDeclareMeaning(t *testing.T) {
	for _, form := range Forms {
		for _, field := range form.Fields {
			if !field.Required && field.Default == nil && !field.AbsenceIsSemantic {
				t.Errorf("%s.%s is optional without Default or AbsenceIsSemantic", form.Kind, field.Wire)
			}
		}
	}
}

func TestRenderedTopicFormsCarryExactAndInterfaceContracts(t *testing.T) {
	rendered, err := RenderForms()
	if err != nil {
		t.Fatal(err)
	}
	if len(rendered) != 2 {
		t.Fatalf("rendered = %d, want two Forms", len(rendered))
	}
	if len(rendered[0].Definition.ProvidedInterfaces) != 1 {
		t.Fatalf("Topic providedInterfaces = %#v", rendered[0].Definition.ProvidedInterfaces)
	}
	provided := rendered[0].Definition.ProvidedInterfaces[0]
	if provided.APIVersion != InterfaceAPIVersion || provided.Name != TopicPublishInterfaceName || provided.Version != "1.0.0" || !strings.HasPrefix(provided.SchemaDigest, "sha256:") {
		t.Fatalf("provided interface = %#v", provided)
	}
	properties := rendered[1].Definition.DesiredSchema["properties"].(map[string]any)
	topic := properties["topic"].(map[string]any)
	refs := topic[model.TargetFormRefsAnnotationKey].([]any)
	if len(refs) != 1 {
		t.Fatalf("topic exact refs = %#v", refs)
	}
	exact := refs[0].(map[string]any)
	if exact["apiVersion"] != Family.APIVersion() || exact["kind"] != "Topic" || exact["definitionVersion"] != "0.1.0" || !strings.HasPrefix(exact["schemaDigest"].(string), "sha256:") {
		t.Fatalf("topic exact ref = %#v", exact)
	}
	target := properties["target"].(map[string]any)
	required := target[model.RequiredInterfaceAnnotationKey].(map[string]any)
	if required["apiVersion"] != queueformcatalog.InterfaceAPIVersion || required["name"] != queuePullInterfaceName || required["version"] != "1.0.0" || !strings.HasPrefix(required["schemaDigest"].(string), "sha256:") {
		t.Fatalf("queue required interface = %#v", required)
	}
	if len(rendered[1].Definition.Constraints) != 2 {
		t.Fatalf("rendered constraints = %#v", rendered[1].Definition.Constraints)
	}
}

func TestTopicExactResolverPinsRenderedDefinition(t *testing.T) {
	resolver, err := NewTargetResolver()
	if err != nil {
		t.Fatal(err)
	}
	refs, err := resolver.TargetFormRefs("Topic")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].APIVersion != Family.APIVersion() || refs[0].Kind != "Topic" || refs[0].DefinitionVersion != "0.1.0" || !strings.HasPrefix(refs[0].SchemaDigest, "sha256:") {
		t.Fatalf("Topic exact FormRef = %#v", refs)
	}
	if _, err := resolver.ResolveExactFormRelations(refs[0]); err != nil {
		t.Fatal(err)
	}
}

func TestTopicResolverAcceptsInjectedQueueInterface(t *testing.T) {
	queueRef, err := QueuePullInterfaceRef()
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := NewTargetResolver(queueRef)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := resolver.ResolveResourceTarget(*queueInterfaceTarget())
	if err != nil {
		t.Fatal(err)
	}
	if resolved.RequiredInterface == nil || resolved.RequiredInterface.SchemaDigest != queueRef.SchemaDigest {
		t.Fatalf("injected queue interface = %#v, want %s", resolved.RequiredInterface, queueRef.SchemaDigest)
	}
}
