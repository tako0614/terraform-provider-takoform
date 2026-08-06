# Form Proposal workspace

This directory contains mutable design material for Forms that have not earned
a public FormRef. A document here is not a package, release, maturity claim,
Host Support decision, or Service Offering.

Current family design work lives in per-family subdirectories; the first is
the [Edge Platform Family](edge/index.md), whose candidates are authored
under the shape-preserving boundary of
[decision 0008](../spec/decisions/0008-forms-preserve-service-shape.md).

The retained v1alpha2 Proposal set is the nine Form-backed Resources
implemented by Takosumi Cloud; it remains reproducible provider-v2 preview
source. Cloud provides a real workload and first host; it does not provide
Takoform maturity or publication authority. Each Proposal must still pass its
own portability, prior-art, fixture, migration, security, provider, package,
and public-readback evidence.

Start another retained-line Proposal by copying `TEMPLATE.md`
to a descriptive
lowercase filename and adding one matching entry to
[`../forms/lifecycle.json`](../forms/lifecycle.json) under `proposals`. Keep
`currentForms` unchanged until an exact v1alpha2 `0.x` Form is carried by a
v1alpha3 package and all Experimental evidence exists. Family Proposals
instead live in per-family subdirectories such as [`edge/`](edge/) and are
tracked by the family candidate set
([`../forms/candidates/edge/v1alpha1/candidate-set.json`](../forms/candidates/edge/v1alpha1/candidate-set.json)),
not [`../forms/lifecycle.json`](../forms/lifecycle.json), until an
Experimental transition defines family lifecycle records.

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
