# Form Proposal workspace

This directory contains mutable design material for Forms that have not earned
a public FormRef. A document here is not a package, release, maturity claim,
Host Support decision, or Service Offering.

Family design work lives in per-family subdirectories; the first is the
[Edge Platform Family](edge/README.md), whose candidates are authored under
the shape-preserving boundary of
[decision 0008](../spec/decisions/0008-forms-preserve-service-shape.md).
Family Proposals are tracked by the family candidate set
([`../forms/candidates/edge/v1beta1/candidate-set.json`](../forms/candidates/edge/v1beta1/candidate-set.json)).
The pre-family flat Proposal set and its lifecycle registry were withdrawn
with the pre-Beta epochs
([decision 0042](../spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.md));
their documents stay in git history as design provenance and reserve no public
identity.

Start a new family Proposal by copying [`TEMPLATE.md`](TEMPLATE.md) into the
family's subdirectory under a descriptive lowercase filename. A Form enters
the family only through the generated candidate set; the public-surface gate
holds every hand-written inventory to that roster, so a Proposal without a
candidate entry cannot leak into the published surface.

Before review, run:

```bash
go run ./cmd/standard-form-conformance verify
```

Proposal review does not authorize publication. When a Proposal is withdrawn,
remove its document unless a decision record needs to retain the design
history; no public identity is reserved by a withdrawn Proposal.
