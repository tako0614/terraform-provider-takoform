# 0025 — A Form's declared outputs are a typed contract, not a JSON blob

- Status: accepted
- Date: 2026-08-08
- Owners: Takoform maintainers

## Context

Every v1alpha3 resource reaches a host-computed value through one attribute:
`outputs_json`, the whole `status.outputs` document serialized to a string. An
author who wants a URL writes

```hcl
jsondecode(takoform_worker_endpoint.x.outputs_json)["url"]
```

where they should write `.url`. The expression is not merely verbose. It is
unchecked in both directions: a typo in the key evaluates to null rather than
failing, the value has no type, and nothing tells the author which keys exist —
so the shape of an output is learned from an apply rather than from the
contract.

The contract was always there. The published Form Definition schema declares
`outputSchema`, and the published wire schema already states the rule that
governs it: `status.outputs` validates against the installed Definition's
`outputSchema`, is REQUIRED when the Definition declares one, and is OMITTED
when it does not. No Form in the lane declared one, no host was ever held to
either half of that rule, and the provider had no reason to read it.

Two things follow, and they are different decisions with different blast
radii. What a Form's output contract IS — how it is declared, what it may
contain, and what a host must return — is an authoring and host rule that
applies to every Form, present and future. How the provider PROJECTS it is a
Terraform surface decision, and the surface is where this goes wrong in
practice: a computed attribute declared in the wrong mode, or left unset on a
recovery path, produces "Provider produced inconsistent result after apply" or a
perpetual diff, both of which surface far from the change that caused them.
Neither is settled by decision [0014](0014-published-schemas-are-structural-minima.md),
which only says where a new invariant may live. This repository's `AGENTS.md`
requires a decision record for normative changes; this is the one for the output
contract and its provider surface.

## Decision

A Form's outputs are a declared, closed, typed contract, and the provider
surfaces them as typed computed attributes. `outputs_json` is retained,
unnarrowed, as the escape hatch for what the contract does not describe. The
rules below are normative; the operative text lives in
`../host-api/v1alpha3.md`.

1. **Outputs are declared, and the declaration is closed.** A Form Definition's
   `outputSchema` is a Draft 2020-12 object with `additionalProperties: false`
   whose every declared member is `required`. Required, because an output a host
   may omit is one no consumer can use without inventing a fallback; closed,
   because an undeclared member is a value the contract never described, and a
   client that typed it would be typing one host's private extension as though
   it were portable. It carries data the published Form Definition schema
   already admits, so nothing is minted: this is rung 1 of the decision 0014
   ladder.

2. **An output is not desired state, and the authoring model refuses one that
   pretends to be.** An output declares no required flag, no default, no
   immutability, and no cross-resource reference. Each of those words describes
   what an author asks for, and an output is what the host answers: a default
   would name a value no host produced, an immutable flag would fence a value
   nobody writes, and a reference would make an output a relation — a thing this
   lane resolves, pins by UID, and protects from deletion, none of which a
   computed value has. An output is one closed scalar, so a consumer reads a
   typed value rather than a document it must decode again.

3. **A host returns exactly the declared members, or none at all, and each one
   validates.** For a Form declaring an `outputSchema`, `status.outputs` is
   present, carries exactly those members, and VALIDATES against that schema —
   the type, the anchored pattern, and the bounds the Form declares. For a Form
   declaring none, `status.outputs` is OMITTED — not an empty object. This is
   the published wire rule, now enforced: it is the required conformance check
   `form-declared-outputs-are-exact`, which drives both halves, because a host
   that returned an empty document everywhere would pass a check that only
   looked at the Form declaring outputs. The lane measures each value against
   the declared schema rather than against an assumption about it, because a
   check that only asked whether a non-empty string came back would accept
   `hostname: "not a hostname"` and reject the first integer output rule 2
   permits — a required check no incorrect host can fail and no correct host can
   pass, in one place.

4. **The provider generates one typed computed attribute per declared output.**
   It is derived from the declarations the `outputSchema` is rendered from, so
   the attribute surface and the schema cannot disagree. The attribute name is
   the output's HCL name; a name the resource surface already owns is refused at
   authoring time against a written list of reserved attributes, because one
   name holding two facts means whichever the resource writes last wins.

5. **Every output attribute is plain `Computed`.** Never `Optional+Computed`,
   and never carrying a framework default. That is what the value IS: a
   configuration cannot write an output, so `Optional` would advertise an
   argument the provider must then refuse, and a default would put a value in
   the plan that no host produced. It is exactly how `outputs_json`,
   `generation`, `revision`, and `ready` already behave. There is no
   `UseStateForUnknown` either: an output is what the address currently is, a
   change to the resource can move it, and holding the prior value known through
   such a plan would show an operator a value the apply may replace.

