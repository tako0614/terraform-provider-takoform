# 0055 — Specification release needs only the normative source

- Status: Accepted
- Date: 2026-08-24
- Supersedes: decisions 0052 and 0053 only where they require candidate-corpus and reference-Host evidence for Specification 1.0

## Context

Takoform is a specification project. Its candidate Forms, reference Host,
official Terraform Provider, and product integrations are useful ways to test
and demonstrate that specification, but they are implementations and adoption
evidence. Requiring every current family corpus and the repository-owned
reference Host to pass before the Specification can be numbered makes the
sample distribution an authority over the standard it is meant to implement.

The separation established by decisions 0052 and 0053 remains correct:
Provider, Host, backend, production, Takoserver, Takosumi, signer, and operator
facts do not govern the Specification. The remaining problem is that the old
three-prerequisite rule still gave Takoform's own candidate catalog and
reference implementation that authority.

## Decision

Specification 1.0 has one release prerequisite:
`specification-source-snapshot`.

The snapshot binds one reachable committed revision of the normative `spec/`
tree by its exact path set and content digests. The portable repository gate
continues to validate the schemas and internal references in that tree. The
publication record itself is excluded from its own digest.

The generated candidate-family index, Form Packages, Interface and Binding
catalogs, conformance corpora, reference Host execution, official Provider,
and external product evidence remain independently checkable. Missing or
failing evidence in those classes does not block the Specification assertion
and is not copied into a numbered Specification release.

This does not weaken an implementation's claims. A Host, Provider, Form
Package, or product may claim only the exact conformance evidence it actually
has. It also does not promote any current `0.x` FormRef, publish a package,
release Provider 3, deploy a Host, or authorize production mutation.

## Consequences

The Specification can be numbered after its own normative bytes are committed
and self-consistent. Reference conformance and all-family coverage remain
valuable quality signals, but they are no longer release authority. The
release ledger becomes smaller: it records the source commit and digest of the
normative snapshot, while implementation streams keep their own evidence and
cadence.
