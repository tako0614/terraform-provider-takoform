package scheduleformcatalog

import (
	"slices"
	"strings"
	"testing"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
	"github.com/tako0614/terraform-provider-takoform/internal/queueformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/topicformcatalog"
)

func TestCatalogValidates(t *testing.T) {
	if err := Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestScheduleIdentityAndDesiredFields(t *testing.T) {
	if Family.APIVersion() != "schedule.forms.takoform.com" {
		t.Fatalf("family apiVersion = %q, want versionless schedule family", Family.APIVersion())
	}
	if len(Forms) != 1 || Forms[0].Kind != "Schedule" || Forms[0].Role != model.RoleIdentity {
		t.Fatalf("forms = %#v, want one Schedule identity", Forms)
	}
	form := Forms[0]
	if form.DefinitionVersion != "0.1.0" || form.RequiresHostAPI != currentHostAPI {
		t.Fatalf("identity = %#v, want 0.1.0/v1", form)
	}
	var names []string
	for _, field := range form.Fields {
		names = append(names, field.Wire)
	}
	if !slices.Equal(names, []string{"cron", "target", "retryPolicy", "paused"}) {
		t.Fatalf("fields = %v, want cron/target/retryPolicy/paused", names)
	}
	cron := form.Fields[0]
	if !cron.Required || cron.Pattern != model.PatternCron || cron.MaxLength != 64 {
		t.Fatalf("cron = %#v, want required UTC PatternCron", cron)
	}
	target := form.Fields[1]
	if target.Kind != model.KindTaggedObject || !target.Required || target.Discriminator != "type" || len(target.Variants) != 2 {
		t.Fatalf("target = %#v, want exactly two tagged variants", target)
	}
	var tags []string
	for _, variant := range target.Variants {
		tags = append(tags, variant.Tag)
		if len(variant.Fields) != 3 {
			t.Fatalf("%s fields = %#v, want reference/body/attributes", variant.Tag, variant.Fields)
		}
		if variant.Fields[1].Kind != model.KindTaggedObject || variant.Fields[1].Wire != "body" || variant.Fields[2].Kind != model.KindStringMap || variant.Fields[2].Wire != "attributes" {
			t.Fatalf("%s payload fields = %#v", variant.Tag, variant.Fields)
		}
		resource := variant.Fields[0].ResourceTarget
		if resource == nil || resource.Contract.Interface == nil || resource.Contract.ExactForm {
			t.Fatalf("%s target = %#v, want Interface-open target", variant.Tag, resource)
		}
		switch variant.Tag {
		case "queueMessage":
			if resource.Group != queueFamilyGroup || resource.Kind != queueKind || resource.Contract.Interface.Name != queueformcatalog.QueuePullInterfaceName || resource.Contract.Interface.Version != "1.0.0" {
				t.Fatalf("queue target = %#v", resource)
			}
		case "topicPublish":
			if resource.Group != topicFamilyGroup || resource.Kind != topicKind || resource.Contract.Interface.Name != topicformcatalog.TopicPublishInterfaceName || resource.Contract.Interface.Version != "1.0.0" {
				t.Fatalf("topic target = %#v", resource)
			}
		default:
			t.Fatalf("unexpected target tag %q", variant.Tag)
		}
	}
	if !slices.Equal(tags, []string{"queueMessage", "topicPublish"}) {
		t.Fatalf("target tags = %v", tags)
	}
	retry := form.Fields[2]
	if retry.Kind != model.KindObject || !retry.Required || len(retry.Fields) != 2 {
		t.Fatalf("retryPolicy = %#v", retry)
	}
	if retry.Fields[0].Wire != "maxAttempts" || retry.Fields[0].Min == nil || retry.Fields[0].Max == nil || *retry.Fields[0].Min != 1 || *retry.Fields[0].Max != 10 || retry.Fields[1].Wire != "retryDelaySeconds" || retry.Fields[1].Min == nil || retry.Fields[1].Max == nil || *retry.Fields[1].Min != 0 || *retry.Fields[1].Max != 3600 {
		t.Fatalf("retry fields = %#v, want 1..10 and 0..3600", retry.Fields)
	}
	paused := form.Fields[3]
	if paused.Kind != model.KindBoolean || !paused.Required || paused.Default != nil {
		t.Fatalf("paused = %#v, want required pending proposal default decision", paused)
	}
}

func TestRenderedScheduleCarriesBothInterfaceRequirements(t *testing.T) {
	rendered, err := RenderForms()
	if err != nil {
		t.Fatal(err)
	}
	if len(rendered) != 1 {
		t.Fatalf("rendered = %d, want one Form", len(rendered))
	}
	item := rendered[0]
	if item.Definition.APIVersion != Family.APIVersion() || item.Definition.Kind != "Schedule" || item.Definition.DefinitionVersion != "0.1.0" {
		t.Fatalf("rendered identity = %#v", item.Definition)
	}
	if len(item.Definition.ProvidedInterfaces) != 0 {
		t.Fatalf("Schedule providedInterfaces = %#v, want none", item.Definition.ProvidedInterfaces)
	}
	properties := item.Definition.DesiredSchema["properties"].(map[string]any)
	target := properties["target"].(map[string]any)
	branches := target["oneOf"].([]any)
	if len(branches) != 2 {
		t.Fatalf("target branches = %#v", branches)
	}
	for _, raw := range branches {
		branch := raw.(map[string]any)
		branchProperties := branch["properties"].(map[string]any)
		tag := branchProperties["type"].(map[string]any)["const"].(string)
		var resource map[string]any
		var wantName, wantAPIVersion string
		switch tag {
		case "queueMessage":
			resource = branchProperties["queue"].(map[string]any)
			wantName, wantAPIVersion = queueformcatalog.QueuePullInterfaceName, queueformcatalog.InterfaceAPIVersion
		case "topicPublish":
			resource = branchProperties["topic"].(map[string]any)
			wantName, wantAPIVersion = topicformcatalog.TopicPublishInterfaceName, topicformcatalog.InterfaceAPIVersion
		default:
			t.Fatalf("unknown branch tag %q", tag)
		}
		required := resource[model.RequiredInterfaceAnnotationKey].(map[string]any)
		if required["name"] != wantName || required["version"] != "1.0.0" || required["apiVersion"] != wantAPIVersion || !strings.HasPrefix(required["schemaDigest"].(string), "sha256:") {
			t.Fatalf("%s required interface = %#v", tag, required)
		}
		if _, exact := resource[model.TargetFormRefsAnnotationKey]; exact {
			t.Fatalf("%s target unexpectedly embeds exact FormRefs", tag)
		}
		body := branchProperties["body"].(map[string]any)
		if len(body["oneOf"].([]any)) != 2 {
			t.Fatalf("%s body = %#v, want utf8/base64 variants", tag, body)
		}
	}
	if _, ok := item.Fixtures["desired.json"]; !ok {
		t.Fatal("Schedule has no canonical desired fixture")
	}
}

func TestScheduleResolverRejectsWrongCrossFamilyCoordinates(t *testing.T) {
	resolver, err := NewTargetResolver()
	if err != nil {
		t.Fatal(err)
	}
	cases := []model.ResourceTarget{
		{Group: queueFamilyGroup, Kind: "Topic", Contract: queueMessageVariant().Fields[0].ResourceTarget.Contract},
		{Group: topicFamilyGroup, Kind: topicKind, Contract: model.TargetContract{Interface: &model.InterfaceRefSource{Name: queueformcatalog.QueuePullInterfaceName, Version: "1.0.0"}}},
		{Group: "other.forms.takoform.com", Kind: queueKind, Contract: queueMessageVariant().Fields[0].ResourceTarget.Contract},
	}
	for index, target := range cases {
		if _, err := resolver.ResolveResourceTarget(target); err == nil {
			t.Fatalf("case %d unexpectedly resolved %#v", index, target)
		}
	}
}

func TestScheduleResolverPinsExactDefinition(t *testing.T) {
	resolver, err := NewTargetResolver()
	if err != nil {
		t.Fatal(err)
	}
	refs, err := resolver.TargetFormRefs("Schedule")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].APIVersion != Family.APIVersion() || refs[0].Kind != "Schedule" || refs[0].DefinitionVersion != "0.1.0" || !strings.HasPrefix(refs[0].SchemaDigest, "sha256:") {
		t.Fatalf("Schedule exact FormRef = %#v", refs)
	}
	if _, err := resolver.ResolveExactFormRelations(refs[0]); err != nil {
		t.Fatal(err)
	}
}

