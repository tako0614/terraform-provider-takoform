package standardforms

// publish_surfaces_v3.go renders the human-facing surfaces of the Host API
// v1beta1 resource lane — one reference document and one example per Edge
// Platform Family resource — from the single catalog declaration in
// internal/edgeformcatalog. Generation and verification share these exact
// bytes, exactly like the retained v2 renderer above them.

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
	"github.com/tako0614/terraform-provider-takoform/internal/edgeformcatalog"
)

// v3DocBasename mirrors docBasename for the family lane.
func v3DocBasename(form model.Form) string {
	return strings.TrimPrefix(form.ResourceType, "takoform_") + ".md"
}

// v3PublishedSurfaces renders every family-lane doc and example. Every v3-lane
// surface is derived from a catalog Form; the lane has no hand-authored
// resource surface, because it exposes no resource that is not a Form
// (spec/decisions/0021).
func v3PublishedSurfaces() []publishedSurface {
	surfaces := make([]publishedSurface, 0, len(edgeformcatalog.Forms)*2)
	for _, form := range edgeformcatalog.Forms {
		surfaces = append(surfaces,
			publishedSurface{
				path:    filepath.ToSlash(filepath.Join("docs", "resources", v3DocBasename(form))),
				content: []byte(v3ResourceDoc(form)),
			},
			publishedSurface{
				path:    filepath.ToSlash(filepath.Join("examples", "resources", form.ResourceType, "resource.tf")),
				content: []byte(v3ExampleHCL(form)),
			},
		)
	}
	return surfaces
}

// v3ProvidedInterfaceProse says which DIRECTION one provided contract runs in.
// Most provided Interfaces are an operation surface the resource exposes to
// other resources through a Binding. The runtime ABI is the other direction:
// it is what a conforming host provides to the code the resource runs, and it
// is what "this host runs module workers" is allowed to mean
// (spec/decisions/0019).
func v3ProvidedInterfaceProse(name string) string {
	if name == edgeformcatalog.WorkerRuntimeInterfaceName {
		return "the exact runtime ABI a conforming host provides to this resource's code: " +
			"handler signatures, the binding environment, `ctx.waitUntil`, exception handling, " +
			"body streaming, the minimum Web API surface, and module loading. " +
			"A host supporting this Form implements that contract at its exact digest."
	}
	return "the exact Interface contract this Form's service exposes."
}

// v3RoleSemantics states the lifecycle meaning of each closed role.
func v3RoleSemantics(role model.Role) string {
	switch role {
	case model.RoleIdentity:
		return "This is an `identity` resource: a long-lived logical identity with a stable name, updated in place."
	case model.RoleRevision:
		return "This is a `revision` resource: an immutable snapshot. It is create-only — every desired attribute forces replacement, and rollback means pointing a deployment at an earlier revision, never editing this one."
	case model.RoleDeployment:
		return "This is a `deployment` resource: the only mutable path for traffic movement and rollback. It selects which revisions are active."
	case model.RoleAttachment:
		return "This is an `attachment` resource: it connects a parent to inward activation (routes, domains, schedules, queue consumption). Deleting the attachment never deletes the parent."
	case model.RolePolicy:
		return "This is a `policy` resource: operating rules changed independently of the parent identity."
	default:
		return ""
	}
}

func v3DocType(field model.Field) string {
	switch field.Kind {
	case model.KindBoolean:
		return "Bool"
	case model.KindInteger:
		return "Number"
	case model.KindStringSet:
		return "Set of String"
	case model.KindBindingList, model.KindObjectList:
		return "List of Object"
	case model.KindObject:
		return "Object"
	default:
		return "String"
	}
}

func v3DocRequirement(form model.Form, field model.Field) string {
	requirement := "optional"
	if field.Required {
		requirement = "required"
	}
	// A Form that declares no update has no in-place path for any desired
	// attribute, so every one of them forces replacement.
	if field.Immutable || !form.DeclaresUpdate() {
		requirement += ", forces replacement"
	}
	return requirement
}

