# Form Definition profiles

A Form Definition is a deterministic, data-only description of one portable
service shape. Requirement keywords are used as described in
[`../conformance.md`](../conformance.md).

The current authoring profile is
[`form-definition-v1.schema.json`](/schemas/v1/form-definition.schema.json)
with
[`form-ref-v1.schema.json`](/schemas/v1/form-ref.schema.json). The
Form Definition profile moves independently of the exact FormRef shape. Stable
v1 carries the closed authoring vocabulary used only by versionless Forms that
declare the stable `forms.takoform.com/v1` Host lane; it does not reinterpret
the occupied Beta Host lane or a retained Form package. The
[`form-definition-v1beta1.schema.json`](/schemas/v1beta1/form-definition.schema.json)
profile is retained unchanged and still validates the
`edge.forms.takoform.com/v1beta1` package bytes embedded by retained Provider
2.1.1. The unpublished
v1beta2 Form Definition schema remains as predecessor history, while
[`form-ref-v1beta1.schema.json`](/schemas/v1beta1/form-ref.schema.json)
remains the exact reference profile of those retained v1beta1 packages; no
published identity is rewritten. Across current and retained profiles:
its `apiVersion` is a namespaced Form Family group
([decision 0009](../decisions/0009-form-families-and-namespaced-api-versions.md)),
it declares a required closed `role`
(identity/revision/deployment/attachment/policy), it references exact
Interface and Binding contracts (`providedInterfaces`, `acceptedBindings`),
its desired schema never contains a `name` property (the wire envelope owns
`metadata.name` — [decision 0011](../decisions/0011-resource-identity-generation-and-revision.md)),
and free semantic tokens are prohibited
([decision 0008](../decisions/0008-forms-preserve-service-shape.md)).

The retained `forms.takoform.com/v1alpha2` Draft 2020-12 schemas are
[`form-definition-v1alpha2.schema.json`](../../formpackage/schemas/form-definition-v1alpha2.schema.json)
and
[`form-ref-v1alpha2.schema.json`](../../formpackage/schemas/form-ref-v1alpha2.schema.json).
The unversioned filenames
[`form-definition.schema.json`](../../formpackage/schemas/form-definition.schema.json)
and [`form-ref.schema.json`](../../formpackage/schemas/form-ref.schema.json)
are retained v1alpha1 Legacy profiles, not aliases for the retained v1alpha2
or current family profiles.

## Exact FormRef

The immutable reference to a definition is exactly these four fields, with no
extensions:

```json
{
  "apiVersion": "function.forms.takoform.com",
  "kind": "Function",
  "definitionVersion": "0.1.0",
  "schemaDigest": "sha256:<64 lowercase hexadecimal characters>"
}
```

`kind` MUST be a PascalCase portable kind, `definitionVersion` MUST be SemVer,
and `schemaDigest` MUST be SHA-256 over the definition's RFC 8785 canonical
bytes. The definition MUST repeat the first three identity fields, and a
verifier MUST reject any mismatch. A current group is versionless: the exact
identity is `(group, kind, definitionVersion, schemaDigest)`. Resolution never
substitutes a family latest, a sibling kind, or a Definition with a different
version or digest.

## Definition fields

A definition contains:

- the three non-digest identity fields;
- a title and optional description;
- inline Draft 2020-12 desired and observed schemas, plus an optional output schema;
- optional immutable JSON Pointer fields;
- an explicit subset of `create`, `read`, `update`, `delete`, `import`, and
  `observe` — the current capability vocabulary. `refresh` and
  `drift` belong to the withdrawn lanes. The current profile
  (`v1/form-definition.schema.json`) admits neither; its predecessor
  `v1beta1` still admits `refresh` and is retained unchanged for the packages
  published against it, because narrowing a served identity would change what
  it meant when it was published;
  the open `(name, version)` Interface descriptors of the withdrawn v1alpha2
  profile are replaced by exact digest-bound `providedInterfaces`
  ([`../interface-contract/`](../interface-contract/index.md));
- optional references to data-only positive desired/observed/output fixtures
  and negative schema fixtures in the same package, with independent maxima of
  32 positive fixtures and 32 negative fixtures.

The current family structure deliberately has no maturity or deprecation
field. Proposal, Experimental, Stable, and Legacy state is owned outside the
immutable definition as defined in
[`../project-lifecycle.md`](../project-lifecycle.md). This keeps one mutable
authority for a fact that can change while immutable Definition bytes cannot.

Published v1alpha1 Definitions retain their historical document-local
`compatibility-candidate`, `standard`, or `deprecated` field. A verifier MUST
preserve those bytes, but current tooling MUST NOT infer maturity from them or
copy the field into a current family definition.

