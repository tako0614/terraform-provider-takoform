package standardforms

// publish_surfaces_v3.go renders the human-facing surfaces of the Host API
// v1alpha3 resource lane — one reference document and one example per Edge
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

// v3GenericResourceType is the generic v1alpha3 carrier. It is not a catalog
// Form: it has no FormRef of its own and carries any third-party Form by
// exact reference, so its surfaces are authored here instead of derived from
// a Form.
const v3GenericResourceType = "takoform_resource"

// v3PublishedSurfaces renders every family-lane doc and example, plus the two
// hand-authored surfaces of the generic takoform_resource carrier.
func v3PublishedSurfaces() []publishedSurface {
	surfaces := make([]publishedSurface, 0, len(edgeformcatalog.Forms)*2+2)
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
	surfaces = append(surfaces,
		publishedSurface{
			path: filepath.ToSlash(filepath.Join("docs", "resources",
				strings.TrimPrefix(v3GenericResourceType, "takoform_")+".md")),
			content: []byte(v3GenericResourceDoc()),
		},
		publishedSurface{
			path:    filepath.ToSlash(filepath.Join("examples", "resources", v3GenericResourceType, "resource.tf")),
			content: []byte(v3GenericExampleHCL()),
		},
	)
	return surfaces
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
		// The enforced grammar is the exact PatternCron subset; a loose
		// "cron expression" claim would promise syntax the provider rejects.
		doc = "UTC cron schedule in the enforced closed grammar: exactly five " +
			"single-value fields separated by single spaces — minute `0`-`59` and " +
			"hour `0`-`23` are literal numbers (`*` is not accepted there), " +
			"day-of-month is `*` or `1`-`31`, month is `*` or `1`-`12`, and " +
			"day-of-week is `*` or `0`-`6`. Ranges, lists, steps (such as `*/5`), " +
			"and names are not accepted."
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
	}
	return fmt.Sprintf("- `%s` (%s, %s) — %s%s%s\n",
		name, docType, v3DocRequirement(form, field), doc, v3DocConstraint(field), v3DocDefault(field))
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
	builder.WriteString("This resource speaks the Host API v1alpha3 lane and requires provider v2.1.0 or\n" +
		"later (source candidate; not yet published). The configured host selects and\n" +
		"operates the concrete backend; no attribute names a vendor, target, credential,\n" +
		"price, or implementation. See the [complete example](../../examples/resources/" +
		form.ResourceType + "/resource.tf).\n")
	builder.WriteString("\n## Arguments\n\n")
	builder.WriteString("- `name` (String, required, forces replacement) — Portable resource name (`metadata.name`).\n")
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
- ` + "`generation`" + ` — desired-state generation; ` + generationDoc + `
- ` + "`revision`" + ` — representation revision; increments whenever the representation changes — a spec-changing update, new status, or new outputs. Deletes fence on it via ` + "`If-Match`" + `.
- ` + "`conditions`" + ` — the complete status condition list the host reports, in its order. Each entry carries
  ` + "`type`" + ` (the closed ` + "`Ready`" + ` / ` + "`Reconciling`" + ` / ` + "`Degraded`" + ` / ` + "`Drifted`" + ` / ` + "`Blocked`" + ` / ` + "`Deleting`" + ` vocabulary),
  ` + "`status`" + ` (` + "`True`" + ` / ` + "`False`" + ` / ` + "`Unknown`" + `), the closed portable ` + "`reason`" + `, an optional ` + "`message`" + `, an optional
  non-portable ` + "`host_reason`" + ` naming exactly what is wrong, the ` + "`observed_generation`" + ` the status reflects,
  and ` + "`last_transition_time`" + `. Conditions are host-rendered state: they change when this resource changes
  AND when a resource it depends on changes, with no desired spec changing anywhere, so they are read-only
  and a configuration must not assert them.
- ` + "`ready`" + ` — derived convenience: true when ` + "`conditions`" + ` carries the closed ` + "`Ready`" + ` condition with status
  ` + "`True`" + `. Read ` + "`conditions`" + ` for the reason it is not.
