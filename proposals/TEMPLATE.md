# Form Proposal: <working name>

Status: draft Proposal; no FormRef or release identity

## Need

- Consumer:
- Real workload:
- Maintainer responsible for portable semantics:
- Intended host implementations:
- Why a host-specific resource or an existing Form is insufficient:

## Portable boundary

Describe only desired state and observable outcomes that every intended host
can implement with the same meaning.

## Host decisions

List placement, capacity, credentials, policy, pricing, availability, and other
decisions deliberately left to each host or operator.

## Substrate-neutrality review

Classify every proposed desired field. Explain why it is consumer-visible
workload meaning rather than placement, capacity, scaling, routing, health,
retry, compatibility, pricing, or another host/operator concern. Name the two
independent implementation models expected to give it the same semantics.

Cloudflare-like general concepts are allowed. A Cloudflare product/config key,
account model, limit, or compatibility API is not portable merely because the
field uses a generic name.

## Lifecycle risks

- Replacement:
- Data loss:
- Delete and retained recovery behavior:
- Import:
- Drift and refresh:
- Migration or rollback if the design changes:

## Security boundary

- Credentials:
- Network:
- Artifacts and provenance:
- Secrets and sensitive state:

## Prior art

For every row, record `applicable` or `not-applicable` and a concrete finding.

| Family | Applicability | Finding |
| --- | --- | --- |
| OCCI |  |  |
| CIMI |  |  |
| TOSCA |  |  |
| Kubernetes/Crossplane |  |  |
| Terraform/OpenTofu |  |  |

## Existing abstraction gap

State the narrow semantic gap that remains after the prior-art review. Product
preference, naming preference, or one provider's implementation detail is not
enough.

## Experimental exit evidence

Do not fill this with plans presented as results. Link exact evidence only when
it exists:

- canonical definition and Form Package:
- positive and negative fixtures:
- host lifecycle implementation:
- real consumer:
- known limitations:
- compatibility and migration:
- security review:
- generated and narrative documentation:
- immutable publication, signature, and public readback plan:
