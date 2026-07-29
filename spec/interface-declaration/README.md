# Interface declaration v1alpha1

A Form Definition or a standalone `takoform_interface` IaC resource MAY declare
a runtime interface a service exposes. The portable declaration says what
exists, the exact non-secret document, its optional schema, and how public
values are resolved. The host owns the resulting record, consumer
authorization, identity fencing, and lifecycle.

Requirement keywords are used as described in
[`../conformance.md`](../conformance.md).

```json
{
  "interfaces": [
    {
      "name": "example.runtime",
      "version": "1",
      "required": true,
      "resourceUriInput": "resource_uri",
      "document": { "title": "Application runtime surface" },
      "documentSchema": {
        "$schema": "https://json-schema.org/draft/2020-12/schema",
        "type": "object",
        "additionalProperties": false,
        "properties": { "title": { "type": "string" } },
        "required": ["title"]
      },
      "inputs": [
        { "name": "endpoint", "source": "output", "pointer": "/url" },
        { "name": "resource_uri", "source": "resource_uri" },
        { "name": "revision", "source": "literal", "value": 1 }
      ]
    }
  ]
}
```

## Exact identity and document

Interface identity is the pair `(name, version)`, serialized as
`name@version` only for display. Duplicate pairs are invalid; the same name may
appear at multiple distinct versions. Both tokens are author-defined. There is
no registry, allowlist, or central approval for interface names or versions.

This descriptor identity does not identify one runtime instance. Multiple
Resources can realize the same Form and descriptor. A runtime declaration is
therefore selected by `(space, resource.kind, resource.name, name, version)`.
The portable Resource reference contains only `{kind,name}`; no host Interface
record id or binding is portable.

`document` is the exact data-only, non-secret document a host copies into the
declaration. A host must not synthesize it from `description` or other fields.
When `documentSchema` is present, the document must validate against it. If
`document` is omitted, its effective value is `{}` and the definition is valid
only when the schema accepts `{}`. `documentSchema` uses the same closed-object,
local-reference, and bounded-work proof as desired and observed schemas.

## Deterministic input mapping

`inputs` contains data, never expressions, templates, commands, or network
requests.

| Source | Meaning | Carries |
| --- | --- | --- |
| `literal` | exact declared JSON constant, including `null` | `value`, no `pointer` |
| `output` | Form's own output document | optional RFC 6901 `pointer`, no `value` |
| `resource_uri` | host's canonical credential-free HTTPS OAuth resource URI for this declaration | no `pointer` or `value` |
| `<host>.<token>` | explicitly non-portable host source | optional RFC 6901 `pointer`, no `value` |

The empty pointer selects the whole source document. Non-empty pointers start
with `/`; `~0` encodes `~` and `~1` encodes `/`. Any dangling or different `~`
escape is invalid. Input names are unique within one descriptor.

A host-namespaced source prevents this project from becoming a central
vocabulary gate. A host may reject it, and a host that does not understand it
must fail closed instead of dropping that input. Host ledger identifiers and
credentials never appear in a Form Definition.

`resourceUriInput` is optional. When present, it names exactly one input whose
source is `resource_uri`. The host resolves that input to its canonical OAuth
resource URI. The resolved value must be an absolute credential-free HTTPS URI
and may be used as an audience fence. The marker
does not declare a token, grant consumer access, create an InterfaceBinding, or
authorize any caller.

## Required semantics

`required: true` is a readiness requirement, not an authorization grant. A host
advertising `interface_declarations` must not report the Resource Ready unless
the exact declaration was materialized, its document validated, and every input
resolved. A host that does not advertise the optional feature remains generally
conforming, but it must reject admission/activation of a Form whose required
declaration it cannot honor; it must not silently install that Resource Ready.

An optional descriptor may be skipped. If skipped, it must not be falsely
listed. Listing any descriptor implies no consumer permission.

## Standalone IaC declaration

`resource "takoform_interface"` carries the same generic descriptor fields
directly:

- exact `name` and `version`;
- the exposing Resource `{kind,name}` and Space;
- `document`, optional `documentSchema`, `inputs`, and optional
  `resourceUriInput`.

The resource is deliberately not a protocol catalogue. It does not define
MCP-, HTTP-, UI-, storage-, or vendor-specific blocks. Application authors put
their complete data-only document in `document_json` and their deterministic
input declarations in `inputs_json`.

A compatible host that supports writes advertises both
`interface_declarations` and `interface_declaration_writes`. `PUT` is
idempotent and uses the same optimistic-concurrency preconditions as Resource
lifecycle operations; `DELETE` is generation-fenced. The portable address is
`(space, resource.kind, resource.name, name, version)`, so no host Interface id
enters configuration or state.

The provider endpoint is control-plane transport only. Resolved application
endpoints are returned as public `values` and, when `resourceUriInput` is
declared, as `resourceUri`. Runtime consumers discover and invoke that URI
directly under host-governed authorization. Environment variables configure
the provider origin, Space, and bearer credential; they are not a second
source of truth for an application's runtime endpoint.

## Hard boundary

A declaration contains no credentials, tokens, authorization, record identity,
generation, provenance, target, placement, capacity, price, billing, quota,
policy, or executable content. A portable declaration write creates no binding
or consumer grant.