// v3DocDefault states the portable meaning of omitting an optional argument.
// Every optional argument has one: a declared default, or an explicit
// absent-case behavior stated in the field's own Doc.
func v3DocDefault(field model.Field) string {
	if field.Default == nil {
		return ""
	}
	if model.EmptyCollectionDefault(field) {
		if field.Kind == model.KindJSONMap {
			return " Defaults to the empty object `{}`."
		}
		return " Defaults to the empty list `[]`."
	}
	rendered, err := json.Marshal(field.Default)
	if err != nil {
		return ""
	}
	return " Defaults to `" + string(rendered) + "`."
}

func v3DocConstraint(field model.Field) string {
	var parts []string
	if len(field.Enum) > 0 {
		parts = append(parts, "One of `"+strings.Join(field.Enum, "`, `")+"`.")
	}
	if field.Min != nil && field.Max != nil {
		parts = append(parts, fmt.Sprintf("Between %d and %d.", *field.Min, *field.Max))
	} else if field.Min != nil {
		parts = append(parts, fmt.Sprintf("At least %d.", *field.Min))
	} else if field.Max != nil {
		parts = append(parts, fmt.Sprintf("At most %d.", *field.Max))
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

// v3FieldDocLine renders one argument bullet for the v3 HCL surface.
func v3FieldDocLine(form model.Form, field model.Field) string {
	name := field.HCL
	doc := field.Doc
	if field.Pattern == model.PatternCron {
		// The grammar is enforced by a PARSER, not only by the pattern, so the
		// documentation states both halves: what the shape admits, and what the
		// parser then refuses. A loose "cron expression" claim would promise
		// syntax the provider rejects at plan time.
		doc = "UTC cron schedule. Five fields separated by single spaces — minute " +
			"`0`-`59`, hour `0`-`23`, day-of-month `1`-`31`, month `1`-`12`, and " +
			"day-of-week `0`-`6` with `0` Sunday — where each field is a " +
			"comma-separated list of `*`, a literal, a range `low-high`, `*/step`, " +
			"or `low-high/step`. Month and day names, and a step on a bare literal " +
			"such as `5/10`, are not accepted. The provider parses the expression at " +
			"plan time exactly as the host does, so a value outside its field's " +
			"range, an inverted range such as `5-1`, or a step outside `1`..span is " +
			"a plan-time error rather than a failed apply. When day-of-month and " +
			"day-of-week are both restricted the schedule fires on a day either " +
			"selects."
	}
	docType := v3DocType(field)
	switch field.Kind {
	case model.KindJSONMap:
		name = field.HCL + "_json"
		doc += " Authored as one JSON object string (for example `jsonencode({...})`); the provider sends the parsed object."
	case model.KindResourceRef:
		doc += " Set the name of the target `" + field.TargetKind + "` resource."
	case model.KindBindingList:
		doc += " Each entry declares `name` (a JavaScript identifier) and `target_name` (the target `" +
			field.TargetKind + "` resource name); the wire carries the typed `resource` reference."
	case model.KindObjectList:
		members := make([]string, 0, len(field.Fields))
		for _, member := range field.Fields {
			item := "`" + member.HCL + "`"
			if member.Min != nil && member.Max != nil {
				item += fmt.Sprintf(" (between %d and %d)", *member.Min, *member.Max)
			}
			members = append(members, item)
		}
		doc += " Each entry declares " + strings.Join(members, ", ") + "."
		if field.MinItems > 0 && field.MaxItems > 0 {
			doc += fmt.Sprintf(" The list must declare between %d and %d entries.", field.MinItems, field.MaxItems)
		}
	case model.KindObject:
		members := make([]string, 0, len(field.Fields))
		for _, member := range field.Fields {
			members = append(members, "`"+member.HCL+"`")
		}
		doc += " The object declares " + strings.Join(members, ", ") + "; when the object is present, every member is required."
	}
	return fmt.Sprintf("- `%s` (%s, %s) — %s%s%s\n",
		name, docType, v3DocRequirement(form, field), doc, v3DocConstraint(field), v3DocDefault(field))
}

// v3NameArgumentDoc renders the `name` bullet, and — on a revision — the
// `revision_owner` bullet beside it.
//
// A revision's name is not the author's to choose: it is derived from the
// revision's own CONTENT, so that changed bytes are a new revision beside the
// old one rather than a replacement of it (spec/decisions/0029). A content
// digest names the bytes and nothing else, though, and a Terraform address has
// exactly one owner — so the derivation also takes the owner, and the two
// arguments have to be documented together or neither is usable.
func v3NameArgumentDoc(form model.Form) string {
	if form.Role != model.RoleRevision {
		return "- `name` (String, required, forces replacement) — Portable resource name (`metadata.name`).\n"
	}
	prefix := strings.TrimPrefix(form.Slug, "worker-")
	if prefix == "" {
		prefix = form.Slug
	}
	return "- `name` (String, optional, computed, forces replacement) — Portable resource name (`metadata.name`). " +
		"Omit it and set `revision_owner` instead: this Form is an immutable revision, so the provider derives " +
		"`" + prefix + "-<content digest prefix>-<owner digest prefix>` from this revision's own content and its " +
		"declared owner. Changed content is then a NEW revision created beside the old one, which is the only way " +
		"a code change applies at all — a host refuses every update to a revision, and replacing one under a name " +
		"it still holds completes in neither apply order. Setting it pins the name, which an imported revision " +
		"needs; the provider then refuses at plan time any change that would replace this revision under it.\n" +
		"- `revision_owner` (String, optional, forces replacement) — Stable name of whatever owns this revision; " +
		"the `takoform_module_worker` it belongs to is the usual answer. Required whenever `name` is omitted. " +
		"Two independent resources built from identical content derive identical content digests, so without an " +
		"owner they would derive one name and two Terraform resources would manage one host address — where a " +
		"destroy of either breaks the other. It is provider-side authoring input: no wire member carries it, the " +
		"host never sees it, and it enters only the derived name. The official " +
		"[`worker-app` module](https://github.com/tako0614/terraform-provider-takoform/tree/main/modules/worker-app) " +
		"sets it for you.\n"
}

// v3RevisionOwnerExample is the owner an example declares for a derived
// revision name: the Module Worker whose aggregate the revision belongs to.
func v3RevisionOwnerExample() string {
	if worker, ok := edgeformcatalog.ByKind("ModuleWorker"); ok {
		return worker.FixtureName()
	}
	return "module-worker"
}

// v3ResourceDoc renders one family resource reference document.
func v3ResourceDoc(form model.Form) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, `---
page_title: "%s Resource - takoform"
subcategory: "Edge Platform Family"
description: |-
  %s
---

# %s

%s

`, form.ResourceType, form.Title+" ("+edgeformcatalog.Family.APIVersion()+", role "+string(form.Role)+").", form.ResourceType, form.Description)
	builder.WriteString(v3RoleSemantics(form.Role) + "\n\n")
	builder.WriteString("This Experimental Form speaks the Host API v1beta1 lane and requires provider v2.1.1 or\n" +
		"later. Provider v2.1.1 is the stable release target; its source descriptor stays\n" +
		"candidate-only until the owner publishes it. The configured host selects and\n" +
		"operates the concrete backend; no attribute names a vendor, target, credential,\n" +
		"price, or implementation. See the [complete example](../../examples/resources/" +
		form.ResourceType + "/resource.tf).\n")
	builder.WriteString("\n## Arguments\n\n")
	builder.WriteString(v3NameArgumentDoc(form))
	if form.Kind == "WorkerBundle" {
		builder.WriteString("- `manifest_digest` (String, optional, computed, forces replacement) — Immutable digest of the committed artifact manifest this bundle is. It is the whole portable desired state: the manifest, not this resource, describes the modules. Declare exactly one of the two authoring modes — reference a manifest already committed to the host by setting this digest, or leave it unset and author the bundle locally with the two arguments below. Writing it alongside local authoring is accepted only when the authored bytes commit exactly that manifest; a disagreement is refused before any host call.\n")
		builder.WriteString("- `main_module` (String, optional, forces replacement) — Local authoring only: relative path of the ES module the runtime instantiates first; it must name one declared module. It is not portable desired state; it describes the artifact manifest the provider commits.\n")
		builder.WriteString("- `modules` (List of Object, optional, forces replacement) — Local authoring only: every module of the bundle. Each entry declares `name`, `content_type` (one of the five closed media types), and `content_file` (a local file path). The provider reads each file, computes its exact `size` and sha256 `digest` (both computed attributes), commits the artifact manifest through the content-addressed artifact API, and records the returned `manifest_digest`. File paths stay in state; file bytes never do. At every plan against existing state the provider re-reads and re-hashes each `content_file`: changed bytes at an unchanged path change the planned manifest digest and force replacement.\n")
	} else {
		for _, field := range form.Fields {
			builder.WriteString(v3FieldDocLine(form, field))
		}
	}
	builder.WriteString("- `space` (String, optional, forces replacement) — Exact opaque SpaceID; overrides the provider default.\n")
	if form.DeclaresUpdate() {
		builder.WriteString("- `create_timeout` / `update_timeout` / `delete_timeout` (String, optional) — Go durations bounding each operation (defaults `20m` / `20m` / `30m`).\n")
	} else {
		builder.WriteString("- `create_timeout` / `delete_timeout` (String, optional) — Go durations bounding each operation (defaults `20m` / `30m`). There is no `update_timeout`: this Form declares no update capability. Changing only these provider-side timeouts is applied in place without any host call.\n")
	}
	generationDoc := "increments only when the portable desired spec changes. Updates fence on it."
	recoveryDoc := "an in-place re-apply of the same desired state, which is all a host needs to re-resolve and re-pin every reference"
	if !form.DeclaresUpdate() {
		generationDoc = "increments only when the portable desired spec changes; this Form declares no update capability — every desired attribute forces replacement instead."
		recoveryDoc = "replacing this resource, because this Form declares no in-place update and a host refuses every apply to the existing one"
	}
	builder.WriteString(`
## Read-only attributes

- ` + "`uid`" + ` — host-issued immutable resource identity; delete and re-create yields a new UID.
- ` + "`generation`" + ` — desired-state generation; ` + generationDoc + ` It is also the DELETE fence, because a delete withdraws desired state like any other desired-state mutation.
- ` + "`revision`" + ` — representation revision; increments whenever the representation changes — a spec-changing update, new status, new outputs, or a change to another resource this one is rendered from. It is the strong ETag, and it is deliberately NOT the delete fence: a teardown removes dependents first and would otherwise be refused by a revision it moved itself.
- ` + "`conditions`" + ` — the complete status condition list the host reports, in its order. Each entry carries
  ` + "`type`" + ` (the closed ` + "`Ready`" + ` / ` + "`Reconciling`" + ` / ` + "`Degraded`" + ` / ` + "`Drifted`" + ` / ` + "`Blocked`" + ` / ` + "`Deleting`" + ` vocabulary),
  ` + "`status`" + ` (` + "`True`" + ` / ` + "`False`" + ` / ` + "`Unknown`" + `), the closed portable ` + "`reason`" + `, an optional ` + "`message`" + `, an optional
  non-portable ` + "`host_reason`" + ` naming exactly what is wrong, the ` + "`observed_generation`" + ` the status reflects,
  and ` + "`last_transition_time`" + `. Conditions are host-rendered state: they change when this resource changes
  AND when a resource it depends on changes, with no desired spec changing anywhere, so they are read-only
  and a configuration must not assert them.
- ` + "`ready`" + ` — derived convenience: true when ` + "`conditions`" + ` carries the closed ` + "`Ready`" + ` condition with status
  ` + "`True`" + `. Read ` + "`conditions`" + ` for the reason it is not.
- ` + "`outputs_json`" + ` — the WHOLE ` + "`status.outputs`" + ` document, JSON-serialized. ` + v3OutputsJSONDoc(form) + `
- ` + "`form_api_version`" + `, ` + "`form_kind`" + `, ` + "`form_definition_version`" + `, ` + "`form_schema_digest`" + ` — the exact immutable FormRef this state is bound to; reads dispatch on it.
- ` + "`form_package_digest`" + ` — audit-only package provenance; never part of resource identity, queries, or fences.
- ` + "`relation_drift_reason`" + ` — internal recovery only: ` + "`ExternalChange`" + ` or ` + "`DependencyMissing`" + ` while the host reports that a resource this one references was replaced or removed out of band, null otherwise. A refresh reports the break as a warning and keeps the resource in state; the next plan then proposes ` + recoveryDoc + `. It is provider-side recovery bookkeeping — no portable wire member carries it — and configurations must not depend on it.
- ` + "`pending_operation_id`" + ` — internal recovery only: the host operation id of a mutation the host accepted but that did not reach a terminal state before the operation deadline, null in steady state. A refresh consults it before it reads the resource, and it is cleared only once that operation settles. It is not resource identity and configurations must not depend on it.
`)
	builder.WriteString(v3OutputAttributeSection(form))
	builder.WriteString(v3StateContinuitySection(form.Kind))
	if len(form.ProvidedInterfaces) > 0 {
		builder.WriteString("\n## Provided interfaces\n\n")
		for _, provided := range form.ProvidedInterfaces {
			fmt.Fprintf(&builder, "- `%s@%s` — %s\n", provided.Name, provided.Version, v3ProvidedInterfaceProse(provided.Name))
		}
	}
	if len(form.AcceptedBindings) > 0 {
		builder.WriteString("\n## Accepted bindings\n\n")
		for _, accepted := range form.AcceptedBindings {
			fmt.Fprintf(&builder, "- `%s@%s`\n", accepted.Name, accepted.Version)
		}
		builder.WriteString("\nOutward capability use is a typed binding held by this revision; inward\nactivation (routes, domains, cron, queue consumption) is a separate\nattachment resource. The two are never merged.\n")
	}
	builder.WriteString("\n## Import\n\n```console\nterraform import " + form.ResourceType + ".example NAME\nterraform import " + form.ResourceType + ".example SPACE/NAME\n```\n")
	builder.WriteString(v3ImportIdentitySection(form.ResourceType, form.Kind))
	if form.Kind == "WorkerBundle" {
		builder.WriteString("\nAn imported bundle restores `manifest_digest` from the host and leaves\n`main_module` and `modules` null: those are local authoring facts the wire\nnever echoes. The resource is fully manageable afterwards — a configuration\nthat states the same `manifest_digest` plans empty, and adopting the local\nfiles that commit exactly that manifest is not a change either, because the\nbundle's identity is the digest.\n")
	}
	return builder.String()
}

