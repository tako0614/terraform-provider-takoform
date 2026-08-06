# 0002 — Artifact URLs are credential-free persisted state

- Status: accepted
- Date: 2026-07-29
- Owners: Takoform maintainers

## Context

Artifact-backed Forms expose `artifact_url` as a nonsensitive provider
attribute and retain it in Terraform/OpenTofu state. The general HTTPS grammar
used by OIDC redirect URIs permits query and fragment components. Reusing that
grammar for artifact fetching allowed signed or token-bearing query material
to enter nonsensitive state.

Artifact bytes are already bound by SHA-256. Fetch authorization belongs to
the host and does not need to travel in portable desired state.

## Decision

Artifact URLs use a distinct credential-free HTTPS grammar. It accepts an
absolute HTTPS URL with a dotted hostname, optional port, and optional path,
and rejects userinfo, query, and fragment. Catalog JSON Schema, provider
schema validation, runtime conversion, conformance fixtures, examples, and
resource documentation derive from that same grammar.

The general HTTPS grammar remains available to redirect URI declarations and
retains its existing acceptance surface.

Tightening an existing desired schema is a breaking Form change. The six
artifact-backed candidates therefore advance to new majors:

- `EdgeWorker@3.0.0`
- `ComputeInstance@2.0.0`
- `StaticSite@2.0.0`
- `Workflow@2.0.0`
- `StatefulEntity@3.0.0`
- `ModelEndpoint@3.0.0`

Earlier release-source identities remain unchanged.

## Consequences

- Artifact fetch credentials stay behind the host credential boundary.
- Nonsensitive provider state cannot retain URL userinfo, query, or fragment.
- Each artifact-backed package carries explicit negative fixtures for all
  three forbidden URL components.
- Provider `v1.0.0`, Form versions, and admission generation remain
  independent.

## Rejected alternatives

- **Mark `artifact_url` sensitive while retaining credential-bearing URLs.**
  Rejected because sensitive state still stores the value and portable
  desired state would still cross the host credential boundary.
- **Tighten the shared HTTPS grammar.** Rejected because the redirect-URI
  contract is a separate surface and narrowing it is outside this decision.
- **Rewrite the existing Form versions.** Rejected because release-source
  identities are immutable and the constraint is a SemVer-major change.
