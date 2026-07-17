# Takoform portable specification

This directory is the portable specification surface for the standalone
Takoform project. It records both the provider characterization boundary and
the implemented data-only Form Package core.

Current committed surfaces:

- [`host-api/`](host-api/) — the minimal discovery, capability, preview, apply, observe, and delete contract used by the provider candidate;
- [`form-definition/`](form-definition/) — exact FormRef and data-only Form Definition contract;
- [`form-package/`](form-package/) — package-index identity, closed payload rules, and local verifier boundary;
- [`trust/`](trust/) — the D-08 provider/Form Package trust decision and its machine-readable fail-closed profile;
- [`../schemas/host-discovery.schema.json`](../schemas/host-discovery.schema.json) — machine-readable discovery validation;
- [`../forms/README.md`](../forms/README.md) — the exact ten-kind compatibility inventory;
- [`../conformance/README.md`](../conformance/README.md) — current evidence and the next fixture boundary.

## Status

The FormRef, Form Definition, package-index and revocation schemas, RFC
8785/I-JSON library, closed local verifier, positive/negative corpus, protected
keyless Sigstore release lane, and signed append-only revocation delivery lane
are implemented. No real Form Package or revocation statement has been
published. Remote host fetch/install, host publisher-policy verification,
activation, and revocation enforcement remain consumer/operator work. The
current ten provider resources remain frozen compatibility candidates; a
release lane does not silently standardize them.

The project identity is `forms.takoform.com/v1alpha1`; the Terraform provider identity is `registry.terraform.io/tako0614/takoform`.