// v3OutputsJSONDoc explains what `outputs_json` holds for one Form.
//
// The two sentences differ because the attribute means two different things. On
// a Form with an output contract it is the unnarrowed document BESIDE the typed
// attributes, which is the promise that an existing configuration decoding it
// keeps working. On a Form with none it is the only way to see anything a host
// returns at all — and, per the wire rule, a conforming host returns nothing,
// so the honest documentation is that it is always `"{}"`.
func v3OutputsJSONDoc(form model.Form) string {
	if len(form.Outputs) == 0 {
		return "This Form declares no `outputSchema`, so a conforming host omits `status.outputs` entirely and this\n" +
			"  attribute is `\"{}\"`. It stays declared because a host may publish a value no contract describes, and\n" +
			"  an undescribed value must still be reachable rather than silently dropped."
	}
	return "It is not narrowed by the typed output attributes below: every value that has a typed\n" +
		"  attribute is still in this document under its wire name, so a configuration that reads\n" +
		"  `jsondecode(...)[\"…\"]` keeps working unchanged. What it is now FOR is the other case — reaching an\n" +
		"  output the Form's `outputSchema` does not describe."
}

// v3OutputAttributeSection documents the typed computed attributes derived from
// a Form's outputSchema, and says plainly what they are and are not.
//
// A Form that publishes no output gets no section at all rather than an empty
// one: `status.outputs` is omitted on the wire for such a Form, so a heading
// promising outputs would describe a document that does not exist.
func v3OutputAttributeSection(form model.Form) string {
	if len(form.Outputs) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("\n## Outputs\n\n")
	builder.WriteString("This Form declares an `outputSchema`, so a conforming host returns exactly these\n" +
		"values in `status.outputs` and the provider surfaces each one as a typed computed\n" +
		"attribute. Read `" + form.ResourceType + ".example.<name>` rather than decoding\n" +
		"`outputs_json`; the JSON document still carries every value under its wire name and\n" +
		"stays the way to reach an output no schema describes.\n\n")
	for _, output := range form.Outputs {
		fmt.Fprintf(&builder, "- `%s` (%s, computed) — %s\n",
			output.AttributeName(), v3DocType(output), output.Doc)
	}
	builder.WriteString("\nOutputs are never arguments: a configuration that sets one is rejected at validate\n" +
		"time. They are host-computed state, so they can change without any desired attribute\n" +
		"changing — a plan that touches this resource shows them as known-after-apply.\n")
	return builder.String()
}