Negative fixtures name their validation stage. Host Support evidence executes
`desired` negatives in the host role; provider compatibility evidence executes
`desired` and `observed` negatives before state projection. `output` remains a
valid package-structure validation stage, but a conformance claim fails closed
on it until a portable runner contract defines how its role executes that case.

The published Form Definition JSON Schema is a normative structural minimum.
The semantic verifier rules below are also normative: document-local reference
closure, closed-object proof, validation-work limits, fixture semantics, and
the portable data-only vocabulary can reject a document that satisfies the
outer JSON Schema.

## Closed recursive values and defaults

The stable-v1 authoring model adds only the concrete data shapes current Forms
need; it does not add a generic JSON, bytes, message, or graph-expression type.

- An ordered string list preserves item order and duplicate values. It is not
  interchangeable with a string set.
- A string map and string-set map both declare `maxProperties`, use the
  portable map-key grammar below, and bound their string values. Every set
  value also declares `maxItems` and uses `uniqueItems: true`.
- RFC 8785 canonical encoding supplies deterministic object-key order. Defaults
  and conformance examples additionally sort each string-set-map value
  lexically; ordered string lists are never sorted or deduplicated.
- Closed objects and object lists recurse through the same vocabulary. Defaults
  materialize inside a present object and every present list element. An
  optional object whose absence is itself semantic remains absent; nested
  defaults do not synthesize that parent.
- A tagged object is a closed `oneOf`. Every branch requires the same string
  discriminator with one branch-specific `const`, forbids unknown members, and
  contains all and only that variant's fields. Reference traversal follows only
  the selected valid branch.

## Structural and resolved-UID constraints

The stable-v1 `constraints` list adds exactly two desired-structure variants and
four resolved-UID variants. The structural variants compare already validated
desired values without coercion:

```json
{"kind":"orderedPair","references":["/minInstances","/maxInstances"]}
{"kind":"uniqueBy","list":"/secondaryIndexes","member":"name"}
```

`orderedPair` carries exactly two distinct non-wildcard pointers, both naming
required numeric properties, and requires the first value to be less than or
equal to the second. `uniqueBy` names an object-list property plus one direct,
required scalar member of every item; no two items in one list may carry the
same JSON scalar value. Equality is typed JSON equality, so implementations do
not stringify or otherwise coerce values. The list itself may be optional when
its schema gives omission a portable meaning.

The four resolved-UID variants compare host-resolved immutable resource UIDs
only: a reused name with a replacement UID is a different target, and group,
kind, name, or desired bytes are never a substitute for resolution.

```json
{"kind":"acyclic","reference":"/deadLetter/queue"}
{"kind":"distinctPair","references":["/target","/deadLetter"]}
{"kind":"uniquePair","references":["/topic","/target"]}
{"kind":"sameResolvedTarget","anchor":"/function","members":"/versions/*/functionVersion","through":"/function"}
```

`acyclic` rejects an edge that would close a cycle through the declared
relation. `distinctPair` requires its two local relations to resolve to
different UIDs. `uniquePair` allows at most one live resource of the same exact
Form to hold the ordered pair of resolved UIDs. `sameResolvedTarget` resolves
the local `anchor`, resolves every local relation selected by `members`, then
follows `through` on each member target; every resulting UID MUST equal the
anchor UID.

The vocabulary is closed. `orderedPair.references`, `uniqueBy.list`,
`acyclic.reference`, both resolved-pair members, and
`sameResolvedTarget.anchor` carry no array wildcard. `members` carries exactly
one `*` array token. `through` is an RFC 6901 pointer in the resolved member
target's desired document, not a pointer in the declaring Form. A Definition
with another kind, a foreign member, the wrong pointer cardinality, or a local
pointer that is not a declared relation is invalid. The comparison and
uniqueness scope are fixed by these variants; there is no universal graph DSL.

`requiresHostApi` is checked against the mechanisms a Form actually declares,
not against its kind or family. Resolved-UID constraints, tagged-branch relation
traversal, sealed external-service slots, and a new Form-neutral required
entrypoint require `forms.takoform.com/v1`. Ordinary JSON Schema
validation does not raise the bound by itself. Retained v1beta1 declarations
keep their published lower bound and meaning; this profile does not reinterpret
or re-identify them.

## Connections were the withdrawn profile's surface

The `connections` map — request-only permission/projection tokens naming
another Resource — was the withdrawn v1alpha2 profile's capability surface.
The current family replaces it end to end with exact digest-bound typed
Bindings ([`../binding-contract/`](../binding-contract/index.md)) and
uid-pinned relations
([decision 0015](../decisions/0015-cross-resource-references-are-uid-pinned-relations.md),
[decision 0022](../decisions/0022-relations-pin-the-target-contract.md));
no current Definition declares `connections`, and the withdrawn semantics
stay in git history with their profile.

## Digest-bound artifact sources

