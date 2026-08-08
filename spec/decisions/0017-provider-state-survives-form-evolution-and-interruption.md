# 0017 — Provider state survives Form evolution and interruption

- Status: accepted
- Date: 2026-08-08
- Owners: Takoform maintainers

## Context

Decision [0011](0011-resource-identity-generation-and-revision.md) settled that a
resource's state identity is `space/apiVersion/kind/uid` and that its Form
semantic identity is the exact FormRef, and decision
[0013](0013-v1alpha3-lane-ships-in-provider-v2-1.md) promised provider v2.1 "a
multi-FormRef registry dispatching on the exact FormRef recorded in state". The
provider could not honour either promise, because everything it dispatches on is
keyed too coarsely or consulted too late.

The generated registry keys a FormRef by `apiVersion + "/" + kind`, so one Form
line has exactly one identity at a time. The moment a Form's definition version
moves, every resource already in state records a FormRef the build no longer
holds, and the dispatch seam that was supposed to absorb that
(`v3FormResource.supportedRefs`) was an unexported, test-only field that no
production construction ever filled. A ref alone would not have been enough
anyway: an older definition may have declared a different field set, so decoding
state written under it needs that definition's declarations, not the current
schema's.

Import made the same mistake in the other direction. The typed `ImportState`
seeded the DEFAULT create ref, so importing a resource created under an earlier
definition version silently rebound it to a contract it was never applied under.
The generic carrier had no `ImportState` at all, on the stated grounds that a
`NAME` / `SPACE/NAME` string cannot carry the four exact FormRef members — true
of that string, and taken as if it were true of import itself.

Two more paths removed resources from Terraform's management without a way back.
When a read found a different UID under the same name, it removed the resource
from state; the host still held the new incarnation, so the next apply fenced on
`If-None-Match: *`, failed, and left no plan that repaired anything. And
`pending_operation_id` — written precisely so an accepted-but-unfinished mutation
is not orphaned — was never consulted: a read went straight to the resource GET,
so on a host where the resource does not exist until its operation commits, a
truthful 404 during that window was read as deletion and the next apply created a
duplicate.

Decision [0015](0015-cross-resource-references-are-uid-pinned-relations.md)
already settled the shape of the answer for one neighbouring case — a relation
whose target changed incarnation is reported, never re-bound, and never allowed
to fail the refresh that computes its repair. This decision applies the same
reasoning to the resource itself and to the operation that created it. These are
MUST-level provider and host semantics, so this repository's `AGENTS.md` requires
a decision record.

## Decision

Provider state is bound to one EXACT Form identity and one host incarnation, and
a refresh never silently changes either. The rules below are normative; the
operative text lives in [`../host-api/v1alpha3.md`](../host-api/v1alpha3.md),
[`../versioning.md`](../versioning.md), and the generated provider resource
documents.

1. **Dispatch on the recorded ref.** Read, update, and delete address the
   resource under the exact FormRef recorded in state. Create — and only
   create — uses the default create ref. Membership in the supported set, not
   equality with the default, decides whether state can be served.

2. **The registry is keyed by the whole identity.** The generated data is two
   maps: `Supported`, keyed by the complete
   `{apiVersion, kind, definitionVersion, schemaDigest}` tuple, holding every
   identity the build can read, observe, update, and delete; and
   `DefaultCreates`, keyed by `{apiVersion, kind}`, naming the one identity a NEW
   resource of that line is created under. The package digest stays inside the
   value as audit evidence and is excluded from the key, because it is
   distribution provenance and never identity (decision 0011). A registry
   snapshot is immutable; registering an identity returns a copy.

3. **Each supported ref carries a codec.** A codec is the field set that exact
   definition declared. It decodes the state written under that ref and encodes
   the spec sent for it, so an update to a resource created under an earlier
   definition version is a mutation of that contract rather than a migration onto
   the current one. The compatibility rule is the Form compatibility rule of
   [`../versioning.md`](../versioning.md), read from the provider's side: an
   ADDITIVE Form minor may share one codec with the definitions before it,
   because every previously valid desired document stays valid with the same
   portable meaning; a BREAKING Form major needs its own codec, because a
   removed, retyped, or re-meant field cannot be encoded or decoded by the other
   definition's declarations.

4. **Fail closed on an identity the build cannot decode.** State bound to a ref
   with no compiled codec produces a hard error naming the recorded ref and every
   ref of that kind the build knows. The provider MUST NOT read, update, or
   delete the resource under a different exact FormRef. Substituting one would
   reinterpret state written against one contract as another, and — worse — the
   substituted query's `resource_not_found` would then look like deletion.

5. **Import names the exact identity.** The canonical import ID is one JSON
   object, `{"space", "apiVersion", "kind", "definitionVersion", "schemaDigest",
   "name"}`, with `space` optional and the four FormRef members all-or-nothing.
   The short `NAME` and `SPACE/NAME` forms are retained and resolve to the
   default create ref. The generic carrier accepts the JSON form only, because it
   has no default create ref to resolve a short form against. An import naming an
   identity the build cannot decode is refused rather than rebound.