// v3StateContinuitySection documents what a refresh does when the world moved
// underneath the recorded state: a Form line that advanced, a replaced
// incarnation, or a mutation the host accepted and has not finished. All three
// are decided in spec/decisions/0017, and all three share one principle — a
// refresh never silently changes which resource, or which contract, state names.
func v3StateContinuitySection(kind string) string {
	return `
## State continuity

- **Reads dispatch on the recorded FormRef.** ` + "`" + kind + "`" + ` state is addressed under the
  exact ` + "`form_*`" + ` identity it records, not under this build's default create ref, so a
  resource created before the Form line advanced stays addressable as itself. An identity
  this provider build carries no codec for is a hard error naming that identity and the
  ones the build does carry; the provider never substitutes another exact FormRef, because
  a substituted query's "not found" is indistinguishable from deletion.
- **A changed ` + "`uid`" + ` is an error, and state is kept.** When the host serves a different
  ` + "`uid`" + ` under the recorded name, the resource this state was applied against is gone and
  something re-used its name. The provider reports a hard error naming both uids and keeps
  the resource in state. It does not re-bind — that would adopt a resource you never
  applied — and it does not remove state, which would make the next apply fail against the
  resource that does exist, with no plan left to repair it. Resolve it by importing the new
  incarnation explicitly, restoring the prior one, or deleting the host-side replacement.
- **An unfinished mutation is resumed, not re-created.** When ` + "`pending_operation_id`" + ` is
  set, a refresh asks the host about that operation before it reads the resource. While the
  operation is still running the resource may legitimately not exist yet, so its absence is
  not treated as deletion and the marker survives; a terminal success is verified against
  the exact identity and settles state; a terminal failure or an expired operation record
  defers to an exact read of the resource, which decides. Refresh again once the host
  settles.
`
}

