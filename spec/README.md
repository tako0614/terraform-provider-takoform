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
- [`../forms/README.md`](../forms/README.md) — the exact ten-kind Stable set and retained legacy inventory;
- [`../conformance/README.md`](../conformance/README.md) — current evidence and the next fixture boundary.

## Status

The FormRef, Form Definition, package-index, revocation, and cumulative
revocation-checkpoint schemas, RFC
8785/I-JSON library, closed local verifier, positive/negative corpus, protected
keyless Sigstore release lane, and signed append-only checkpoint delivery lane
are implemented. The ten `1.0.0` Form Packages have real immutable releases,
and their retained package indexes pass offline production-root/publisher-policy
verification. No revocation checkpoint or admission activation has been
published. Remote host fetch/install, host publisher-policy verification,
activation, and revocation enforcement remain consumer/operator work. The
current ten provider resources pin independent exact `1.0.0 / standard`
definition candidates with local structural verification. Their inventory is
`structural-candidate`, not `portable-standard`; definition status pins proposed
final bytes and does not perform admission. The historical
`0.0.0-legacy.1` packages remain frozen compatibility candidates and were not
renamed or promoted. Passed Takosumi host and Terraform provider lifecycle
evidence, portable negative wire-code coverage, Registry installation/readback,
signed admission evidence, and live revocation-chain proof are still external
requirements. Package signatures and immutable tags are now retained
publication evidence only. Authenticated host and provider evidence is the only
path to `portable-standard` classification.

The project identity is `forms.takoform.com/v1alpha1`; the Terraform provider identity is `registry.terraform.io/tako0614/takoform`.