6. **A UID mismatch is an error that preserves state.** When a read finds a
   different `metadata.uid` under the name state records, the provider emits a
   hard error naming the expected and current UIDs and the three remedies —
   import the new incarnation explicitly, restore the prior one, or delete the
   host-side replacement — and keeps the resource in state. It never re-binds and
   never removes. This is deliberately stricter than the relation drift of
   decision 0015, which warns and offers a repairing apply: there, the resource
   itself is still the one state names and an apply re-pins the reference; here,
   no apply the provider can send converts one incarnation into another, so the
   operator must choose.

7. **A read resumes a pending operation before it reads the resource.** When
   state carries `pending_operation_id`, the operation is consulted first, and
   what the following resource read may conclude is:

   | operation state       | the read may then conclude                                                                                             |
   | --------------------- | ---------------------------------------------------------------------------------------------------------------------- |
   | still running         | absence is NOT deletion; a readable representation settles state but the marker survives, because nothing has committed  |
   | terminal, success     | the result resource is verified against the exact identity and its UID checked; the ordinary read settles and clears     |
   | terminal, error       | the exact resource GET is the final word: absent means state may be removed, present means the UID decides              |
   | `operation_not_found` | the record expired; the exact resource GET is the final word, under the same UID rule                                    |

   No branch re-binds by name alone: a known UID is verified in every branch, and
   an unknown one is adopted only from a representation the host served under the
   exact FormRef state records.

8. **The host side is proven, not assumed.** Two required conformance checks
   carry what rules 1 and 7 depend on: `operation-resumable-after-settlement`
   proves a recorded operation stays addressable and replays its terminal state
   byte-identically after the resource it created has been updated and a second
   operation has run; `exact-form-ref-fails-closed-on-unknown-definition` proves
   an exact FormRef query naming a definition version or schema digest the host
   does not have fails `form_unknown` on availability, on the Form Definition,
   and on a resource read, instead of matching by group and kind.

## Consequences

- A Form line may advance without a state migration. Adding the new definition
  to `Supported` keeps every existing resource addressable as itself; moving
  `DefaultCreates` changes only what new resources are created under. The two are
  now separate acts rather than one entry that means both.
- The exact-FormRef test seam is gone. Production and the multi-version tests run
  the same registry type through the same code path; the tests differ only in
  registering a synthetic second definition version, which is also what makes the
  eleven-entry generated data provably capable of more.
- A refresh can now end in a hard error that leaves the resource in state. That
  is a deliberate reversal of the usual provider instinct to "heal" a refresh: a
  refresh that heals by forgetting produces an apply that cannot succeed, and the
  operator has no way to see why.
- `pending_operation_id` becomes load-bearing rather than informational, so it is
  cleared only by an operation that actually settled. A read taken while the
  operation is still running keeps it, which means an interrupted create is
  resumable across as many refreshes as the host needs.
- The conformance corpus grows to 68 required checks, and hosts that pass the
  lane today must additionally keep operation records addressable past the next
  operation.

## Rejected alternatives

- **Keep the registry keyed by group+kind.** This is the current shape, and its
  cost is exactly the blocker: one Form line can hold one identity, so the day a
  definition version moves, every existing resource's recorded FormRef becomes
  unknown to the build at once. The only recoveries would have been to reinterpret
  that state under the new ref (rejected below) or to require every user to
  destroy and re-create their resources — a migration forced by the provider's
  data structure rather than by any change in what the resources are.
- **Rebind state to the default ref on read.** Rejected because it is a silent
  claim, made by the party least entitled to make it, that two contracts are the
  same. The read would succeed, state would afterwards name a definition the
  resource was never applied under, and every later update would send that
  definition's field set. Where the substituted identity is not one the host
  serves for that resource, the failure is worse than silent: the query returns
  `resource_not_found`, the provider reads it as deletion, and the resource is
  dropped from management. Failing closed costs an error message; rebinding costs
  the ability to tell which contract a resource is under.
- **Remove state on a UID mismatch.** This is what the lane did, and it is the
  one remedy that removes its own path. The host still holds the replacement, so
  the next apply fences on `If-None-Match: *` and fails; the plan that would have
  repaired anything was computed from a state that no longer has the resource.
  The operator is left editing state by hand — outside Terraform, which is where
  a provider's error handling is supposed to keep them from going. Preserving
  state and naming the three remedies leaves every option open, including the
  removal, which the operator can still perform deliberately.
- **A delimiter-joined import ID carrying the exact FormRef.** Rejected because
  no delimiter is safe. A SpaceID is opaque, case-sensitive UTF-8 whose only
  forbidden character is `/` (decision 0003), so every separator a reader could
  type may legitimately appear inside the space, and any escaping convention has
  to survive copy-paste through a shell that also interprets it. A JSON object
  names each member, needs no positional convention, and can be copied straight
  out of a resource representation.
- **Poll the operation to completion inside the read.** Rejected because a read
  is Terraform's refresh: blocking it for the length of a provisioning operation
  makes `terraform plan` hang for as long as the host takes, and a cancelled plan
  then loses the very information the poll was gathering. Consulting the
  operation once, reporting what it says, and keeping the marker leaves the
  resource under management and lets the next refresh finish the job.
- **Treat `operation_not_found` as "the mutation failed".** Rejected because the
  lane explicitly permits an operation record to expire while the resource it
  created lives on. Reading an expired record as failure would delete a healthy
  resource from state on nothing more than the passage of time.
