# Form Definition profiles

A Form Definition is a deterministic, data-only description of one portable
service shape. Requirement keywords are used as described in
[`../conformance.md`](../conformance.md).

The current family profile is
[`form-definition-v1beta2.schema.json`](../schemas/form-definition-v1beta2.schema.json)
with
[`form-ref-v1beta1.schema.json`](../schemas/form-ref-v1beta1.schema.json) —
the FormRef schema does not move with it, because it admits any namespaced
group by design and had nothing to correct. Its predecessor
[`form-definition-v1beta1.schema.json`](../schemas/form-definition-v1beta1.schema.json)
is retained unchanged and still validates the `edge.forms.takoform.com/v1beta1`
packages published under it. In both profiles:
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
  "apiVersion": "edge.forms.takoform.com/v1beta2",
  "kind": "ExampleStore",
  "definitionVersion": "0.1.0",
  "schemaDigest": "sha256:<64 lowercase hexadecimal characters>"
}
```

`kind` MUST be a PascalCase portable kind, `definitionVersion` MUST be SemVer,
and `schemaDigest` MUST be SHA-256 over the definition's RFC 8785 canonical
bytes. The definition MUST repeat the first three identity fields, and a
verifier MUST reject any mismatch.

## Definition fields

A definition contains:

- the three non-digest identity fields;
- a title and optional description;
- inline Draft 2020-12 desired and observed schemas, plus an optional output schema;
- optional immutable JSON Pointer fields;
- an explicit subset of `create`, `read`, `update`, `delete`, `import`, and
  `observe` — the current family's whole capability vocabulary. `refresh` and
  `drift` belong to the withdrawn lanes. The CURRENT profile
  (`v1beta2/form-definition.schema.json`) admits neither; its predecessor
  `v1beta1` still admits `refresh` and is retained unchanged for the packages
  published against it, because narrowing a served identity would change what
  it meant when it was published;
  the open `(name, version)` Interface descriptors of the withdrawn v1alpha2
  profile are replaced by exact digest-bound `providedInterfaces`
  ([`../interface-contract/`](../interface-contract/README.md));
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

## Connections were the withdrawn profile's surface

The `connections` map — request-only permission/projection tokens naming
another Resource — was the withdrawn v1alpha2 profile's capability surface.
The current family replaces it end to end with exact digest-bound typed
Bindings ([`../binding-contract/`](../binding-contract/README.md)) and
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
[`../artifact-transport/`](../artifact-transport/README.md)). No URL enters
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

Every package is an exact one-definition package, never one multi-definition
package. The current generated catalog and package set predate decision
[`0004`](../decisions/0004-takoform-is-an-experimental-specification.md): their
`status: standard`, `structural-candidate`, and `portable-standard` values are
retained Legacy document facts, not a current maturity hierarchy. Host Support
and provider compatibility evidence are supplied by their owning
implementations and are not synthesized by the package generator.
