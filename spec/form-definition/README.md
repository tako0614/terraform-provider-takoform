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
- inline Draft 2020-12 desired and observed schemas;
- optional immutable JSON Pointer fields;
- an explicit subset of `create`, `update`, `observe`, `delete`, and `import`;
- optional non-secret Interface document schemas;
- optional references to data-only conformance payloads in the same package.

All JSON Schema references are document-local fragments. Network and
package-path `$ref`/`$dynamicRef` values are rejected, so schema validation
cannot fetch another resource.

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

## Hard boundary

Definitions and every JSON payload are recursively checked for credential,
secret, token, account, operator, target/pool, capacity, backend manager,
provider config, price, SKU, billing, quota, SLA/support policy, executable,
command, script, source/adapter/validation/runtime code, WebAssembly, and plugin
fields. This policy is intentionally fail-closed. A host-owned implementation,
placement, commercial configuration, or executable extension is not portable
Form Definition data.

The check is structural: normalized exact names and exact camelCase,
snake_case, or kebab-case tokens are compared with a reviewed forbidden
vocabulary. It does not use substring matching. Standard schema keys such as
`description`, and prose values that discuss authentication or billing, remain
valid; fields such as `authorization`, `oauthClient`, `sessionCookie`,
`invoice`, `paymentMethod`, `currency`, `taxCode`, `serviceOffering`,
`managerId`, and `region` do not.

This contract does not standardize the provider's ten characterization kinds.
Each kind remains a compatibility candidate until its own one-definition
package and conformance review is completed. A ten-kind compatibility set is
ten exact FormRefs/packages plus an external mapping payload, never one
multi-definition package.
