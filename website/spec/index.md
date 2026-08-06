# Specification

## Identity

An exact **FormRef** is the join of an API group, a kind, a definition version,
and a schema digest. Compatibility is never inferred from a kind name alone:
the same kind in a different epoch is a different contract.

| Surface | Identity |
| --- | --- |
| Current FormRef | `forms.takoform.com/v1alpha2` |
| Legacy FormRef | `forms.takoform.com/v1alpha1` |
| Current package envelope | `packages.forms.takoform.com/v1alpha3` |
| Provider | `v1.0.3` published · `v2.0.0` source candidate |

## Normative schemas

Published at `forms.takoform.com/schemas/...`:

- [form-ref v1alpha2](/schemas/v1alpha2/form-ref.schema.json)
- [form-definition v1alpha2](/schemas/v1alpha2/form-definition.schema.json)
- [package-index v1alpha3](/schemas/v1alpha3/package-index.schema.json)
- [host-api-wire v1alpha2](/schemas/v1alpha2/host-api-wire.schema.json)

## Lifecycle

A Form moves Proposal → Experimental → Stable → Legacy. Maturity is earned
from independent implementation and evidence, never from publication or
popularity. Every new Form starts from prior art (OCCI, CIMI, TOSCA,
Kubernetes/Crossplane, provider-native resources).

<div class="status-note">

Takoform is an **Experimental specification project**. Current FormRefs use
`forms.takoform.com/v1alpha2` and current package envelopes use
`packages.forms.takoform.com/v1alpha3`. Provider `v1.0.3` is the published
Legacy client; provider `v2.0.0` is an unpublished source candidate. The 34
published Form Package identities from `forms.takoform.com/v1alpha1` are
immutable Legacy evidence. There is no current central approval or admission.

</div>