- ` + "`outputs_json`" + ` — JSON-serialized ` + "`status.outputs`" + ` document (` + "`\"{}\"`" + ` when empty).
- ` + "`form_api_version`" + `, ` + "`form_kind`" + `, ` + "`form_definition_version`" + `, ` + "`form_schema_digest`" + ` — the exact immutable FormRef this state is bound to; reads dispatch on it.
- ` + "`form_package_digest`" + ` — audit-only package provenance; never part of resource identity, queries, or fences.
- ` + "`relation_drift_reason`" + ` — internal recovery only: ` + "`ExternalChange`" + ` or ` + "`DependencyMissing`" + ` while the host reports that a resource this one references was replaced or removed out of band, null otherwise. A refresh reports the break as a warning and keeps the resource in state; the next plan then proposes ` + recoveryDoc + `. It is provider-side recovery bookkeeping — no portable wire member carries it — and configurations must not depend on it.
- ` + "`pending_operation_id`" + ` — internal recovery only: the host operation id of a mutation the host accepted but that did not reach a terminal state before the operation deadline, null in steady state. A refresh consults it before it reads the resource, and it is cleared only once that operation settles. It is not resource identity and configurations must not depend on it.
`)
	builder.WriteString(v3StateContinuitySection(form.Kind))
	if len(form.ProvidedInterfaces) > 0 {
		builder.WriteString("\n## Provided interfaces\n\n")
		for _, provided := range form.ProvidedInterfaces {
			fmt.Fprintf(&builder, "- `%s@%s` — the exact Interface contract this Form's service exposes.\n", provided.Name, provided.Version)
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
      # provider v2.1.0 is an unpublished source candidate; build the provider from source.
      version = "= 2.1.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

`)
	fmt.Fprintf(&builder, "resource %q \"example\" {\n", form.ResourceType)
	scalars := [][2]string{{"name", fmt.Sprintf("%q", form.FixtureName())}}
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
	fmt.Fprintf(&builder, "output %q {\n  value = %s.example.outputs_json\n}\n",
		strings.TrimPrefix(form.ResourceType, "takoform_")+"_outputs", form.ResourceType)
	return builder.String()
}

// v3GenericResourceDoc renders the reference document for the generic
// takoform_resource carrier. It is authored rather than derived because the
// carrier has no Form Definition: it names an exact third-party FormRef and
// one JSON desired spec. Every statement below mirrors
// internal/provider/v3_generic_resource.go.
func v3GenericResourceDoc() string {
	return "---\n" +
		"page_title: \"takoform_resource Resource - takoform\"\n" +
		"subcategory: \"Host API v1alpha3\"\n" +
		"description: |-\n" +
		"  Generic Host API v1alpha3 carrier for any Form, including third-party Forms, addressed by exact FormRef.\n" +
		"---\n" +
		"\n" +
		"# takoform_resource\n" +
		"\n" +
		"Carries any Host API v1alpha3 Form by exact FormRef, including Forms published\n" +
		"by third parties in their own API groups. It is not a Form and has no FormRef\n" +
		"of its own: the configuration names the exact reference plus one JSON desired\n" +
		"spec, so a Form this provider was never built against is still usable.\n" +
		"\n" +
		"This resource speaks the Host API v1alpha3 lane and requires provider v2.1.0 or\n" +
		"later (source candidate; not yet published). The configured host selects and\n" +
		"operates the concrete backend; no attribute names a vendor, target, credential,\n" +
		"price, or implementation. See the [complete example](../../examples/resources/takoform_resource/resource.tf).\n" +
		"\n" +
		"Prefer a typed resource where one exists: the Edge Platform Family resources\n" +
		"carry per-field validation, a typed plan, and short import IDs. This carrier\n" +
		"trades all three for reach — it imports too, but only through the exact\n" +
		"identity it cannot infer.\n" +
		"\n" +
		"## Arguments\n" +
		"\n" +
		"The four `form_*` attributes are the exact FormRef. They, `name`, and `space`\n" +
		"all force replacement, so existing state is never rebound to another identity.\n" +
		"\n" +
		"- `form_api_version` (String, required, forces replacement) — Exact namespaced Form group and version, for example `forms.example.com/v1alpha1`. The frozen `forms.takoform.com/v1alpha1` and `forms.takoform.com/v1alpha2` groups are rejected; retained-epoch Forms belong to the provider-v2 typed resources.\n" +
		"- `form_kind` (String, required, forces replacement) — Exact Form kind, matching `^[A-Z][A-Za-z0-9]{0,63}$`.\n" +
		"- `form_definition_version` (String, required, forces replacement) — Exact immutable Form definition version, a semantic version.\n" +
		"- `form_schema_digest` (String, required, forces replacement) — Exact immutable Form schema digest, `sha256:` followed by 64 lowercase hexadecimal characters.\n" +
		"- `name` (String, required, forces replacement) — Portable resource name (`metadata.name`), matching `^[a-z][a-z0-9-]{0,62}$`.\n" +
		"- `space` (String, optional, forces replacement) — Exact opaque SpaceID; overrides the provider default.\n" +
		"- `spec_json` (String, optional) — The portable desired spec as one JSON object string (for example `jsonencode({...})`); the provider sends the parsed object as `spec`. Omitting it means the empty spec `{}`.\n" +
		"- `create_timeout` / `update_timeout` / `delete_timeout` (String, optional) — Go durations bounding each operation (defaults `20m` / `20m` / `30m`).\n" +
		"\n" +
		"## Read-only attributes\n" +
		"\n" +
		"- `uid` — host-issued immutable resource identity; delete and re-create yields a new UID.\n" +
		"- `generation` — desired-state generation; increments only when the portable desired spec changes. Updates fence on it together with `uid`.\n" +
		"- `revision` — representation revision; increments whenever the representation changes — a spec-changing update, new status, or new outputs. Deletes fence on it via `If-Match`.\n" +
		"- `conditions` — the complete status condition list the host reports, in its order. Each entry carries\n" +
		"  `type` (the closed `Ready` / `Reconciling` / `Degraded` / `Drifted` / `Blocked` / `Deleting` vocabulary),\n" +
		"  `status` (`True` / `False` / `Unknown`), the closed portable `reason`, an optional `message`, an optional\n" +
		"  non-portable `host_reason` naming exactly what is wrong, the `observed_generation` the status reflects,\n" +
		"  and `last_transition_time`. Conditions are host-rendered state: they change when this resource changes\n" +
		"  AND when a resource it depends on changes, with no desired spec changing anywhere, so they are read-only\n" +
		"  and a configuration must not assert them.\n" +
		"- `ready` — derived convenience: true when `conditions` carries the closed `Ready` condition with status\n" +
		"  `True`. Read `conditions` for the reason it is not.\n" +
		"- `outputs_json` — JSON-serialized `status.outputs` document (`\"{}\"` when empty).\n" +
		"- `relation_drift_reason` — internal recovery only: `ExternalChange` or `DependencyMissing` while the host reports that a resource this one references was replaced or removed out of band, null otherwise. A refresh reports the break as a warning and keeps the resource in state; the next plan then proposes an in-place re-apply, which is all a host needs to re-resolve and re-pin every reference. A Form whose Definition omits `update` refuses that apply, naming the missing capability; replace the resource instead. It is provider-side recovery bookkeeping — no portable wire member carries it — and configurations must not depend on it.\n" +
		"- `pending_operation_id` — internal recovery only: the host operation id of a mutation the host accepted but that did not reach a terminal state before the operation deadline, null in steady state. A refresh consults it before it reads the resource, and it is cleared only once that operation settles. It is not resource identity and configurations must not depend on it.\n" +
		"\n" +
		"State records the four FormRef fields and no package digest: the distribution\n" +
		"a host installed is audit evidence, never resource identity.\n" +
		"\n" +
		"## No local schema compilation\n" +
		"\n" +
		"The provider neither fetches nor compiles the Form's desired schema:\n" +
		"\n" +
		"- at plan time it checks only that the four FormRef fields are wholly known and satisfy the FormRef grammar and that `spec_json` parses as one JSON object; it reads no Form Definition and makes no host call;\n" +
		"- before each apply the client requires `GET {api}/forms` to return that exact FormRef as installed, executable, activated, available to the principal, and supporting the requested operation, then calls `prepare` and `apply`;\n" +
		"- the host validates `spec` against that exact Form's `desiredSchema`, so a spec that violates the Form surfaces as a host `invalid_argument` error during apply rather than as a plan-time diagnostic.\n" +
		"\n" +
		"Reads compare `spec_json` semantically rather than textually: your formatting\n" +
		"survives while the parsed document still equals the host's `spec`, and a real\n" +
		"out-of-band change adopts the host's canonical serialization so the next plan\n" +
		"shows the drift.\n" +
		"\n" +
		"## State continuity\n" +
		"\n" +
		"- **A changed `uid` is an error, and state is kept.** When the host serves a different\n" +
		"  `uid` under the recorded name, the resource this state was applied against is gone and\n" +
		"  something re-used its name. The provider reports a hard error naming both uids and keeps\n" +
		"  the resource in state. It does not re-bind — that would adopt a resource you never\n" +
		"  applied — and it does not remove state, which would make the next apply fail against the\n" +
		"  resource that does exist, with no plan left to repair it. Resolve it by importing the new\n" +
		"  incarnation explicitly, restoring the prior one, or deleting the host-side replacement.\n" +
		"- **An unfinished mutation is resumed, not re-created.** When `pending_operation_id` is\n" +
		"  set, a refresh asks the host about that operation before it reads the resource. While the\n" +
		"  operation is still running the resource may legitimately not exist yet, so its absence is\n" +
		"  not treated as deletion and the marker survives; a terminal success is verified against\n" +
		"  the exact identity and settles state; a terminal failure or an expired operation record\n" +
		"  defers to an exact read of the resource, which decides.\n" +
		"\n" +
		"## Write the Form's defaults explicitly\n" +
		"\n" +
		"A host materializes the portable defaults a Form declares before it validates,\n" +
		"digests, or stores your spec, so the `spec` it serves back can carry properties\n" +
		"your `spec_json` omitted. Because this carrier reads no Form Definition, it\n" +
		"cannot fill those defaults into the plan, and a read that adopts the host's\n" +
		"materialized document leaves a difference the next plan proposes again — each\n" +
		"apply is a host no-op that never advances `generation`, and the difference\n" +
		"returns. Write every defaulted property explicitly in `spec_json`, or use the\n" +
		"typed resource for that Form where one exists: typed resources carry each\n" +
		"declared default in the schema, so an omitted attribute plans as the value the\n" +
		"host will materialize and the second plan is empty.\n" +
		"\n" +
		"## Import\n" +
		"\n" +
		"The import ID is one JSON object naming the exact identity. It is not a\n" +
		"delimiter-joined string, because a SpaceID is opaque UTF-8 whose only forbidden\n" +
		"character is `/`, so no separator can escape it safely:\n" +
		"\n" +
		"```console\n" +
		"terraform import takoform_resource.example \\\n" +
		"  '{\"space\":\"prod\",\"apiVersion\":\"forms.example.com/v1alpha1\",\"kind\":\"ExampleWidget\",\"definitionVersion\":\"1.0.0\",\"schemaDigest\":\"sha256:…\",\"name\":\"example-widget\"}'\n" +
		"```\n" +
		"\n" +
		"`space` is optional and falls back to the provider default; every other member\n" +
		"is required. The `NAME` and `SPACE/NAME` short forms the typed family resources\n" +
		"accept are refused here: this carrier has no default create ref to resolve them\n" +
		"against, and guessing one would bind state to a Form the resource may not be.\n" +
		"\n" +
		"Import writes exactly the identity you state and nothing else; the refresh that\n" +
		"follows is what verifies it against the host, and `spec_json` is adopted from\n" +
		"the host's materialized spec there. Nothing about the carrier's trust model\n" +
		"changes: no Form Definition is fetched and no schema is compiled.\n"
}

// v3GenericExampleHCL renders the generic carrier's example. The exact
// FormRef values are illustrative third-party placeholders: no such Form is
// published, and the provider resolves nothing from a catalog.
func v3GenericExampleHCL() string {
	return `terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # provider v2.1.0 is an unpublished source candidate; build the provider from source.
      version = "= 2.1.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

# The four form_* values come from the Form's own publisher. They are an exact
# identity, not a lookup key: the provider validates their grammar and the host
# validates the spec against that exact Form.
resource "takoform_resource" "example" {
  form_api_version        = "forms.example.com/v1alpha1"
  form_kind               = "ExampleWidget"
  form_definition_version = "1.0.0"
  form_schema_digest      = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
  name                    = "example-widget"

  spec_json = jsonencode({ size = "small" })
}

output "resource_outputs" {
  value = takoform_resource.example.outputs_json
}
`
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
source candidates for the Host API v1alpha3 resource lane; the typed
resources require provider v2.1.0 or later (source candidate; not yet
published). Roles come from the closed v1alpha3 role enum and decide
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
A generic ` + "`takoform_resource`" + ` carries any third-party v1alpha3 Form by exact
FormRef. Family membership grants no maturity: these members are tracked in
the family candidate set, a lifecycle record begins only at an Experimental
transition, and hosts state their supported subset in their Host Support
Profiles.
`)
	return builder.String()
}
