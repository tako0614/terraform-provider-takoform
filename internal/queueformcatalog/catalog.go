// Package queueformcatalog declares the provider-neutral Pull Queue Form
// Family. The catalog owns the queue's desired contract and exact Interface
// reference only; provider resource names, credentials, endpoints, and queue
// runtime implementations live outside this package.
package queueformcatalog

import (
	"fmt"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// Family is the versionless Pull Queue Form Family group. A Form's own
// definition SemVer and schema digest identify its exact contract.
var Family = model.Family{Group: "queue.forms.takoform.com"}

const (
	definitionVersion = "0.1.0"
	firstHostAPI      = "forms.takoform.com/v1"
	currentHostAPI    = "forms.takoform.com/v1"
)

func ref(kind, name string) map[string]any {
	return map[string]any{"apiVersion": Family.APIVersion(), "kind": kind, "name": name}
}

// pullQueueTarget is deliberately an exact Group+Kind plus Interface
// requirement. A self-referential exact FormRef would put PullQueue's own
// schemaDigest inside the bytes whose digest it is, which has no honest
// content-hash fixed point. The host resolves the concrete target Resource's
// exact FormRef at relation admission; the required Interface pins the
// behavior the dead-letter move actually depends on.
func pullQueueTarget() *model.ResourceTarget {
	return &model.ResourceTarget{
		Group: Family.APIVersion(), Kind: "PullQueue",
		Contract: model.TargetContract{Interface: &model.InterfaceRefSource{
			Name: QueuePullInterfaceName, Version: "1.0.0",
		}},
	}
}

// Forms is the complete Pull Queue Family MVP set, in stable order.
// ResourceType is intentionally absent: provider resource names are not Form
// identity or semantics.
var Forms = []model.Form{
	{
		Family: Family, Kind: "PullQueue", Slug: "pull-queue", Role: model.RoleIdentity,
		DefinitionVersion: definitionVersion, RequiresHostAPI: currentHostAPI,
		Title: "Pull Queue", Description: "Unordered at-least-once pull queue with visibility timeout, " +
			"receive counting, optional dead-lettering, and bounded long polling. The queue.pull Interface " +
			"fixes the send, receive, delete, and changeVisibility behavior; these fields are the queue's " +
			"retention and delivery-policy declaration.",
		Fields: []model.Field{
			{
				HCL: "message_retention_seconds", Wire: "messageRetentionSeconds", Kind: model.KindInteger,
				Required: true, Min: model.I64(60), Max: model.I64(1209600),
				Doc: "Maximum lifetime, in seconds, of an undeleted message. A message older than this bound expires " +
					"and is removed.",
				Example: 345600, AltExample: 604800, CounterExample: 59,
			},
			{
				HCL: "default_visibility_timeout_seconds", Wire: "defaultVisibilityTimeoutSeconds", Kind: model.KindInteger,
				Required: true, Min: model.I64(0), Max: model.I64(43200),
				Doc: "Default invisibility duration, in seconds, applied by receive when its visibility timeout is " +
					"omitted. A received message becomes deliverable again after this duration unless deleted or its " +
					"visibility is changed.",
				Example: 30, AltExample: 0, CounterExample: 43201,
			},
			{
				HCL: "receive_wait_bound_seconds", Wire: "receiveWaitBoundSeconds", Kind: model.KindInteger,
				Required: true, Min: model.I64(0), Max: model.I64(20),
				Doc: "Maximum long-poll wait, in seconds, accepted by receive. A receive's waitSeconds is bounded by " +
					"this value; zero means receive returns without waiting.",
				Example: 20, AltExample: 0, CounterExample: 21,
			},
			{
				HCL: "dead_letter", Wire: "deadLetter", Kind: model.KindObject,
				AbsenceIsSemantic: true,
				Doc: "Optional dead-letter policy. Without it, a message that exceeds its receive count is removed " +
					"only by deletion or retention expiry; when present, the host moves an exhausted message to queue " +
					"as a new message after maxReceiveCount.",
				Fields: []model.Field{
					{
						HCL: "queue", Wire: "queue", Kind: model.KindResourceRef, Required: true,
						ResourceTarget: pullQueueTarget(),
						Doc: "PullQueue that receives exhausted messages as new messages. It must not be this queue or " +
							"close a dead-letter cycle; the host rejects self and cyclic resolved-UID graphs.",
						Example: ref("PullQueue", "dead-letters"),
					},
					{
						HCL: "max_receive_count", Wire: "maxReceiveCount", Kind: model.KindInteger, Required: true,
						Min: model.I64(1), Max: model.I64(1000),
						Doc: "Receive count at which the next receive moves the message to queue instead of delivering " +
							"it again. The count starts at one on first delivery.",
						Example: 5, AltExample: 100, CounterExample: 0,
					},
				},
				Example: map[string]any{
					"queue": ref("PullQueue", "dead-letters"), "maxReceiveCount": 5,
				},
			},
		},
		ProvidedInterfaces: []model.InterfaceRefSource{{Name: QueuePullInterfaceName, Version: "1.0.0"}},
		ResolvedUIDConstraints: []model.Constraint{{
			Kind: model.ConstraintAcyclic, Reference: "/deadLetter/queue",
		}},
	},
}

// Validate proves the provider-neutral catalog is closed and each Form's
// declaration is internally coherent. Interface and Definition checks also
// run during RenderForms, but remain explicit for source-only callers.
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
			return fmt.Errorf("duplicate Queue family identity %s/%s", form.Kind, form.Slug)
		}
		seenKinds[form.Kind], seenSlugs[form.Slug] = true, true
		if form.DefinitionVersion != definitionVersion {
			return fmt.Errorf("form %s definition version %q, want %q", form.Kind, form.DefinitionVersion, definitionVersion)
		}
	}
	return ValidateInterfaceDefinitions(InterfaceDefinitions())
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
