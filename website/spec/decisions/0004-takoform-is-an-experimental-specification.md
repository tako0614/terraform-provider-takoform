# 0004 — Takoform is an experimental specification project

- Status: accepted
- Date: 2026-08-04
- Owners: Takoform maintainers
- Supersedes: admission and graduation policy in
  [0001](0001-provider-v1-keeps-form-versions-independent.md)

## Context

Takoform published immutable Form packages and introduced `standard` and
`portable-standard` classifications before independent hosts and sustained
real-world consumers had established interoperability. Package publication,
provider support, one host's lifecycle evidence, and commercial availability
were consequently presented as one maturity ladder. The result was excessive
Form version churn, a privileged ten-Form subset, and documentation that
overstated the maturity of the specification.

The published identities cannot be safely renumbered, overwritten, or erased.
The project can still provide a useful portable boundary for desired-state
schemas, content-addressed packages, host lifecycle interoperability, and
Terraform/OpenTofu clients, but that value must be demonstrated rather than
self-declared.

## Decision

Takoform remains a standalone **Experimental specification and tooling
project**. It is not currently an industry standard, certification authority,
universal cloud API, or guarantee of backend portability.

Current Form work follows four lifecycle states:

1. **Proposal** — mutable and unversioned design material tied to a named owner,
   real consumer, host implementation, and prior-art analysis;
2. **Experimental** — a reproducible public `0.x` FormRef whose contract may
   still change under the documented compatibility policy;
3. **Stable** — an evidence-earned contract with independent implementation,
   interoperability, migration, deprecation, and operational experience;
4. **Legacy** — an immutable retained identity that is no longer the basis for
   new specification work.

The existing public FormRefs, packages, schemas, tags, provider releases,
signed reports, and admission closures remain immutable legacy evidence. Their
historical `standard` and `portable-standard` fields do not make a current
normative approved set, and no replacement central admission generation is
introduced.

Form maturity, provider compatibility, Host Support, operator activation, and
Cloud availability are separate facts:

- the provider version describes only provider protocol, schema, state, and
  host-client compatibility;
- a host reports support for an exact FormRef and does not grant that Form
  maturity;
- an operator decides whether supported Forms are installed and activated;
- a commercial platform publishes a Service Offering for an exact supported
  FormRef without changing the Form's maturity.

OCCI, CIMI, TOSCA, Kubernetes/Crossplane, and established provider APIs are
required prior art where relevant, not automatic compliance targets. A new
Takoform abstraction must state the concrete difference and why it is needed.

## Consequences

- New Form designs can change freely as Proposals without consuming public
  versions or package identities.
- New public Form lines begin at `0.x`; existing public identities never rewind
  or reuse an occupied version.
- Stable is deliberately difficult to earn and cannot be inferred from a
  package, provider release, generated catalog entry, or a single host report.
- Existing users retain exact read, observe, delete, recovery, revocation, and
  explicit migration paths for legacy Forms.
- Takoform documentation must lead with experimental status, the portable
  boundary, known limitations, and exact host support rather than approval
  counts or release closure evidence.
- Decisions 0001 through 0003 remain historical rationale. This decision
  replaces only the active admission/graduation model; provider/Form version
  independence and immutable public identities remain in force.

## Rejected alternatives

- **Delete Takoform.** Rejected because the verified schema, package, trust,
  lifecycle, and IaC boundary remains useful if governed by real evidence.
- **Renumber the provider or published Forms back to `0.1.0`.** Rejected because
  provider compatibility is independent and published identities are
  immutable.
- **Keep ten approved Forms and treat the remainder as lower quality.** Rejected
  because one admission closure reflects evidence for exact identities, not a
  permanent normative hierarchy.
- **Adopt OCCI or CIMI wholesale.** Rejected because they remain important prior
  art but do not directly supply the active implementation ecosystem or modern
  edge/runtime contracts Takoform needs.
