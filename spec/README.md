# Takoform portable specification

This directory is the portable specification surface for the standalone
Takoform project. It records both the provider characterization boundary and
the implemented data-only Form Package core.

Requirement keywords, conformance classes, and what a passing check does and
does not prove are defined in [`conformance.md`](conformance.md). How the API
group, Form definitions, packages, and the provider are versioned — and what
`v1alpha1` must satisfy to graduate — is in [`versioning.md`](versioning.md).

Current committed surfaces:

- [`conformance.md`](conformance.md) — requirement keywords and the four conformance classes;
- [`versioning.md`](versioning.md) — independent version streams, stability, and deprecation;
- [`schemas/`](schemas/) — the normative machine-readable schemas;
- [`host-api/`](host-api/) — the discovery, availability, preview, apply, read, import, observe, refresh, and delete contract, with [`operations.json`](host-api/operations.json) as its machine-readable form;
- [`form-definition/`](form-definition/) — exact FormRef and data-only Form Definition contract;
- [`form-package/`](form-package/) — package-index identity, closed payload rules, and local verifier boundary;
- [`interface-declaration/`](interface-declaration/) — open `(name, version)` runtime interface descriptors, exact non-secret documents, and deterministic input mappings;
- [`data-indexed/`](data-indexed/) — a proposed bounded key and declared-index operation contract for `data.indexed@1`, required by no Form in this release;
- [`trust/`](trust/) — the D-08 provider/Form Package trust decision and its machine-readable fail-closed profile;
- [`../schemas/host-discovery.schema.json`](../schemas/host-discovery.schema.json) — machine-readable discovery validation;
- [`../forms/README.md`](../forms/README.md) — the generated portable Form inventory;
- [`../conformance/README.md`](../conformance/README.md) — current evidence and the next fixture boundary.

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
