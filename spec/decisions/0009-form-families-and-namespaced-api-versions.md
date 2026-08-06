# 0009 — Form Families and namespaced API groups

- Status: accepted
- Date: 2026-08-06
- Owners: Takoform maintainers

## Context

Every current FormRef carries the single central group
`forms.takoform.com/v1alpha2`. Decision
[0008](0008-forms-preserve-service-shape.md) multiplies the number of Forms:
one shape per proven service primitive, plus revision, deployment, attachment,
and policy resources around each identity. A single flat namespace cannot
carry many coherent product families, third-party Forms, or independent
family evolution without renumbering the whole epoch.

## Decision

Forms are organized into **Form Families**: named groups of Forms that share a
platform model and are designed to compose. A family is a catalog grouping and
an API namespace; it is not a package unit. The existing trust boundary — one
Form Package contains exactly one Form Definition — is retained.

The FormRef `apiVersion` becomes a DNS-like group with its own version:

```json
{
  "apiVersion": "edge.forms.takoform.com/v1alpha1",
  "kind": "ModuleWorker",
  "definitionVersion": "0.1.0",
  "schemaDigest": "sha256:..."
}
```

Validation accepts `<dns-like group>/<version>` rather than one fixed
constant. Third-party groups such as `forms.example.com/v1alpha1` are valid
FormRefs; trust in a publisher remains a separate policy fact and never enters
the FormRef.

The first official family is the **Edge Platform Family**,
`edge.forms.takoform.com/v1alpha1`: Module Workers with versions,
deployments, routes, domains and cron triggers; edge KV; object buckets;
SQLite databases; at-least-once queues with consumers; dense vector indexes.
Container platform, managed database, and eventing families are separate
groups defined later.

Every current Form Definition declares a `role` from a closed enum:

- `identity` — long-lived logical resource;
- `revision` — immutable implementation snapshot; update is forbidden;
- `deployment` — selects which revisions are active;
- `attachment` — connects a parent resource to external events or endpoints;
- `policy` — configuration changed independently of the parent.

Tooling enforces role rules mechanically: revisions are never updated in
place, deleting an attachment never deletes its parent, and policy state never
migrates into the parent identity.

## Consequences

- `forms.takoform.com/v1alpha2` remains the retained provider-v2 group; new
  families never reuse it.
- FormRef parsing, package verification, registries, and the provider accept
  namespaced groups and can carry multiple exact FormRefs for one kind name in
  different groups.
- Catalog and docs generation group by family.
- Publisher identity, package digest, and host support remain separate facts
  from the FormRef, exactly as before.

## Rejected alternatives

- **Keep one central group and encode the family in the kind name.** Rejected
  because it renumbers every Form whenever any family evolves and prevents
  third-party groups entirely.
- **One package per family.** Rejected because it widens the audited trust
  boundary; per-Form packages keep review, digesting, and revocation exact.
- **Encode the publisher in the FormRef.** Rejected because semantic identity
  and distribution trust must stay independently verifiable; a publisher
  change must not re-identify semantics.
