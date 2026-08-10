# Specification

## Identity

An exact **FormRef** is the join of an API group, a kind, a definition version,
and a schema digest. Compatibility is never inferred from a kind name alone:
the same kind in a different epoch is a different contract. Form groups are
namespaced into [Form Families](/spec/form-families.html); the Host API group
is a protocol compatibility identity independent of any nested Form group.

| Surface | Identity |
| --- | --- |
| Current Form Family | `edge.forms.takoform.com/v1beta1` (Beta family; 15 Experimental `0.1.0` Forms) |
| Current Host API wire | `forms.takoform.com/v1beta1`, discovered at `/.well-known/takoform/v1beta1` |
| Current package envelope | `packages.forms.takoform.com/v1alpha4` |
| Retained provider-v2 FormRef | `forms.takoform.com/v1alpha2`, wire discovered at `/.well-known/takoform/v1alpha2` |
| Retained provider-v2 package envelope | `packages.forms.takoform.com/v1alpha3` |
| Legacy FormRef | `forms.takoform.com/v1alpha1` (frozen) |
| Provider distribution | **Provider 2.1.1** Registry-published stable distribution (descriptor `candidate-only` metadata by design) · **Provider 2.0.0** published compatibility predecessor · **Provider 1.0.3** Legacy |

The `forms.takoform.com/v1alpha2` epoch and its nine `0.1.0` candidates are
retained provider-v2 preview source; they are not the basis for new
specification work. The provider's SemVer is independent of every API
identity.

The 34 published Form Package identities are immutable Legacy history. There
is no current central approval or admission derived from that historical
package count.

## Current-lane contracts

- [Form Families](/spec/form-families.html) — namespaced Form groups and the
  Edge Platform Family
- [Host API v1beta1](/spec/host-api/v1beta1.html) — uid/generation/revision
  identity, long-running operations, fencing
- [Interface contracts](/spec/interface-contract/) — exact capability
  contracts a Form's service exposes
- [Binding contracts](/spec/binding-contract/) — typed outward capability use
  held by revisions
- [Artifact transport](/spec/artifact-transport/) — content-addressed
  artifact upload and manifests

## Normative schemas

Published at `forms.takoform.com/schemas/...`. Current Beta identities:

- [form-ref v1beta1](/schemas/v1beta1/form-ref.schema.json)
- [form-definition v1beta1](/schemas/v1beta1/form-definition.schema.json)
- [host-api-wire v1beta1](/schemas/v1beta1/host-api-wire.schema.json)
- [package-index v1alpha4](/schemas/v1alpha4/package-index.schema.json)

Retained provider-v2 compatibility schemas:

- [form-ref v1alpha2](/schemas/v1alpha2/form-ref.schema.json)
- [form-definition v1alpha2](/schemas/v1alpha2/form-definition.schema.json)
- [package-index v1alpha3](/schemas/v1alpha3/package-index.schema.json)
- [host-api-wire v1alpha2](/schemas/v1alpha2/host-api-wire.schema.json)

The v1alpha3 schema identities remain immutable retained history and are never
rewritten as Beta.

## Lifecycle

A Form moves Proposal → Experimental → Stable → Legacy. Maturity is earned
from independent implementation and evidence, never from publication or
popularity. Every new Form starts from prior art (OCCI, CIMI, TOSCA,
Kubernetes/Crossplane, provider-native resources).

## More surfaces

- [Proposals](/proposals/) — mutable design material for Forms that have not
  earned a public FormRef
- [Form inventory](/forms/) — generated candidate and family inventory
- [Conformance](/conformance/) — executable compatibility evidence
- [Release](/release/) — provider and Form Package publication boundary
- [Glossary](/docs/glossary.html) — terms used across the documentation

<StatusNote />
