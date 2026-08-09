# 0030 — A Form line moves; a Terraform resource type may not

- Status: accepted
- Date: 2026-08-09
- Owners: Takoform maintainers

## Context

Decision [0017](0017-provider-state-survives-form-evolution-and-interruption.md)
settled what a moving Form line costs persisted STATE: the provider dispatches
on the exact FormRef recorded in state and carries one codec per supported
identity, so a resource created under an earlier definition version stays
addressable as itself. Decision [0011](0011-resource-identity-generation-and-revision.md)
settled that the Form semantic identity of a resource is its exact FormRef.

Neither settles what a moving Form line costs the Terraform SCHEMA, and the
schema cannot do what the codec does. There is exactly one
`takoform_worker_version` schema in a provider build. Every resource of that
type is decoded through it, every configuration is written against it, and the
configuration is source a user maintains and a reviewer reads. A codec absorbs a
definition change invisibly; a schema change is felt by every author at once.

The rule was implicit, which meant it was decided per change by whoever made
one.

## Decision

The rule is written down, and the mechanical half of it is enforced.

**The same Terraform resource type is kept** when every existing attribute keeps
exactly its meaning and the change is one of:

- adding an Optional attribute;
- adding a Computed attribute, a declared output
  ([0025](0025-declared-outputs-are-a-typed-contract.md)) included;
- relaxing validation;
- adding an enum value that breaks neither an existing host nor existing state.

**A new Terraform resource type is required** for:

- removing an attribute;
- changing an attribute's type;
- making an attribute Required;
- changing a declared output's type;
- changing the Form's lifecycle role;
- changing the identity or the replacement unit;
- any other semantic break.

A Form that breaks `takoform_worker_version`'s schema becomes
`takoform_worker_version_v2`, or a different Form kind. Both types then exist in
one build: the old one keeps serving the state written under it through its own
codec, and the new one is what new configurations write. Removing the old type
is a provider MAJOR, governed by "Provider versions are independent" in
[`../versioning.md`](../versioning.md).

Enforcement is split, because the rule is:

1. **A committed surface floor.** `internal/provider/testdata/v3-schema-baseline.json`
   records every v1alpha3 resource type, its schema version, and each
   attribute's type and Required/Optional/Computed/Sensitive flags. A test
   derives the current surface from the live schemas and refuses any difference
   that is not on the allowed list — naming the `_v2` type that would have to be
   minted instead. It is a FLOOR, not a snapshot: an additive change needs no
   edit and passes. The baseline is hand-committed and never rewritten by a
   gate, because a baseline a gate rewrites agrees with every change.
2. **A schema version and a registered state upgrader on every resource of the
   lane.** The schema version is what lets a resource type outlive a change to
   its own state layout, and a state upgrader is how state written at an earlier
   version becomes readable at the current one. Version 1 is the first numbered
   layout; the upgrade from version 0 is a whole-value carry, declared with a
   prior schema so the framework decodes the old value rather than a hand-written
   parse losing an attribute. The lane ships in provider v2.1.0, an unpublished
   source candidate, so no user state exists at version 0 outside development —
   registering the mechanism now is what makes it available on the day the
   upgrade is not an identity.
3. **Review.** Whether an attribute that kept its name, type, and flags kept its
   MEANING is not decidable by a test. The rule states it and review carries it.

The behavioural half is covered end to end against a synthetic second definition
version of one Form: a NEW resource is created under the current identity while
state recorded under the OLD one is refreshed, imported by its exact FormRef,
read for the outputs that definition published, and deleted — all through ONE
Terraform resource type.

## Consequences

- A Form line may advance additively without a provider surface change, which is
  what [0017](0017-provider-state-survives-form-evolution-and-interruption.md)
  promised and this makes checkable.
- A breaking Form major now has a named cost in the provider — a second resource
  type and a migration an author performs deliberately — rather than an implicit
  one discovered by users.
- The baseline is a file a reviewer reads. A change that edits it is a change
  that is claiming something about compatibility, and the diff says which.
- Every v1alpha3 resource carries schema version 1 from this decision onward.
  Fresh state is written at version 1; there is no user state at version 0 to
  migrate.

## Rejected alternatives

- **Pin the schema exactly and require a re-pin for every change.** Rejected
  because it makes the additive case — the common, permitted one — cost a
  file edit and a review argument, and because a gate that must be re-pinned
  constantly stops being read.
- **Derive the rule from the Form's own SemVer.** A Form minor is additive in
  the DESIRED contract, which is not the same claim as additive in the Terraform
  surface: the provider adds envelope attributes, recovery attributes, and typed
  outputs that no Form version describes. The surface has to be measured
  directly.
- **Version the resource type name on every Form definition version
  (`..._v0_1_0`).** Rejected because it makes every author's configuration
  churn on changes that cost them nothing, and it defeats the codec machinery
  that exists precisely so one type can serve several identities.
- **Leave the schema version at 0 and add an upgrader when one is first
  needed.** Rejected because the first time a state layout has to move is the
  worst time to discover the mechanism is absent, and because the cost of
  registering it while the lane is an unpublished candidate is zero.