An artifact-backed Form of the current family declares its bytes as ONE
member: the content-addressed manifest digest committed through the
artifact-transport upload API
([decision 0012](../decisions/0012-artifacts-use-content-addressed-upload.md),
[`../artifact-transport/`](../artifact-transport/index.md)). No URL enters
desired state — where bytes are fetched from is a host fact, and the digest
alone says which bytes. The withdrawn v1alpha2 profile's
`artifactUrl`/`artifactSha256`/`artifactMediaType` source object and its
credential-free-HTTPS grammar stay in git history with that profile.

Object schemas are closed by default and MUST set
`"additionalProperties": false`. A pure typed map is the only open-key
escape. It must explicitly use `"type": "object"`, must not mix fixed or
dependent properties, must reject `patternProperties`, and must use a schema
for `additionalProperties` plus this exact key policy:

```json
{
  "propertyNames": {
    "type": "string",
    "pattern": "^[A-Za-z][A-Za-z0-9._-]{0,63}$",
    "x-takoform-fieldPolicy": "portable-data-only-v1"
  }
}
```

The marker is a host conformance requirement, not an annotation to ignore:
map keys are checked with the same portable data-only forbidden-field policy
as declared fields. `additionalProperties: true`, an omitted
`additionalProperties`, a permissive or unmarked `propertyNames`, and
`patternProperties` are rejected.

This rule applies at every nested schema node, not only at the desired or
observed root. Boolean `false` is safe because it accepts no value; boolean
`true`, `{}`, an implicit schema such as `{"not":{"type":"string"}}`, and
object keywords such as `minProperties` without an explicit closed
`type: object` are rejected because each can admit arbitrary objects.
`allOf`/`anyOf`/`oneOf` and local `$ref` remain usable only when every relevant
branch or resolved target proves that objects are excluded or closed. A
non-object `type` is the normal proof for primitive and array schemas.
Arrays must additionally declare `items` (a safe schema or `false`) so omitted
item constraints cannot reintroduce arbitrary nested objects; tuple
`prefixItems` do not remove that requirement for trailing items.

## Hard boundary

Definitions and every JSON payload are recursively checked for credential,
secret, token, account, operator, target/pool, capacity, backend manager,
provider config, price, SKU, billing, quota, SLA/support policy, executable,
command, script, source/adapter/validation/runtime code, WebAssembly, and plugin
fields. This policy is intentionally fail-closed. A host-owned implementation,
placement, commercial configuration, or executable extension is not portable
Form Definition data.

The check is structural: normalized exact names, exact camelCase, snake_case,
or kebab-case tokens, and reviewed token sequences such as `api` + `key`,
`private` + `key`, `service` + `offering`, and `manager` + `identifier` are
compared with a forbidden vocabulary. Glued lowercase spellings are limited by
exact reviewed compound-base and qualifier pairs such as `apikey` +
`material`; the policy does not use arbitrary substring matching.
Standard schema keys such as `description`, and prose values that discuss
authentication, API keys, service offerings, or billing, remain valid; fields
such as `authorization`, `oauthClient`, `sessionCookie`, `apiKeyValue`,
`privateKeyPem`, `invoice`, `paymentMethod`, `currency`, `taxCode`,
`serviceOfferingId`, `managerIdentifier`, and `region` do not.

Three desired-property spellings have shape-dependent exceptions; their names
alone never bypass the boundary. `command` is permitted only as a bounded
ordered list whose string items have both a non-empty pattern and an explicit
maximum length. `concurrencyTarget` is permitted only as an integer with
explicit minimum and maximum bounds. `target` is permitted only when its child
schema proves one portable relation contract:

- a closed object with exactly `apiVersion`, `kind`, and `name`, a bounded name
  grammar, and exactly one of `x-takoform-target-formrefs` or
  `x-takoform-required-interface`; or
- a closed, discriminator-selected `oneOf` with two through sixteen distinct
  branches, where every branch contains such an annotated relation object.

Exact-Form annotations contain only complete group, kind, definition-version,
and canonical SHA-256 identities. Required-Interface annotations contain only
complete API version, name, version, and canonical SHA-256 identity. A bare
string, an open object, an unannotated or ambiguously annotated reference, a
tagged branch without a reviewed relation, and lookalike names such as
`backendTarget` remain forbidden. These are schema proofs consumed equally by
every implementation; they do not grant a provider or host authority to widen
Form semantics.

Every package is an exact one-definition package, never one multi-definition
package. The current generated catalog and package set predate decision
[`0004`](../decisions/0004-takoform-is-an-experimental-specification.md): their
`status: standard`, `structural-candidate`, and `portable-standard` values are
retained Legacy document facts, not a current maturity hierarchy. Host Support
and provider compatibility evidence are supplied by their owning
implementations and are not synthesized by the package generator.
