# 0037 — Immutability begins at Stable; a pre-Stable identity may be withdrawn

- Status: Accepted
- Date: 2026-08-22

## Context

[`versioning.md`](../versioning.md) promised that "every released `0.x` identity
remains immutable". Read as "never overwritten" that rule is right and stays.
Read as "served forever" it committed a pre-Stable project to carrying every
generation it had ever produced, and that is what it was being read as.

The cost is visible. Four Host API lanes, four Form groups, four package
envelopes and three provider lines are alive at once; 169 published documents
declare ten different lanes; the same `vN` shape appears on seven independent
axes so that the word "v3" does not identify anything without saying which axis
it belongs to. None of that is an accident of naming — it is the retention rule
working exactly as written, applied to material that never earned it.

The repository's own records say the material never earned it:

```
forms/candidates/v1alpha2/candidate-set.json   publicationStatus: "unpublished"
forms/lifecycle.json                           currentForms: []
forms/README.md   "none is published, Experimental, Stable, centrally approved,
                   or guaranteed commercially available"
```

`currentForms` has been empty in every revision since the file was created. The
nine retained `forms.takoform.com/v1alpha2` Forms are Proposal-derived
candidates that never reached Experimental, and Stable-grade permanence was
being spent on them.

Three facts decide how much a withdrawal actually costs, and all three were
measured rather than assumed:

- **A released provider does not fetch what would be withdrawn.**
  `formpackage/schema.go` embeds the schemas with `go:embed`; nothing in
  `internal/provider`, `internal/client`, or `internal/clientv3` requests
  `https://forms.takoform.com/schemas/...` at runtime. An installed provider is
  self-contained.
- **The bytes are not destroyed.** They stay in this repository's history and
  under 78 immutable `forms/*` tags. Removing something from the working tree
  and the served site is not the same act as destroying it.
- **Nothing needs to be unpublished from the Terraform Registry.** Provider
  `v1.0.3`, `v2.0.0` and `v2.1.1` are immutable there, keep working, and are
  unaffected.

## Decision

**Permanence begins at Stable.** A Stable identity is served for as long as the
project exists, under the rules already written. A Proposal or Experimental
identity carries no such promise and MAY be withdrawn.

**"Never overwritten" is unchanged and has no exceptions.** While an identity is
served it means exactly what it meant when it was published. A breaking
correction mints a new identity. This is the rule that makes an exact FormRef
worth recording in client state, and withdrawal does not touch it.

**A withdrawal is a recorded act, never a silence.** The identity moves to the
`retired` list of the ledger that published it, keeping the bytes and the lane
it was published with, and saying why. The ledger therefore still answers what
that address meant after it has stopped answering. Three properties follow, and
each is enforced rather than described:

- a writer will not drop a published address on its own — regenerating from
  whatever happens to be on disk is the laundering these ledgers exist to
  prevent, so `sync:published-document-lanes` refuses and names what it would
  have forgotten;
- a withdrawn address may not be reused. Publishing something else at it fails
  by name, because the danger a reader faces is not a `404` but a URL they know
  that quietly means something new;
- a withdrawal may not restate history. The retired record must carry the same
  bytes the identity was served under, so retirement cannot launder a change
  that "never overwritten" would have refused.

## Enforcement

`release/published-document-lanes.json` and `release/public-schema-identities.json`
each gain a `retired` list, and their checks gain the three refusals above.
`scripts/deploy.mjs` passes the retired set into
`enforceAppendOnlyPublicSchemaIdentities` at both call sites, so the full-history
append-only walk admits a recorded withdrawal and nothing else.

Both were driven backwards before being trusted: an unrecorded removal fails, a
`--write` that would forget an address fails and names it, a recorded withdrawal
passes, a withdrawn address that is published again fails, a withdrawal under
different bytes fails, and an identity listed as both served and retired fails.

[Decision 0036](0036-artifact-names-state-the-lane-they-describe.md) is
unaffected. It governs what a name says, not how long the thing it names is
served, and the corpus-naming gate it added still applies to whatever remains.

## Consequences

Retiring the pre-Stable generations becomes possible, which is the point: the
project can carry one live generation instead of four, and "which one is
current" stops being a question a reader has to answer by inference.

What this decision does not do is decide which generations go. It makes
withdrawal expressible and auditable; each withdrawal is still its own reviewed
change, and each has to say in its `retiredBecause` why the address stopped
answering.

The rule is asymmetric on purpose. Reaching Stable is a commitment to serve an
identity indefinitely, and that commitment should be made once, deliberately,
about contracts that earned it — not by default, in advance, about every
candidate the project ever built.