// v3ImportIdentitySection documents the exact-identity import form. Both short
// forms resolve to the default create FormRef, which is right for almost every
// import and wrong for exactly one case: a resource created under an EARLIER
// definition version of this Form. Seeding the default there would bind state to
// a contract the resource was never applied under, so the canonical JSON form
// exists to name the identity outright (spec/decisions/0017).
func v3ImportIdentitySection(resourceType, kind string) string {
	return "\nBoth short forms bind state to this provider build's default create\n" +
		"FormRef. To adopt a resource created under an EARLIER definition version of\n" +
		"this Form, name the exact identity instead. The import ID is then one JSON\n" +
		"object — not a delimiter-joined string, because a SpaceID is opaque UTF-8\n" +
		"whose only forbidden character is `/`, so no separator can escape it safely:\n" +
		"\n```console\nterraform import " + resourceType + ".example \\\n" +
		"  '{\"space\":\"prod\",\"apiVersion\":\"" + edgeformcatalog.Family.APIVersion() + "\",\"kind\":\"" + kind + "\",\"definitionVersion\":\"0.1.0\",\"schemaDigest\":\"sha256:…\",\"name\":\"…\"}'\n```\n" +
		"\n`space` is optional and falls back to the provider default; the four FormRef\n" +
		"members are all-or-nothing. An identity this provider build carries no codec\n" +
		"for is refused, naming the identities it does carry — it is never silently\n" +
		"rebound to the default.\n"
}

