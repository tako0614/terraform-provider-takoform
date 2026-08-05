# Historical admission v1 material

This directory retains the earliest production package and admission evidence
from the pre-reset `1.0.1` generation. Its `standard`, `portable-standard`,
candidate, workflow, and policy vocabulary records what that experiment
asserted at the time. It is not a current publication lane, maturity authority,
candidate queue, or host activation policy.

The immutable package snapshots, Sigstore roots/policies, host/provider
reports, registry readback, and release identities remain useful for verifying
historical bytes and Git objects. References to the former Takoform admission
workflows and Takosumi `standard-form-host-report.yml` identify the producers of
those retained signatures; those workflows are retired and must not be
recreated to make old instructions pass.

Use the repository root's history-only checks:

```console
go run ./cmd/standard-form-conformance legacy-published-package-check
go run ./cmd/standard-form-conformance legacy-admission-evidence-check
```

They authenticate retained history. They do not promote a Form in
`forms/lifecycle.json`, authorize Host Support, create a FormActivation, or
publish an Offering.
