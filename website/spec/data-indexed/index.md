# `data.indexed@1`

`data.indexed@1` is the retained runtime Interface descriptor declared by the
v1alpha1 Legacy `IndexedStore` Form. No current v1alpha2 candidate declares
it. Requirement keywords are used as described in
[`../conformance.md`](../conformance.md).

## Retained descriptor

The exact Interface identity is the open pair `data.indexed` / `1`. Its
portable document contains only these operation capability tokens:

```json
{
  "operations": [
    "delete",
    "get",
    "put",
    "query"
  ]
}
```

The document schema requires that exact four-item, unique, closed set. The
tokens advertise the bounded item operations an `IndexedStore` makes
available; they do not select a transport, endpoint, request envelope, query
language, consistency implementation, or authorization mechanism.

The descriptor maps exactly two values from the declaring Resource's public
output:

| Input | Source | JSON Pointer |
| --- | --- | --- |
| `resource` | `output` | `/id` |
| `name` | `output` | `/name` |

It declares no literal or host-resolved input. In particular, it declares no
endpoint, request/response schema digest, `resourceUriInput`, credential,
binding, token, target, backend, or commercial value.

The canonical machine-readable descriptor is the `interfaces` entry in the
retained Legacy
[`IndexedStore` Form Definition](../../conformance/form-package-v1/positive/standard/indexed-store/definition.json).
The immutable definition and retained Legacy catalogue are checked together
by `go run ./cmd/standard-form-conformance verify`.

## Host boundary

The Form owns the exact descriptor above. A host that materializes it owns the
Interface record, routing and endpoint choice, request protocol,
authorization and consumer bindings, credential issuance, lifecycle, target
selection, quota, billing, and implementation. The descriptor grants no
access and does not make those host decisions portable state.

## Deliberate omissions

This version defines no common endpoint or request/response payload contract.
It also defines no schema digests, revisions, cursor protocol, batching
protocol, or resource URI input. A future portable wire contract would require
a separately reviewed Interface descriptor/version and regenerated Form
Package; implementations MUST NOT infer one merely from the current
`data.indexed@1` name.