// v3ExampleHCL renders one family resource example configuration.
func v3ExampleHCL(form model.Form) string {
	var builder strings.Builder
	builder.WriteString(`terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # stable v2.1.1 release target; descriptor remains candidate-only until owner publication.
      version = "= 2.1.1"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

`)
	fmt.Fprintf(&builder, "resource %q \"example\" {\n", form.ResourceType)
	// A revision shows the shape an author should write: no `name`, and the
	// owner the derived name needs beside the content digest. Pinning a name
	// stays legitimate — an imported revision has one — but it is the exception.
	scalars := [][2]string{{"name", fmt.Sprintf("%q", form.FixtureName())}}
	if form.Role == model.RoleRevision {
		scalars = [][2]string{{"revision_owner", fmt.Sprintf("%q", v3RevisionOwnerExample())}}
	}
	var blocks []string
	if form.Kind == "WorkerBundle" {
		scalars = append(scalars, [2]string{"main_module", `"worker.mjs"`})
		blocks = append(blocks, `  modules = [
    {
      name         = "worker.mjs"
      content_type = "application/javascript+module"
      content_file = "${path.module}/dist/worker.mjs"
    },
  ]
`)
	} else {
		for _, field := range form.Fields {
			if field.Example == nil {
				continue
			}
			switch field.Kind {
			case model.KindBindingList:
				blocks = append(blocks, v3BindingBlockHCL(field))
			case model.KindObjectList:
				blocks = append(blocks, v3ObjectListHCL(field))
			case model.KindObject:
				blocks = append(blocks, v3ObjectHCL(field))
			case model.KindJSONMap:
				scalars = append(scalars, [2]string{field.HCL + "_json", v3JSONEncodeHCL(field.Example)})
			case model.KindResourceRef:
				ref, _ := field.Example.(map[string]any)
				name, _ := ref["name"].(string)
				scalars = append(scalars, [2]string{field.HCL, fmt.Sprintf("%q", name)})
			default:
				scalars = append(scalars, [2]string{field.HCL, quoteHCL(field.Example)})
			}
		}
	}
	width := 0
	for _, line := range scalars {
		if len(line[0]) > width {
			width = len(line[0])
		}
	}
	for _, line := range scalars {
		fmt.Fprintf(&builder, "  %-*s = %s\n", width, line[0], line[1])
	}
	for _, block := range blocks {
		builder.WriteString("\n" + block)
	}
	builder.WriteString("}\n\n")
	prefix := strings.TrimPrefix(form.ResourceType, "takoform_")
	// A Form that declares an outputSchema shows the typed attributes, because
	// that is what an author should write. outputs_json still holds the same
	// values and is what the example shows when a Form describes none.
	if len(form.Outputs) == 0 {
		fmt.Fprintf(&builder, "output %q {\n  value = %s.example.outputs_json\n}\n",
			prefix+"_outputs", form.ResourceType)
		return builder.String()
	}
	for _, output := range form.Outputs {
		fmt.Fprintf(&builder, "output %q {\n  value = %s.example.%s\n}\n\n",
			prefix+"_"+output.AttributeName(), form.ResourceType, output.AttributeName())
	}
	return strings.TrimRight(builder.String(), "\n") + "\n"
}

