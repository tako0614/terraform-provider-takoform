# Form Definition v1alpha1

A Form Definition is a deterministic, data-only description of one portable
service shape. Its normative Draft 2020-12 schema is
[`../../formpackage/schemas/form-definition.schema.json`](../../formpackage/schemas/form-definition.schema.json).

## Exact FormRef

The immutable reference to a definition is exactly these four fields, with no
extensions:

```json
{
  "apiVersion": "forms.takoform.com/v1alpha1",
  "kind": "ExampleStore",
  "definitionVersion": "1.0.0",
  "schemaDigest": "sha256:<64 lowercase hexadecimal characters>"
}
```

The normative schema is
[`../../formpackage/schemas/form-ref.schema.json`](../../formpackage/schemas/form-ref.schema.json).
`kind` is a PascalCase portable kind, `definitionVersion` is SemVer, and
`schemaDigest` is SHA-256 over the definition's RFC 8785 canonical bytes. The
definition repeats the first three identity fields; a verifier rejects any
mismatch.

## Definition fields

A definition contains:

- the three non-digest identity fields;
- a title, optional description, and explicit `compatibility-candidate`,
  `standard`, or `deprecated` status;
- inline Draft 2020-12 desired and observed schemas, plus an optional output schema;
- optional immutable JSON Pointer fields;
- an explicit subset of `create`, `read`, `update`, `delete`, `import`,
  `observe`, `refresh`, and `drift`;
- optional portable Interface descriptors with exact `(name, version)`, an
  exact non-secret document/schema, `required` readiness metadata, and
  deterministic literal/output input mappings plus an optional host-resolved
  canonical OAuth `resource_uri` audience input;
- optional references to data-only positive desired/observed/output fixtures
  and negative schema fixtures in the same package.

JSON Schema `$ref` values are limited to the document root (`#`) or a
document-local JSON Pointer (`#/...`). The closure proof resolves the target
and rejects missing or cyclic pointers. Anchor, dynamic, network, and package
path references are rejected, including every `$dynamicRef`, so validation
cannot fetch another resource or change resolution scope at runtime.
Inline `$id`, `$anchor`, `$dynamicAnchor`, `$recursiveAnchor`, and
`$recursiveRef` are also rejected, as is `$vocabulary`; any nested `$schema`
must still name Draft 2020-12. These limits keep the verifier's JSON Pointer
proof and the compiler aligned on one resolution base and one dialect.

Object schemas are closed by default and must set
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

The provider's ten kinds now have independent exact `1.0.1 / standard`
definition candidates and local structural fixtures. Their package-set
classification remains `structural-candidate`; `status: standard` pins proposed
final definition bytes and is not admission. Their earlier `0.0.0-legacy.1`
characterization packages remain compatibility candidates. Both sets are ten
exact one-definition packages, never one multi-definition package. Portable
host/provider admission evidence is externally supplied and is not synthesized
by the package generator; only authenticated evidence may classify an exact
package `portable-standard`.