6. **Every state write sets every output attribute.** Including the write an
   accepted-but-unfinished mutation leaves behind, where the host returned no
   representation and every output is null. A Computed attribute left unset
   after an apply whose plan marked it unknown is "Provider produced
   inconsistent result after apply", and it is unset precisely on the recovery
   paths nobody exercises by hand (decision
   [0017](0017-provider-state-survives-form-evolution-and-interruption.md)).
   Outputs are re-read from the host on every refresh and never carried forward
   from prior state: a stale address would be reported as current with no plan
   able to correct it, because no desired attribute changed. Null belongs to
   that path and to no other. A response carrying `status` carries every
   declared output with its declared type (rule 3), so on an ordinary create,
   update, or refresh a missing or wrongly-typed one FAILS the write with a
   diagnostic naming the output and what was wrong. Recording it as a null
   instead would leave the practitioner holding an address-shaped hole — no
   endpoint, no error, and every expression reading it evaluating to null — with
   the host's fault nowhere on screen.

7. **The outputs a state ref declares are the recorded ref's, not this build's.**
   Output attributes are decoded through the same per-exact-FormRef codec every
   desired field uses, so a resource created under an earlier definition version
   publishes that definition's outputs (decision 0017).

8. **`outputs_json` keeps the WHOLE document.** It is not narrowed to the
   members no schema describes. Every value that has a typed attribute is still
   in the JSON document under its wire name, so an existing configuration that
   decodes it keeps working unchanged, and the typed attributes are a strictly
   additive surface an author adopts when they choose. What `outputs_json` is
   now FOR is the other case: reaching an output the Form's `outputSchema` does
   not describe. Removing it is a separate decision and a provider major.

`WorkerEndpoint` (decision
[0024](0024-a-worker-is-reachable-at-a-host-assigned-address.md)) is the first
Form to declare an output contract, and it is why this decision exists now
rather than later: an address a consumer cannot read is an address that does not
exist.

## Consequences

- The lane enforces a published wire rule it previously only stated. A host that
  publishes an outputs document for a Form that declares none, or omits one for
  a Form that declares one, now fails the corpus.
- The authoring model gains a written list of reserved resource attributes. It
  lives in the model rather than the provider so a Form is refused before any
  surface is derived from it, and a provider test proves the list is the
  provider's actual envelope surface — a list that drifted would admit exactly
  the collision it exists to prevent.
- Generated resource documentation gains an Outputs section for a Form that
  declares them, and the generated example reads the typed attributes rather
  than decoding JSON, because the example is where an author copies from.
- The `outputSchema` is not reachable from the wire. The published
  form-definition response is a closed object carrying identity, display name,
  description, and `desiredSchema`, and its bytes are immutable, so a client
  learns a Form's output contract from the Form Package it installs and the
  conformance corpus pins the WHOLE declared schema it holds a host to — the
  contract rather than its member names, because that is what the wire rule says
  a host's outputs validate against. The corpus carries it rather than the
  runner deriving it from the pinned Form identity, because the corpus is the
  digest-pinned artifact a host is measured against: a runner compiling its own
  copy of the catalog could hold two hosts to two contracts under one corpus
  digest. A repository test compares the pinned bytes against the installed
  Definition at the exact FormRef, so the two cannot drift. Serving the output
  contract over the API is a future schema generation, not a change to this one.
- Nothing about the retained v1alpha2 lane changes. Its Forms derive their
  output schemas from a different catalog, and its resource carries a different
  state surface.

## Rejected alternatives

- **Narrow `outputs_json` to the members no schema describes.** Rejected
  because it silently breaks every existing configuration that reads a
  now-typed key out of it: the expression keeps parsing, keeps evaluating, and
  starts producing null. A change that turns a working configuration into a
  quietly wrong one is worse than the verbosity it removes, and the escape hatch
  is more useful whole — one document, one meaning, no rule about which half a
  value landed in.
- **Remove `outputs_json` once typed attributes exist.** Rejected here, not
  forever. It is a breaking provider change and belongs to a major with its own
  decision; and an output no schema describes must stay reachable, so something
  of its kind has to survive in any case.
- **Read the `outputSchema` from the host at runtime and build attributes from
  it.** Impossible and undesirable. The published form-definition response
  carries no such member and its bytes are immutable; and a Terraform schema is
  fixed at provider build time, so a schema that depended on a host response
  could not be produced before the provider is configured.
- **Type the outputs as one object attribute instead of one attribute each.**
  Rejected because it reintroduces the indirection the change removes: an author
  would write `.outputs.url` instead of `.url`, and every nested member would
  still need the same mode and default reasoning while being harder to document
  and harder to deprecate one member at a time.
- **Let outputs be optional in the schema.** Rejected because "sometimes
  returned" is not a contract a consumer can use. Every consumer would write the
  same fallback, each one guessing differently at what a missing value means,
  which is the least-common-denominator behavior decision 0008 rejects.
- **Make the output attributes `Optional+Computed` so an author can override
  them.** Rejected because there is nothing to override. An output is the host's
  answer, so a written value would either be ignored — a configuration that lies
  — or sent, which would make it desired state under another name.