func v3BindingBlockHCL(field model.Field) string {
	entries, _ := field.Example.([]any)
	var builder strings.Builder
	fmt.Fprintf(&builder, "  %s = [\n", field.HCL)
	for _, raw := range entries {
		entry, _ := raw.(map[string]any)
		name, _ := entry["name"].(string)
		targetName := ""
		if ref, ok := entry["resource"].(map[string]any); ok {
			targetName, _ = ref["name"].(string)
		}
		fmt.Fprintf(&builder, "    {\n      name        = %q\n      target_name = %q\n    },\n", name, targetName)
	}
	builder.WriteString("  ]\n")
	return builder.String()
}

func v3ObjectListHCL(field model.Field) string {
	entries, _ := field.Example.([]any)
	var builder strings.Builder
	fmt.Fprintf(&builder, "  %s = [\n", field.HCL)
	for _, raw := range entries {
		entry, _ := raw.(map[string]any)
		builder.WriteString("    {\n")
		width := 0
		lines := make([][2]string, 0, len(field.Fields))
		for _, member := range field.Fields {
			value, present := entry[member.Wire]
			if !present {
				continue
			}
			var rendered string
			if member.Kind == model.KindResourceRef {
				ref, _ := value.(map[string]any)
				name, _ := ref["name"].(string)
				rendered = fmt.Sprintf("%q", name)
			} else {
				rendered = quoteHCL(value)
			}
			lines = append(lines, [2]string{member.HCL, rendered})
			if len(member.HCL) > width {
				width = len(member.HCL)
			}
		}
		for _, line := range lines {
			fmt.Fprintf(&builder, "      %-*s = %s\n", width, line[0], line[1])
		}
		builder.WriteString("    },\n")
	}
	builder.WriteString("  ]\n")
	return builder.String()
}

