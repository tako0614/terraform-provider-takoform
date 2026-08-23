# 0049 — A Form versions alone: the family group carries no version, and constraints leave the schema

- Status: Accepted
- Date: 2026-08-23

## Context

The maintainer directed two changes together (2026-08-23): *abolish the
version on the whole, and make it each Form's own version*; and, of the
mechanism annotations, *this does not feel like a good specification — why not
a REST API that defines the paths and everything except the Form-specific
parts?*

Both are answered by measurement rather than taste.

### The family group's version has never carried information

A Form's exact identity is `(apiVersion, kind, definitionVersion,
schemaDigest)`. The digest already distinguishes every contract from every
other; `definitionVersion` already orders them; and since
[decision 0047](0047-the-host-api-is-the-substrate-a-form-declares-against.md)
the substrate dependency is stated by `requiresHostApi`. The version inside the
GROUP adds nothing to that tuple.

What it did do is force generation moves. The continuity ledger measured both:
`v1alpha1 → v1beta1` re-identified 15 Forms and changed **3**;
`v1beta1 → v1beta2` recorded 15 changed and 0 re-identified, and that 15 is
itself the evidence — the whole family counted as changed because one shared
reference moved, not because fifteen contracts did. In neither move did the
group version tell a reader something the exact FormRef had not.

A version that adds no information and forces every member to move together is
ceremony with a cost.

### The behavioural annotations are a constraint language in a schema's clothes

[Decision 0048](0048-the-protocol-states-mechanisms-not-forms.md) took the
family out of the protocol by having Definitions declare mechanisms. Counted
against the Definitions that actually instantiate them, those annotations are
three different things:

| Kind | Annotations | Instantiations |
| --- | --- | --- |
| Reference resolution | `target-formrefs`, `required-interface`, `binding`, `standard-services`, `required-entrypoint` | 3, 7, 1, 1, 4 |
| Behavioural constraint | `exclusive`, `sum`, `claim`, `host-assigned` | 5, 1, 1, 1 |
| Declared and unused | `relation-extras` | 0 |

The first kind belongs where it is: it annotates the exact node that IS the
reference, and moving it elsewhere would mean carrying a JSON Pointer back to
that node. The second kind is not about the shape of a document at all — it is
a rule about resources, riding in a schema's extension slots where no standard
validator will ever see it. The third is vocabulary nobody instantiates.

An earlier draft of this record counted `equals` and `omittedWhen` in that
third row. That was a measurement taken in the wrong place: both live in the
WIRE schema, where they carry envelope mirroring and conditional members, and
counting Form Definitions is not where a wire annotation would ever appear.
They are used, they stay, and the correction is recorded rather than quietly
applied — a count is only evidence if it counted the right thing.

## Decision

**`forms.takoform.com/v1beta4` is minted, and in it a Form versions alone.**

1. **A Form Family group carries no version.** `edge.forms.takoform.com`, not
   `edge.forms.takoform.com/vN`. A Form's identity is its group, its kind, its
   own `definitionVersion`, and its `schemaDigest` — which is what it always
   was, minus a member that never varied independently. There is no such thing
   as a generation move, so nothing can be moved in lockstep by one.

2. **Behavioural constraints leave the desired schema** and become a
   first-class `constraints` list on the Form Definition, with a closed
   grammar the lane states. Adding a constraint kind is then one reviewed
   change in one place, rather than a new `x-` key that may appear anywhere in
   a schema tree. The desired schema goes back to being plain JSON Schema that
   any validator reads completely.

3. **Reference-resolution annotations stay in the schema**, for the reason
   they were put there: they qualify the node they sit on.

4. **The unused annotation is removed.** `x-takoform-relation-extras` is
   emitted by nothing, read by nothing, and has no field on the model, while
   the lane document described it as the mechanism behind the class-selecting
   gate — which actually works through a plain sibling property. Vocabulary
   with no instantiation is a promise nobody has kept, and one described as
   load-bearing is worse than one merely unused.

## What this does not do

It does not renumber or withdraw anything already minted — but which
generations that covers was measured after the first draft asserted it, and the
answer is narrower. Against takoform.com: the v1beta1 candidate set and
Definition profile answer **200**; the v1beta2 ones answer **404**. Only
v1beta1 is published, so only v1beta1 has a meaning
[decision 0037](0037-immutability-begins-at-stable.md) protects.

**The v1beta2 generation therefore BECOMES the versionless group** rather than
being frozen beside a third tree. Nothing is served under its name, so nothing
is being changed out from under a consumer; minting a third family tree to
avoid touching an unpublished one would have been ceremony of exactly the kind
this record exists to remove.

It does not make the protocol smaller by pushing rules onto hosts. A second
implementer still reads one closed constraint grammar rather than one family's
prose per family — which is the property the annotations bought and this record
keeps, while moving them somewhere a specification can be read.

## Consequences

A Form moves when its own contract moves, and never because a sibling's did.
The continuity ledger stops measuring generations and starts measuring Forms,
which is what it was always deriving anyway.

The cost is one more lane and a re-identification of every Form that adopts the
versionless group — paid once, against a version line that would otherwise
force the same cost on every future move.
