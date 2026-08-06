# Normative schemas

These are the normative structural minima of the Takoform specification.
Schema validity is necessary but not sufficient: the semantic verifier rules
in the owning Form Definition, Form Package, Interface, and trust contracts
are also normative and may reject a structurally valid document. Where prose
and a schema directly disagree about the same structural condition, the schema
wins.

| Schema | Contract |
| --- | --- |
| [`form-ref.schema.json`](form-ref.schema.json) | the frozen v1alpha1 four-field immutable Form reference |
| [`form-ref-v1alpha2.schema.json`](form-ref-v1alpha2.schema.json) | the current v1alpha2 four-field immutable Form reference |
| [`form-definition.schema.json`](form-definition.schema.json) | the frozen v1alpha1 data-only Form Definition |
| [`form-definition-v1alpha2.schema.json`](form-definition-v1alpha2.schema.json) | the current v1alpha2 data-only Form Definition |
| [`package-index.schema.json`](package-index.schema.json) | the closed Form Package inventory and its identity |
| [`package-index-v1alpha2.schema.json`](package-index-v1alpha2.schema.json) | the retained content-addressed package profile for v1alpha1 FormRefs |
| [`package-index-v1alpha3.schema.json`](package-index-v1alpha3.schema.json) | the current content-addressed package profile for v1alpha2 FormRefs |
| [`form-package-revocation.schema.json`](form-package-revocation.schema.json) | one append-only revocation statement |
| [`form-package-revocation-checkpoint.schema.json`](form-package-revocation-checkpoint.schema.json) | the cumulative, hash-chained revocation checkpoint |
| [`host-discovery.schema.json`](host-discovery.schema.json) | the frozen provider-v1 host discovery document |
| [`host-api-wire.schema.json`](host-api-wire.schema.json) | the frozen provider-v1 Resource and Interface wire envelopes |
| [`host-discovery-v1alpha2.schema.json`](host-discovery-v1alpha2.schema.json) | the current provider-v2 host discovery document |
| [`host-api-wire-v1alpha2.schema.json`](host-api-wire-v1alpha2.schema.json) | the current Resource, lifecycle response, Interface projection, and error envelopes |

The Form Package verifier embeds its own copies of the package schemas so it
has no filesystem dependency at runtime. The host discovery implementation
keeps its published copy under `schemas/`. The wire-envelope schema is
normative and is consumed by host/provider conformance rather than embedded in
the data-only package verifier. `go test ./spec` compiles every schema and
proves every implementation copy is byte-identical to its normative source.

Every schema `$id` is also a retrieval URL. The files in this directory are the
only source; `bun run sync:public-schemas` projects them byte-for-byte into
`website/public/schemas/`, and `bun run check:public-surfaces` rejects missing,
extra, or drifted public copies. Do not edit the generated public copies.

Once a `$id` URL resolves, its bytes are an immutable published identity.
[`release/public-schema-identities.json`](../../release/public-schema-identities.json)
is the append-only ledger of every such identity, its exact digest, normative
source, and public projection. The public-surface gate requires the normative
set to equal that ledger. Deployment also compares the candidate ledger with
every retained ledger version in repository history, so a published identity
cannot disappear by deleting both its source and current ledger entry.

Deployment fetches every retained URL immediately before mutation and refuses
to overwrite a differing or unavailable identity. A wholly DNS-absent origin
can be minted only through the explicit initial-origin acknowledgement
documented in [`website/README.md`]; that
acknowledgement cannot bypass any existing response or mismatch.
