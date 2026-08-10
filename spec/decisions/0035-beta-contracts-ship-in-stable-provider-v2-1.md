# 0035 — Stable provider v2.1 ships the immutable Beta 1 contracts

- Status: Accepted
- Date: 2026-08-10
- Supersedes: [0013](0013-v1alpha3-lane-ships-in-provider-v2-1.md)

## Context

Decision 0013 assigned provider v2.1 to the then-current
`forms.takoform.com/v1alpha3` source lane. Those schema and specification
identities were subsequently published and are immutable, but the provider and
Edge Platform Family had not been published. Treating the provider version,
Host API channel, Form family channel, Form maturity, and package publication
as one release axis created an unnecessary all-or-nothing freeze.

The portable Host API and Edge Platform Family can be consumed by Takosumi as
one independent adopter before that product reaches its own GA. That adoption
does not make the contracts Stable, the Forms Stable, the package artifacts
published, or any hosted service GA. Takoform owns the maturity decision.

## Decision

### Independent version axes

These identities advance independently:

- provider release: stable SemVer `v2.1.0`;
- Host API channel: `forms.takoform.com/v1beta1`;
- Edge Platform Family channel: `edge.forms.takoform.com/v1beta1`;
- Form Definition versions: the 15 Forms remain Experimental `0.1.0`;
- package envelope: `packages.forms.takoform.com/v1alpha4`;
- Interface and Binding references:
  `interfaces.takoform.com/v1alpha1` and
  `bindings.takoform.com/v1alpha1`.

Provider v2.1.1 is the stable provider release targeting these Beta contracts
and is now Registry-published under its immutable release identity. The
provider descriptor stays `candidate-only` as metadata after owner publication;
that descriptor status is not Provider availability and is not SemVer
prerelease syntax.

### New Beta identities

The current Host API is discovered only at
`/.well-known/takoform/v1beta1` and uses API root
`/apis/forms.takoform.com/v1beta1`. New Form Definition, FormRef, Host wire,
discovery, operation-table, and normative Host API identities are minted for
Beta 1.

Every `v1alpha3` schema, specification, operation table, public URL, and byte
remains retained history. The old
`edge.forms.takoform.com/v1alpha1` candidate tree is also retained; generators
write the Beta family to a new path and never replace it.

### Provider identity and state immutability

[`release/provider-form-identities.json`](../../release/provider-form-identities.json)
is the append-only provider compatibility authority. Provider v2.1 embeds
exactly the 15 Beta FormRefs, Definition digests, package digests, and Terraform
resource type mappings. This identity commitment exists even while the package
artifacts remain unpublished.

A resource persists the whole Beta FormRef in state. Read, update, import,
observe, and delete dispatch on that exact tuple. A later stable `v1` family
may become the default for newly created resources, but refresh never rewrites
existing Beta state and the provider continues to carry its Beta identity and
codec. Migration is explicit create/import work.

### Beta compatibility

Published Beta 1 identities are immutable. An additive correction may add a
new exact Definition identity only where the Beta contract permits it. A
breaking Host/family correction mints `v1beta2` and new FormRefs. It never
changes a Beta 1 schema, definition, digest, route meaning, or persisted state
interpretation.

### Takoform-owned Stable promotion

The portable Host API and Edge Platform Family remain Beta until Takoform's
lifecycle authority records earned evidence for promotion. Stable `v1` and
Form `1.0.0` are new exact identities, not consequences of a provider release,
host availability, or a Takosumi product milestone. The promotion record must
contain independent-host, real-backend, runtime, third-party-ecosystem,
operational, migration, and recovery evidence required by
[`project-lifecycle.md`](../project-lifecycle.md). A Takosumi deployment may
be one independent adopter, but it cannot be the sole host/adoption evidence
or promotion authority. These obligations remain open without blocking
provider v2.1.

## Release gates

The provider v2.1 gate must prove:

1. the 15 Terraform resource schemas and names remain compatible;
2. generated Beta Definitions, FormRefs, package digests, provider registry,
   and provider identity ledger agree exactly;
3. the fake host and reference host pass the v1beta1 conformance corpus;
4. state recorded under every supported Beta FormRef remains exact, including
   after a synthetic future stable default is introduced;
5. retained published identities are append-only and old alpha bytes are not
   overwritten.

The stricter `bun run assert:publishable` continues to guard Form Package and
public-service publication. It must not be used to deny the provider-first path.

## Consequences

- Provider and API version numbers no longer imply one another.
- Beta is an API/family compatibility channel, not Stable Form maturity.
- Package availability and provider-embedded identity are separate facts.
- No hosted-service or Takosumi product milestone is a maturity decision; an
  independent production host remains an evidence obligation until Takoform
  records it.
- Decision 0013 remains in history as the superseded alpha-lane plan; it is not
  rewritten.