func TestScheduleResolverAcceptsInjectedQueueAndTopicInterfaces(t *testing.T) {
	queueRef, err := queueformcatalog.InterfaceRefFor(queueformcatalog.QueuePullInterfaceName, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	topicRef, err := topicformcatalog.InterfaceRefFor(topicformcatalog.TopicPublishInterfaceName, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := NewTargetResolver(queueRef, topicRef)
	if err != nil {
		t.Fatal(err)
	}
	queueResolved, err := resolver.ResolveResourceTarget(*queueTarget())
	if err != nil {
		t.Fatal(err)
	}
	topicResolved, err := resolver.ResolveResourceTarget(*topicTarget())
	if err != nil {
		t.Fatal(err)
	}
	if queueResolved.RequiredInterface == nil || queueResolved.RequiredInterface.SchemaDigest != queueRef.SchemaDigest {
		t.Fatalf("injected queue interface = %#v, want %s", queueResolved.RequiredInterface, queueRef.SchemaDigest)
	}
	if topicResolved.RequiredInterface == nil || topicResolved.RequiredInterface.SchemaDigest != topicRef.SchemaDigest {
		t.Fatalf("injected topic interface = %#v, want %s", topicResolved.RequiredInterface, topicRef.SchemaDigest)
	}
}

func TestOptionalScheduleFieldsDeclareMeaning(t *testing.T) {
	for _, form := range Forms {
		for _, field := range form.Fields {
			if !field.Required && field.Default == nil && !field.AbsenceIsSemantic {
				t.Errorf("%s.%s is optional without Default or AbsenceIsSemantic", form.Kind, field.Wire)
			}
		}
	}
}
