# 0031 — Host capability is decided at plan time

- Status: accepted
- Date: 2026-08-09
- Owners: Takoform maintainers

## Context

A conforming host may implement `WorkerVersion` and the `edge.kv` binding while
implementing none of bucket, SQLite, or queue. Nothing about that is unusual:
[0013](0013-v1alpha3-lane-ships-in-provider-v2-1.md) made Host Support Profiles
a readable API surface precisely so a host can say which exact FormRefs,
Interfaces, and Bindings it carries, and
[0016](0016-the-worker-aggregate-has-one-active-deployment.md) and
[0024](0024-a-worker-is-reachable-at-a-host-assigned-address.md) both name
`unsupported_capability` (422) as the refusal a host raises for a capability it
does not offer.

The provider never read any of it. An author wrote a bucket binding, planned
cleanly, and discovered at APPLY — after the bundle bytes had been uploaded and
part of the aggregate created — that the host had no buckets. The information
was published, in a document the provider already had a client for, and the
plan simply did not ask.

## Decision

The provider reads the Host Support Profile surface during `ModifyPlan` and
decides capability there, transparently, for every v1alpha3 resource.

**Transparently, not through a data source.** `data "takoform_host_support"` was
the alternative. It was rejected because a data source REPORTS and a plan has to
DECIDE: an author who never writes the data block keeps discovering the
unsupported binding at apply, which is the whole defect, and an author who does
write it has to restate every Form's own requirements as `precondition` blocks —
the same facts a third time, in HCL, by hand, on every worker. Both sides of the
comparison are already declared: the catalog Form names its Interfaces, its
Bindings, its enums and its ranges, and the profile names what the host
implements. Comparing two declarations the provider already holds needs no
configuration at all.

**What the plan decides**, per resource:

- is this exact FormRef supported, and does the profile's `operations` cover the
  whole lifecycle the resource needs (create, read, delete, and update where the
  Form declares one);
- is each Interface the Form states in `providedInterfaces` implemented, read
  from `{api}/support/interfaces/{name}/{version}`;
- is each Binding contract the configuration actually USES implemented, read
  from `{api}/support/bindings/{name}/{version}` — only the ones used, so a
  KV-only worker is never refused for a bucket contract it does not declare;
- does every planned closed-enum value fall inside `supportedEnums`, and every
  planned integer inside `supportedRanges`;
- does every planned collection stay inside the ceiling the profile publishes
  for it, read by the profile schema's own lowerCamelCase convention
  (`maximumHandlers` bounds `handlers`, `maximumVersions` bounds a deployment's
  `versions`, `maximumRequiredSensitiveVars` bounds the secret slots a version
  asks for);
- does the artifact this plan would commit stay inside `maximumBundleBytes`,
  checked before a byte is uploaded.

Whether endpoint assignment is offered, whether a queue is offered, and whether
a secret slot is offered all fall out of the first rule applied to the resource
that needs it, rather than being separate special cases.

**A host that says nothing is not a host that says no.** The published profile
schema lets a profile omit everything but identity and operations, so reading
omission as denial would refuse conforming hosts. Only an explicit refusal —
`form_unknown` or `resource_not_found` on a support route — is an error. An
unreadable support surface is a WARNING that names what could not be decided,
and apply remains the backstop it already is. A provider must not refuse an
apply because it failed to ASK.

**Three planes, kept apart, and said so.** Form semantics belong to Takoform:
what a `WorkerVersion` means and which values its fields admit are properties of
the exact FormRef, identical on every host. Host capability belongs to the Host
Support Profile: which of those a THIS host implements. Capacity, price, region,
and SLA belong to a Service Offering and are not in this API at all — the
published profile schema admits none of them, and the provider would ignore one
if a host sent it. No host-specific value enters a Form's desired state as a
result of any of this: the plan either proceeds with the desired state the
author wrote, or refuses it.

Each profile is read at most once per provider configuration and cached, because
a profile is a static statement about one host and a plan asks the same
questions once per resource.

## Consequences

- `terraform plan` now makes a small number of read-only host requests, bounded
  by the existing 30-second plan preview deadline. A slow or absent support
  surface costs a warning, never a failed plan.
- An author on a partial host sees the refusal before any artifact is uploaded
  and before any resource is created, naming the exact Form or contract and the
  fact that the same configuration applies unchanged on a host that implements
  it.
- The client gains `GetInterfaceSupport` and `GetBindingSupport`; the Form
  support read already existed and was unused.
- A host that publishes a ceiling gets it honoured with no client change,
  because the ceiling names are read by convention from the field's own wire
  name rather than from a list in the provider.

## Rejected alternatives

- **A `data "takoform_host_support"` source.** See above: it reports where the
  plan has to decide, and it makes correctness opt-in per configuration.
- **Refuse whatever the profile does not explicitly permit.** Rejected because
  the published profile schema makes `supportedEnums`, `supportedRanges`,
  `supportedBindings`, and `limits` all optional. A host that publishes only
  identity and operations is conforming, and treating its silence as denial
  would make the provider unusable against it.
- **Read the profile once at Configure and hold it for the whole run.** Rejected
  as premature: Configure runs before the plan knows which kinds are in play, so
  it would have to read every profile a build knows about on every command,
  including `terraform output`. Reading lazily and caching costs the same
  requests for a plan that uses them and none for a command that does not.
- **Check capability at apply, but earlier in the apply.** Rejected because a
  plan the author approved is the artifact a review reads. A refusal that only
  ever appears after approval has already cost the review its meaning.
