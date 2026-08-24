// Package scheduleformcatalog declares the provider-neutral Schedule Form
// Family. A Schedule owns a UTC cron and one closed message target; provider
// resource names, endpoints, credentials, and delivery implementations do
// not belong in this package.
package scheduleformcatalog

import (
	"fmt"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
	"github.com/tako0614/terraform-provider-takoform/internal/queueformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/topicformcatalog"
)

// Family is the versionless Schedule family group. A Form's own definition
// version and schema digest identify its exact contract.
var Family = model.Family{Group: "schedule.forms.takoform.com"}

const (
	definitionVersion = "0.1.0"
	currentHostAPI    = "forms.takoform.com/v1"

	queueFamilyGroup = "queue.forms.takoform.com"
	queueKind        = "PullQueue"
	topicFamilyGroup = "topic.forms.takoform.com"
	topicKind        = "Topic"

	messageBodyDataMaxLength = 262144
	messageBodyBase64MaxLen  = 4 * ((messageBodyDataMaxLength + 2) / 3)
	messageBodyUTF8Pattern   = `^[\s\S]*$`
	messageBodyBase64Pattern = `^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$`
	messageAttributeMaxLen   = 262144
	messageAttributePattern  = `^[\s\S]*$`
)

func ref(group, kind, name string) map[string]any {
	return map[string]any{"apiVersion": group, "kind": kind, "name": name}
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

func queueTarget() *model.ResourceTarget {
	return interfaceTarget(queueFamilyGroup, queueKind, queueformcatalog.QueuePullInterfaceName, "1.0.0")
}

func topicTarget() *model.ResourceTarget {
	return interfaceTarget(topicFamilyGroup, topicKind, topicformcatalog.TopicPublishInterfaceName, "1.0.0")
}

// messageBodyField is the common declarative message body. It intentionally
// uses the closed tagged object required by the Schedule desired contract,
// rather than a raw string-or-object union. The Interface/host enforces the
// aggregate body-plus-attributes byte limit.
func messageBodyField() model.Field {
	return model.Field{
		HCL: "body", Wire: "body", Kind: model.KindTaggedObject, Required: true,
		Discriminator: "encoding",
		Doc: "Message body as UTF-8 text or canonical RFC 4648 base64 bytes. The " +
			"body and attributes together are bounded to 262144 bytes by the target " +
			"Interface; the desired union keeps the encoding explicit.",
		Example: map[string]any{"encoding": "utf8", "data": "scheduled.message"},
		Variants: []model.TaggedObjectVariant{
			{Tag: "utf8", Fields: []model.Field{{
				HCL: "data", Wire: "data", Kind: model.KindString, Required: true,
				Pattern: messageBodyUTF8Pattern, MaxLength: messageBodyDataMaxLength,
				Doc:     "UTF-8 message text whose encoded byte length contributes to the aggregate message limit.",
				Example: "scheduled.message", CounterExample: true,
			}}},
			{Tag: "base64", Fields: []model.Field{{
				HCL: "data", Wire: "data", Kind: model.KindString, Required: true,
				Pattern: messageBodyBase64Pattern, MaxLength: messageBodyBase64MaxLen,
				Doc:     "RFC 4648 base64 text for opaque message bytes. Padding is canonical.",
				Example: "c2NoZWR1bGVkLm1lc3NhZ2U=", CounterExample: "not base64!",
			}}},
		},
	}
}

func attributesField() model.Field {
	return model.Field{
		HCL: "attributes", Wire: "attributes", Kind: model.KindStringMap, Required: true,
		ItemPattern: messageAttributePattern, MaxLength: messageAttributeMaxLen, MaxProperties: 10,
		Doc: "At most ten named string attributes. Attribute names use the portable map-key grammar; values " +
			"contribute to the aggregate message bound with the body.",
		Example: map[string]any{"source": "schedule"},
	}
}

func queueMessageVariant() model.TaggedObjectVariant {
	return model.TaggedObjectVariant{
		Tag: "queueMessage",
		Fields: []model.Field{
			{
				HCL: "queue", Wire: "queue", Kind: model.KindResourceRef, Required: true,
				ResourceTarget: queueTarget(),
				Doc:            "PullQueue providing queue.pull@1.0.0. The host pins the resolved resource's exact FormRef at admission.",
				Example:        ref(queueFamilyGroup, queueKind, "scheduled-work"),
			},
			messageBodyField(), attributesField(),
		},
	}
}

func topicPublishVariant() model.TaggedObjectVariant {
	return model.TaggedObjectVariant{
		Tag: "topicPublish",
		Fields: []model.Field{
			{
				HCL: "topic", Wire: "topic", Kind: model.KindResourceRef, Required: true,
				ResourceTarget: topicTarget(),
				Doc:            "Topic providing topic.publish@1.0.0. The host pins the resolved resource's exact FormRef at admission.",
				Example:        ref(topicFamilyGroup, topicKind, "scheduled-events"),
			},
			messageBodyField(), attributesField(),
		},
	}
}

// Forms is the complete Schedule Family MVP set. ResourceType and provider
// names are deliberately absent: they are not Form identity or semantics.
var Forms = []model.Form{{
	Family: Family, Kind: "Schedule", Slug: "schedule", Role: model.RoleIdentity,
	DefinitionVersion: definitionVersion, RequiresHostAPI: currentHostAPI,
	Title: "Schedule",
	Description: "UTC five-field cron schedule that delivers one declared message at each " +
		"matched window to either a PullQueue or Topic. Delivery is at least once; failed " +
		"attempts use the declared bounded retry policy and missed windows are never replayed.",
	Fields: []model.Field{
		{
			HCL: "cron", Wire: "cron", Kind: model.KindString, Required: true,
			Pattern: model.PatternCron, MaxLength: 64,
			Doc: "Portable five-field cron expression, interpreted in UTC only. A host also parses it and refuses " +
				"values outside each field's domain, inverted ranges, and out-of-span steps.",
			Example: "*/5 * * * *", AltExample: "0 3 * * *", CounterExample: "0 3 * *",
		},
		{
			HCL: "target", Wire: "target", Kind: model.KindTaggedObject, Required: true,
			Discriminator: "type",
			Doc: "Exactly one message target: queueMessage sends to queue.pull@1.0.0, or topicPublish " +
				"publishes to topic.publish@1.0.0. The target resource reference is explicit and uid-pinned.",
			Example: map[string]any{
				"type": "queueMessage", "queue": ref(queueFamilyGroup, queueKind, "scheduled-work"),
				"body":       map[string]any{"encoding": "utf8", "data": "scheduled.message"},
				"attributes": map[string]any{"source": "schedule"},
			},
			Variants: []model.TaggedObjectVariant{queueMessageVariant(), topicPublishVariant()},
		},
		{
			HCL: "retry_policy", Wire: "retryPolicy", Kind: model.KindObject, Required: true,
			Doc:     "Bounded fixed-delay retry policy for one matched window. maxAttempts includes the first attempt.",
			Example: map[string]any{"maxAttempts": 3, "retryDelaySeconds": 60},
			Fields: []model.Field{
				{
					HCL: "max_attempts", Wire: "maxAttempts", Kind: model.KindInteger, Required: true,
					Min: model.I64(1), Max: model.I64(10),
					Doc: "Maximum attempts for one matched window, counting the first attempt.", Example: 3, CounterExample: 0,
				},
				{
					HCL: "retry_delay_seconds", Wire: "retryDelaySeconds", Kind: model.KindInteger, Required: true,
					Min: model.I64(0), Max: model.I64(3600),
					Doc: "Fixed delay between failed attempts, in seconds.", Example: 60, CounterExample: 3601,
				},
			},
		},
		{
			HCL: "paused", Wire: "paused", Kind: model.KindBoolean, Required: true,
			Doc: "While true, matched windows are permanently skipped and are not replayed when unpaused. " +
				"The field is required because the proposal declares no portable omission default.",
			Example: false, AltExample: true,
		},
	},
}}

// Validate proves the family source is closed and every Form is coherent.
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
			return fmt.Errorf("duplicate Schedule family identity %s/%s", form.Kind, form.Slug)
		}
		seenKinds[form.Kind], seenSlugs[form.Slug] = true, true
		if form.DefinitionVersion != definitionVersion {
			return fmt.Errorf("form %s definition version %q, want %q", form.Kind, form.DefinitionVersion, definitionVersion)
		}
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