func v3ObjectHCL(field model.Field) string {
	entry, _ := field.Example.(map[string]any)
	lines := make([][2]string, 0, len(field.Fields))
	width := 0
	for _, member := range field.Fields {
		value, present := entry[member.Wire]
		if !present {
			continue
		}
		var rendered string
		if member.Kind == model.KindResourceRef {
			ref, _ := value.(map[string]any)
			name, _ := ref["name"].(string)
			rendered = fmt.Sprintf("%q", name)
		} else {
			rendered = quoteHCL(value)
		}
		lines = append(lines, [2]string{member.HCL, rendered})
		if len(member.HCL) > width {
			width = len(member.HCL)
		}
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "  %s = {\n", field.HCL)
	for _, line := range lines {
		fmt.Fprintf(&builder, "    %-*s = %s\n", width, line[0], line[1])
	}
	builder.WriteString("  }\n")
	return builder.String()
}

// v3JSONEncodeHCL renders a jsonencode(...) expression for one JSON-map
// example with deterministic key order.
func v3JSONEncodeHCL(example any) string {
	entries, _ := example.(map[string]any)
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%q = %s", key, quoteHCL(entries[key])))
	}
	return "jsonencode({ " + strings.Join(parts, ", ") + " })"
}

// v3FormInventorySection is appended to the generated forms/README.md: the
// Edge Platform Family members and their roles, rendered from the same
// catalog the candidates are built from.
func v3FormInventorySection() string {
	var builder strings.Builder
	builder.WriteString(`
## Edge Platform Family (` + edgeformcatalog.Family.APIVersion() + `)

The first official Form Family fixes the shape of a proven edge developer
platform without naming its vendor (spec/form-families.md). Its members are
Experimental Forms for the Host API v1beta1 resource lane; their package
artifacts remain unpublished. The typed resources require provider v2.1.1 or
later. Roles come from the closed v1beta1 role enum and decide
lifecycle mechanics: revisions are immutable, deployments move traffic,
attachments activate inward events.

| Kind | Resource | Role | Version | Portable intent |
| --- | --- | --- | --- | --- |
`)
	for _, form := range edgeformcatalog.Forms {
		fmt.Fprintf(&builder, "| `%s` | `%s` | `%s` | `%s` | %s |\n",
			form.Kind, form.ResourceType, form.Role, form.DefinitionVersion, form.Description)
	}
	builder.WriteString(`
The provider exposes exactly these typed resources on the v1beta1 lane, and no
generic carrier for a Form it was not built against: nothing in the lane lets a
client verify a FormRef it did not compile in, so a carrier would offer reach
with no verification behind it (spec/decisions/0021). Family membership grants
no Stable maturity: the generated family candidate set records all 15 as
Experimental 0.1.0 Forms, and hosts state their supported subset in their Host
Support Profiles. Beta is the API/family channel, not Form stability.
`)
	return builder.String()
}
