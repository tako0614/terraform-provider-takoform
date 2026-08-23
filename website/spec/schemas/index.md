# Normative schemas

These are the normative structural minima of the Takoform specification.
Schema validity is necessary but not sufficient: the semantic verifier rules
in the owning Form Definition, Form Package, Interface, and trust contracts
are also normative and may reject a structurally valid document. Where prose
and a schema directly disagree about the same structural condition, the schema
wins.

| Schema | Contract |
| --- | --- |
| [`form-ref-v1beta1.schema.json`](form-ref-v1beta1.schema.json) | the Beta namespaced-group four-field immutable Form reference |
| [`form-definition-v1beta1.schema.json`](form-definition-v1beta1.schema.json) | the Beta data-only Form Definition with roles, exact Interfaces, and typed Bindings |
| [`package-index-v1alpha4.schema.json`](package-index-v1alpha4.schema.json) | the content-addressed package profile for namespaced-group FormRefs |
| [`form-package-revocation.schema.json`](form-package-revocation.schema.json) | one append-only revocation statement |
| [`form-package-revocation-checkpoint.schema.json`](form-package-revocation-checkpoint.schema.json) | the cumulative, hash-chained revocation checkpoint |
| [`host-discovery-v1beta1.schema.json`](host-discovery-v1beta1.schema.json) | the Beta host discovery document |
| [`host-api-wire-v1beta1.schema.json`](host-api-wire-v1beta1.schema.json) | the Beta UID/generation/revision Resource, condition, operation, artifact, and error envelopes |
| [`interface-ref-v1alpha1.schema.json`](interface-ref-v1alpha1.schema.json) | the exact digest-bound Interface reference |
| [`interface-definition-v1alpha1.schema.json`](interface-definition-v1alpha1.schema.json) | the exact data-only Interface Definition with operations, semantics, and behavior fixtures |
| [`binding-ref-v1alpha1.schema.json`](binding-ref-v1alpha1.schema.json) | the exact digest-bound Binding reference |
| [`binding-definition-v1alpha1.schema.json`](binding-definition-v1alpha1.schema.json) | the retained typed Binding Definition, whose target groups all carry a version |
| [`binding-definition-v1alpha2.schema.json`](binding-definition-v1alpha2.schema.json) | the current typed Binding Definition: source role, target Interface, runtime projection, and a target group that may carry no version |
| [`binding-ref-v1alpha2.schema.json`](binding-ref-v1alpha2.schema.json) | the exact BindingRef into the current Binding envelope |
| [`artifact-manifest-v1alpha1.schema.json`](artifact-manifest-v1alpha1.schema.json) | the content-addressed artifact manifest for uploaded bundles |
| [`operation-v1alpha1.schema.json`](operation-v1alpha1.schema.json) | the long-running Operation envelope |
| [`host-support-profile-v1alpha1.schema.json`](host-support-profile-v1alpha1.schema.json) | the Host Support Profile: supported refs, capability subsets, and limits |

The withdrawn pre-Beta epochs' schemas (the v1alpha1, v1alpha2, and v1alpha3
FormRef/definition/package/discovery/wire sets) are recorded as retired
identities in the ledger below and are deliberately absent here
([decision 0042](../decisions/0042-the-pre-beta-epochs-are-withdrawn.md)).
The `formpackage` verifier keeps embedded copies of every
epoch's package schemas so bytes retained in git history and release tags stay
verifiable.

The Form Package verifier embeds its own copies of the package schemas so it
has no filesystem dependency at runtime. The wire-envelope schema is normative
and is consumed by host/provider conformance rather than embedded in the
data-only package verifier. `go test ./spec` compiles every schema and proves
every implementation copy is byte-identical to its normative source.

Every schema `$id` is also a retrieval URL. The files in this directory are the
only source; `bun run sync:public-schemas` projects them byte-for-byte into
`website/public/schemas/`, and `bun run check:public-surfaces` rejects missing,
extra, or drifted public copies. Do not edit the generated public copies.

Once a `$id` URL resolves, its bytes are an immutable published identity while
they are served. [`release/public-schema-identities.json`](../../release/public-schema-identities.json)
is the append-only ledger of every such identity, its exact digest, normative
source, and public projection; a withdrawn identity moves to the ledger's
`retired` list with the bytes it had and the reason, and can never be reused
for different bytes. The public-surface gate requires the normative set to
equal that ledger. Deployment also compares the candidate ledger with every
retained ledger version in repository history, so a published identity cannot
disappear by deleting both its source and current ledger entry.

Deployment fetches every retained URL immediately before mutation and refuses
to overwrite a differing or unavailable identity. A wholly DNS-absent origin
can be minted only through the explicit initial-origin acknowledgement
documented in `website/README.md`; that
acknowledgement cannot bypass any existing response or mismatch.
