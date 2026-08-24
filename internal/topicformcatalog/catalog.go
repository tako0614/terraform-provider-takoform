// Package topicformcatalog declares the provider-neutral Fanout Topic Form
// Family.  The package contains data-only Form and Interface sources: provider
// resource names, endpoints, credentials, and delivery implementations do not
// belong here.
package topicformcatalog

import (
	"fmt"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// Family is the versionless Topic family group.  A Form's own definition
// version and schema digest are its identity; the family never carries a
// latest/default version.
var Family = model.Family{Group: "topic.forms.takoform.com"}

const (
	definitionVersion = "0.1.0"
	firstHostAPI      = "forms.takoform.com/v1"
	currentHostAPI    = "forms.takoform.com/v1"

	// Queue and topic messages use one common declarative body shape.  The
	// desired state is deliberately a closed tagged object rather than a raw
	// string-or-object JSON union; this keeps the host's input contract typed
	// without embedding a data-plane protocol in the Form model.
	messageBodyDataMaxLength = 262144
	messageBodyBase64MaxLen  = 4 * ((messageBodyDataMaxLength + 2) / 3)
	messageBodyUTF8Pattern   = `^[\s\S]*$`
	messageBodyBase64Pattern = `^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$`
	messageAttributeMaxLen   = 262144
	messageAttributePattern  = `^[\s\S]*$`
	filterValuePattern       = `^[\s\S]+$`
	filterValueMaxLength     = messageAttributeMaxLen
)

const (
	queueFamilyGroup       = "queue.forms.takoform.com"
	queueKind              = "PullQueue"
	queuePullInterfaceName = "queue.pull"
)

// ref renders a closed resource reference fixture.
func ref(group, kind, name string) map[string]any {
	return map[string]any{"apiVersion": group, "kind": kind, "name": name}
}

func sameFamilyRef(kind, name string) map[string]any {
	return ref(Family.APIVersion(), kind, name)
}

func exactTarget(group, kind string) *model.ResourceTarget {
	return &model.ResourceTarget{
		Group: group, Kind: kind, Contract: model.TargetContract{ExactForm: true},
	}
}

func interfaceTarget(group, kind, name, version string) *model.ResourceTarget {
	return &model.ResourceTarget{
		Group: group,
		Kind:  kind,
		Contract: model.TargetContract{Interface: &model.InterfaceRefSource{
			Name: name, Version: version,
		}},
	}
}

// messageBodyField is the common declarative message body.  The host applies
// the aggregate 262144-byte message bound (including attributes); the schema
// carries the per-encoding structural ceilings.
func messageBodyField() model.Field {
	return model.Field{
		HCL: "body", Wire: "body", Kind: model.KindTaggedObject, Required: true,
		Discriminator: "encoding",
		Doc: "Message body as UTF-8 text or canonical RFC 4648 base64 bytes. The " +
			"body and attributes together are bounded to 262144 bytes by the target " +
			"Interface; the desired union keeps the encoding explicit.",
		Example: map[string]any{"encoding": "utf8", "data": "order.created"},
		Variants: []model.TaggedObjectVariant{
			{
				Tag: "utf8",
				Fields: []model.Field{{
					HCL: "data", Wire: "data", Kind: model.KindString, Required: true,
					Pattern: messageBodyUTF8Pattern, MaxLength: messageBodyDataMaxLength,
					Doc: "UTF-8 message text. Its encoded byte length contributes to the " +
						"aggregate message limit.",
					Example: "order.created", CounterExample: true,
				}},
			},
			{
				Tag: "base64",
				Fields: []model.Field{{
					HCL: "data", Wire: "data", Kind: model.KindString, Required: true,
					Pattern: messageBodyBase64Pattern, MaxLength: messageBodyBase64MaxLen,
					Doc: "RFC 4648 base64 text for opaque message bytes. Padding is " +
						"canonical and its decoded length contributes to the aggregate message limit.",
					Example: "b3JkZXIuY3JlYXRlZA==", CounterExample: "not base64!",
				}},
			},
		},
	}
}

func attributesField(hcl, wire string, required bool) model.Field {
	return model.Field{
		HCL: hcl, Wire: wire, Kind: model.KindStringMap, Required: required,
		ItemPattern: messageAttributePattern, MaxLength: messageAttributeMaxLen,
		MaxProperties: 10,
		Doc: "At most ten named string attributes. Attribute names use the " +
			"portable map-key grammar; values contribute to the aggregate message " +
			"bound with the body.",
		Example: map[string]any{"eventType": "order.created"},
	}
}

func filterPolicyField() model.Field {
	return model.Field{
		HCL: "filter_policy", Wire: "filterPolicy", Kind: model.KindStringSetMap,
		AbsenceIsSemantic: true, ItemPattern: filterValuePattern,
		MaxLength: filterValueMaxLength, MinItems: 1, MaxItems: 16, MaxProperties: 10,
		Doc: "Optional closed attribute-equality filter. When absent, every " +
			"message matches; each named attribute must equal one of its non-empty " +
			"candidate strings. There is no range, prefix, negation, or payload-path " +
			"matching.",
		Example: map[string]any{"eventType": []any{"order.created", "order.updated"}},
	}
}

func retryDelayField() model.Field {
	return model.Field{
		HCL: "retry_delay_seconds", Wire: "retryDelaySeconds", Kind: model.KindInteger,
		Required: true, Min: model.I64(0), Max: model.I64(3600),
		Doc: "Delay before a failed delivery attempt is retried, in seconds.", Example: 60,
	}
}

func maxRetriesField() model.Field {
	return model.Field{
		HCL: "max_retries", Wire: "maxRetries", Kind: model.KindInteger,
		Required: true, Min: model.I64(0), Max: model.I64(100),
		Doc: "Number of delivery retries after the first attempt. A message is " +
			"attempted at most one plus this value times.", Example: 3,
	}
}

func queueInterfaceTarget() *model.ResourceTarget {
	return interfaceTarget(queueFamilyGroup, queueKind, queuePullInterfaceName, "1.0.0")
}

// Forms is the complete Fanout Topic Family MVP set, in stable order.
// Provider resource names are intentionally absent: they are assigned by the
// provider-owned registry and are not Form identity.
var Forms = []model.Form{
	{
		Family: Family, Kind: "Topic", Slug: "topic", Role: model.RoleIdentity,
		DefinitionVersion: definitionVersion, RequiresHostAPI: firstHostAPI,
		Title: "Topic",
		Description: "Fanout topic whose accepted publishes are delivered at least once " +
			"to every matching TopicSubscription. The topic retains and replays nothing; " +
			"the topic.publish Interface fixes the message and publish semantics.",
		ProvidedInterfaces: []model.InterfaceRefSource{{Name: TopicPublishInterfaceName, Version: "1.0.0"}},
	},
	{
		Family: Family, Kind: "TopicSubscription", Slug: "topic-subscription", Role: model.RoleAttachment,
		DefinitionVersion: definitionVersion, RequiresHostAPI: currentHostAPI,
		Title: "Topic Subscription",
		Description: "Attachment that delivers each matching Topic publish into one PullQueue. " +
			"Delivery is independent and at least once per subscription; retry and dead-letter " +
			"behavior belong to this attachment.",
		Fields: []model.Field{
			{
				HCL: "topic", Wire: "topic", Kind: model.KindResourceRef,
				ResourceTarget: exactTarget(Family.APIVersion(), "Topic"),
				Required:       true, Immutable: true,
				Doc:     "Exact Topic identity whose accepted publishes this subscription receives. Changing it replaces the attachment.",
				Example: sameFamilyRef("Topic", "topic"),
			},
			{
				HCL: "target", Wire: "target", Kind: model.KindResourceRef,
				ResourceTarget: queueInterfaceTarget(),
				Required:       true, Immutable: true,
				Doc:     "PullQueue resource providing queue.pull@1.0.0. Changing it replaces the attachment.",
				Example: ref(queueFamilyGroup, queueKind, "events"),
			},
			filterPolicyField(),
			retryDelayField(),
			maxRetriesField(),
			{
				HCL: "dead_letter", Wire: "deadLetter", Kind: model.KindResourceRef,
				ResourceTarget: queueInterfaceTarget(), AbsenceIsSemantic: true,
				Doc: "Optional PullQueue receiving messages that exhaust delivery retries. " +
					"When omitted, exhausted messages are dropped. It must resolve to a " +
					"different queue from target.",
				Example: ref(queueFamilyGroup, queueKind, "dead-letters"),
			},
		},
		ResolvedUIDConstraints: []model.Constraint{
			{Kind: model.ConstraintDistinctPair, References: []string{"/target", "/deadLetter"}},
			{Kind: model.ConstraintUniquePair, References: []string{"/topic", "/target"}},
		},
	},
}

// Validate proves the family source is closed and every Form is internally
// coherent. External Interface and exact target identities are injected at
// RenderForms time, when their canonical bytes are available.
func Validate() error {
	if err := model.ValidateNoOpenTokens(Forms); err != nil {
		return err
	}
	seenKinds, seenSlugs := map[string]bool{}, map[string]bool{}
	for _, form := range Forms {
		if err := form.Validate(); err != nil {
			return err
		}
		if form.Family != Family {
			return fmt.Errorf("form %s belongs to family %s, want %s", form.Kind, form.Family.APIVersion(), Family.APIVersion())
		}
		if seenKinds[form.Kind] || seenSlugs[form.Slug] {
			return fmt.Errorf("duplicate Topic family identity %s/%s", form.Kind, form.Slug)
		}
		seenKinds[form.Kind], seenSlugs[form.Slug] = true, true
		if form.DefinitionVersion != definitionVersion {
			return fmt.Errorf("form %s definition version %q, want %q", form.Kind, form.DefinitionVersion, definitionVersion)
		}
	}
	if err := ValidateInterfaceDefinitions(InterfaceDefinitions()); err != nil {
		return err
	}
	return nil
}

// ByKind returns one source Form by its exact portable kind.
func ByKind(kind string) (model.Form, bool) {
	for _, form := range Forms {
		if form.Kind == kind {
			return form, true
		}
	}
	return model.Form{}, false
}
