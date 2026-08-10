# Form Proposal workspace

This directory contains mutable design material for Forms that have not earned
a public FormRef. A document here is not a package, release, maturity claim,
Host Support decision, or Service Offering.

Current family design work lives in per-family subdirectories; the first is
the [Edge Platform Family](edge/index.md), whose candidates are authored
under the shape-preserving boundary of
[decision 0008](../spec/decisions/0008-forms-preserve-service-shape.md).

The retained v1alpha2 Proposal set is the nine Form-backed Resources evaluated
against a Takosumi-hosted preview during the v1alpha2 reset. That dated
evaluation is provenance only: it does not establish that any hosted product
provides or runs these Forms now, and a host never grants Takoform maturity or
publication authority. Takoform owns every maturity decision. A Takosumi
deployment may be one independent adopter, but Stable/1.0 promotion requires
the independent conformance and real-consumer evidence recorded by Takoform.
Each Proposal must still pass its own portability, prior-art, fixture,
migration, security, provider, package, and public-readback evidence.

Start another retained-line Proposal by copying `TEMPLATE.md`
to a descriptive
lowercase filename and adding one matching entry to
[`../forms/lifecycle.json`](../forms/lifecycle.json) under `proposals`. Keep
the retained central-line `currentForms` registry unchanged until an exact
central v1alpha2 `0.x` Form is carried by a v1alpha3 package and all
Experimental evidence exists. Current Family Proposals
instead live in per-family subdirectories such as [`edge/`](edge/) and are
tracked by the family candidate set
([`../forms/candidates/edge/v1beta1/candidate-set.json`](../forms/candidates/edge/v1beta1/candidate-set.json)),
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
