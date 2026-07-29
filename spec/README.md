# Takoform portable specification

This directory is the portable specification surface for the standalone
Takoform project. It records both the provider characterization boundary and
the implemented data-only Form Package core.

Requirement keywords, conformance classes, and what a passing check does and
does not prove are defined in [`conformance.md`](conformance.md). How the API
group, Form definitions, packages, and the provider are versioned — and what
`v1alpha1` must satisfy to graduate — is in [`versioning.md`](versioning.md).

## Product contract map

Takoform has four public contract interfaces:

1. **Exact Form and Package data.**
   [`form-definition/`](form-definition/) defines immutable `FormRef` and the
   desired/observed/output shape. [`form-package/`](form-package/) binds one
   exact definition and its data-only fixtures into immutable package bytes.
2. **Desired Resource lifecycle.**
   [`host-api/`](host-api/) defines discovery, exact Form availability,
   preview/apply, read/import/observe/refresh/delete, fencing, and portable
   errors. The host chooses implementation and placement.
3. **Read-only Form-derived Interface projection.**
   [`interface-declaration/`](interface-declaration/) defines open
   `(name, version)` descriptors embedded in Forms and their read-only host
   projection. Focused contracts such as
   [`data.indexed@1`](data-indexed/) only define the descriptor data the current
   Form actually declares.
4. **Trust, version, and release identity.**
   [`trust/`](trust/) defines immutable publisher evidence and revocation;
   [`versioning.md`](versioning.md) keeps provider, API, Form, Package, and
   admission versions independent.
   [`release/`](../release/README.md) binds artifacts to those exact identities
   without changing the three contracts above.

[`schemas/`](schemas/), [`conformance.md`](conformance.md), and
[`decisions/`](decisions/) support those interfaces with structural minima,
executable evidence language, and decision rationale. They are not additional
product interfaces. The generated current inventory is
[`../forms/README.md`](../forms/README.md), host discovery validation is
[`../schemas/host-discovery.schema.json`](../schemas/host-discovery.schema.json),
and the local evidence map is
[`../conformance/README.md`](../conformance/README.md).

## Status

The FormRef, Form Definition, package-index, revocation, and cumulative
revocation-checkpoint schemas, the RFC 8785/I-JSON library, the closed local
verifier, the positive/negative corpus, the protected keyless Sigstore release
lane, and the signed append-only checkpoint delivery lane are implemented.

The portable Form set was rebuilt as intent-shaped kinds declared once in
`internal/formcatalog`; [`../forms/README.md`](../forms/README.md) is the
generated inventory. That set is `structural-candidate`: packages verify
locally, the provider derives the same schema from the same declaration, and
the protocol lifecycle runs against an in-process host. None of that admits a
Form. Signed release bytes, a conforming host's signed lifecycle report,
Registry installation and readback, and signed admission evidence are external
requirements.

The previously published generation is retired, not erased. Its immutable
`1.0.1` releases and admission evidence stay verifiable offline through
[`../forms/retired-package-set.json`](../forms/retired-package-set.json), and
this build refuses to reissue their proofs rather than restamp them with a new
provider identity.

The project identity is `forms.takoform.com/v1alpha1`; the Terraform provider identity is `registry.terraform.io/tako0614/takoform`.

## Normative consistency audit

`go test ./spec` is the cross-specification contradiction gate. It does not
repeat the Form Package verifier, provider schema tests, or portable-host
runner. Instead, it joins their machine-readable inputs and fails when:

- host operations, mutation fences, idempotency, or the stable error taxonomy
  disagree with the portable-host conformance contract;
- the optional Interface projection stops being read-only, same-origin, and
  materialized only from Form-declared descriptors;
- the portable API identity, provider candidate version, or canonical provider
  FQN diverges between release, schema, trust, and conformance locks; or
- a normative active Form, package, schema, or host contract leaks a concrete
  backend vocabulary such as Cloudflare/Workers configuration.

The complete repository gate, `bun run check`, runs this audit together with
the deeper package-byte, provider-schema, generated-surface, and lifecycle
verifiers. Passing it remains local evidence only; it does not prove Registry
publication, host admission, or production interoperability.
