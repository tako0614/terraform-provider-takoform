# Normative schemas

These are the normative machine-readable artifacts of the Takoform
specification. Where this repository's prose and one of these schemas
disagree, the schema wins.

| Schema | Contract |
| --- | --- |
| [`form-ref.schema.json`](form-ref.schema.json) | the exact four-field immutable Form reference |
| [`form-definition.schema.json`](form-definition.schema.json) | the data-only Form Definition |
| [`package-index.schema.json`](package-index.schema.json) | the closed Form Package inventory and its identity |
| [`form-package-revocation.schema.json`](form-package-revocation.schema.json) | one append-only revocation statement |
| [`form-package-revocation-checkpoint.schema.json`](form-package-revocation-checkpoint.schema.json) | the cumulative, hash-chained revocation checkpoint |
| [`host-discovery.schema.json`](host-discovery.schema.json) | the versioned host discovery document |

The Go implementation embeds its own copies so the verifier has no filesystem
dependency at runtime. `go test ./spec` proves those copies are byte-identical
to these files, so the implementation can never quietly diverge from the
specification it claims to implement.
