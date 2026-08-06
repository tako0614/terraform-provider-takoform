# Interface declaration v1alpha1

A Form Definition MAY declare a runtime interface its service exposes. The
portable descriptor says what exists, the exact non-secret document, its
optional schema, and how public values are resolved. The Takoform data source
MAY read a projection made visible by the host, but it neither creates nor owns
an Interface record. The host (Takosumi in the Takos ecosystem) owns Interface
records and bindings, write fencing, lifecycle, authorization, and token
issuance.

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
using the same `credentialFreeHTTPSURL` grammar as an immutable artifact URL:
literal lowercase `https://`, a dotted ASCII hostname, an optional one-to-five
decimal-digit port, and an optional path without whitespace, `?`, or `#`.
Userinfo, query, fragment, single-label and Unicode hostnames are forbidden.
Unicode path text may be direct or percent-encoded; an internationalized
hostname must use its dotted ASCII IDNA representation. The whole value must
also be valid URI syntax, so malformed percent escapes and control characters
are forbidden. The URI may be used as an audience fence. The marker does not
declare a token, grant consumer access, create an InterfaceBinding, or
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

## Read-only IaC projection

The Takoform provider MAY expose `data "takoform_interface"` to read an exact
host projection selected by the descriptor identity and exposing Resource.
The projection may contain the declaration's non-secret document, resolved
public values, and declaring Form kind. It MUST NOT expose a host Interface
record id, binding, credential, token, write generation, or lifecycle state.

Every projection read is scoped to one effective Space. An explicit data
source `space` selector takes precedence; otherwise the provider's configured
default Space is used. A provider MUST fail before making a request when
neither supplies a non-empty Space, and MUST send that effective value as the
`space` query parameter on both list and exact reads. The host MUST scope
visibility and ambiguity checks to that Space and MUST NOT substitute another
Space.

The provider endpoint is control-plane transport only. Resolved application
endpoints may be returned as public values and, when `resourceUriInput` is
declared, as a credential-free `resourceUri`. Runtime consumers discover and
invoke that URI directly under host-governed authorization. Environment
variables configure the provider origin, Space, and bearer credential; they
are not a second source of truth for an application's runtime endpoint.

Reading a projection does not import the Interface into Terraform state as a
managed object. It grants no consumer access and gives Terraform no authority
to mutate the corresponding host record.

## No portable Interface write path

This specification MUST NOT define a `resource "takoform_interface"`, a generic
Interface create/update/delete operation, or any portable `PUT` or `DELETE`
transport for Interface records. It also MUST NOT place a host record id,
write-generation fence, binding, or lifecycle state in portable provider
state.

Form descriptors are declarative inputs that a host interprets while it owns
Resource activation. They do not create a second lifecycle authority. The host
(Takosumi in the Takos ecosystem) exclusively creates and updates Interface
records, manages bindings and write fences, authorizes consumers, issues
tokens, and retires records with the exposing Resource. A host-specific write
API, if one exists, is outside the portable Takoform contract and MUST NOT be
presented as a portable Takoform resource.

## Hard boundary

A declaration contains no credentials, tokens, authorization, record identity,
generation, provenance, target, placement, capacity, price, billing, quota,
policy, or executable content. No portable Interface declaration write exists;
reading a projection creates no record, binding, or consumer grant.
