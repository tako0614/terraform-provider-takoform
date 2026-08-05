# Form Proposal workspace

This directory contains mutable design material for Forms that have not earned
a public FormRef. A document here is not a package, release, maturity claim,
Host Support decision, or Service Offering.

The active v1alpha2 Proposal set begins with the nine Form-backed Resources
currently implemented by Takosumi Cloud. Cloud provides a real workload and
first host; it does not provide Takoform maturity or publication authority.
Each Proposal must still pass its own portability, prior-art, fixture,
migration, security, provider, package, and public-readback evidence.

Start another Proposal by copying [`TEMPLATE.md`](TEMPLATE.md) to a descriptive
lowercase filename and adding one matching entry to
[`../forms/lifecycle.json`](../forms/lifecycle.json) under `proposals`. Keep
`currentForms` unchanged until an exact v1alpha2 `0.x` Form is carried by a
v1alpha3 package and all Experimental evidence exists.

Before review, run:

```bash
go run ./cmd/standard-form-conformance verify
```

The verifier compiles [`../forms/lifecycle.schema.json`](../forms/lifecycle.schema.json),
strictly validates the registry, checks repository-local evidence paths without
following symlinks, and rejects a Proposal that claims a FormRef or release
identity. Proposal review does not authorize publication.

When a Proposal is withdrawn, remove its registry entry and document unless a
decision record needs to retain the design history. No public identity is
reserved by a withdrawn Proposal.
