# Takoform portable specification

This directory is the Phase 0 specification surface for the standalone Takoform project. It records the portable boundary implemented by the current form-only provider while the complete versioned Form Definition and signed Form Package contracts are still being designed.

Current committed surfaces:

- [`host-api/`](host-api/) — the minimal discovery, capability, preview, apply, observe, and delete contract used by the provider candidate;
- [`form-definition/`](form-definition/) — the target data-only definition boundary and unresolved standardization work;
- [`trust/`](trust/) — the D-08 provider/Form Package trust decision and its machine-readable fail-closed profile;
- [`../schemas/host-discovery.schema.json`](../schemas/host-discovery.schema.json) — machine-readable discovery validation;
- [`../forms/README.md`](../forms/README.md) — the exact ten-kind compatibility inventory;
- [`../conformance/README.md`](../conformance/README.md) — current evidence and the next fixture boundary.

## Status

This is an honest characterization scaffold, not a declaration that every current field is a stable portable standard. The D-08 trust profile selects separate provider and Form Package trust lanes, but the Form Package schema, signing workflow, verifier, and remote installation implementation do not exist yet. Until those implementations and conformance fixtures land, the ten resources are a frozen compatibility candidate.

The project identity is `forms.takoform.com/v1alpha1`; the Terraform provider identity is `registry.terraform.io/tako0614/takoform`.
