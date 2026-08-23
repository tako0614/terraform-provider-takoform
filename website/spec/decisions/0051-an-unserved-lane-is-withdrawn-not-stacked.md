# 0051 — An unserved lane is withdrawn into its successor, not stacked beside it

- Status: Accepted
- Date: 2026-08-23

## Context

[Decision 0049](0049-a-form-versions-alone.md) minted
`forms.takoform.com/v1beta4` and removed the version segment from the Form
Family group. Implementing it surfaced a constraint the record did not know
about, and the constraint decides something the record left open.

There is exactly one current family. `currentformregistry.V3Current()` is
generated from the candidate tree, and `LoadCatalog` refuses any Form the
registry does not carry, so **every runnable corpus installs the same family**.
A lane whose wire schema cannot describe that family therefore cannot be run at
all — not "runs with fewer checks", but has no corpus it can execute.

That was measured rather than reasoned about. With the family versionless, a
v1beta2 self-test answers HTTP 400 on the first authenticated read, because the
group pattern in that lane's wire schema requires a `/vN` segment the group no
longer has.

So the three lanes could not coexist. The available moves were: keep a second,
versioned family tree for the older lanes — which decision 0049 forbids in as
many words; widen the older lanes' grammars in place — which would leave three
lanes accepting the same thing and nothing left for v1beta4 to be minted for;
or withdraw them.

The measurement that settles it is what is actually served. Against
takoform.com: `forms.takoform.com/v1beta1`'s corpus, wire schema and FormRef
schema answer **200**; every v1beta2 and v1beta3 identity answers **404**.
Neither lane was ever served, so neither has a consumer whose expectations a
withdrawal could break.

## Decision

**`forms.takoform.com/v1beta2` and `forms.takoform.com/v1beta3` are withdrawn
into `forms.takoform.com/v1beta4`.** Their documents, wire schemas, discovery
schemas, operation tables and corpora are removed; their identities stay in the
lane ledger as retired, so neither name can ever be reused meaning something
else; and everything they stated is stated by v1beta4, which is the only lane
this project asks a host to implement today.

This is the same move [decision 0042](0042-the-pre-beta-epochs-are-withdrawn.md)
made for the pre-Beta epochs and the same sentence decision 0049 used for the
family tree, applied to the thing it was next true of: *minting a fourth lane
beside two unpublished ones is ceremony of exactly the kind that record exists
to remove.*

**Two lanes remain**: `v1beta1`, published and retained, and `v1beta4`, current.

## What this does not do

It does not discard what v1beta2 and v1beta3 were minted for. The Beta 2
hardening review's wire changes (decision 0046) and the mechanism declarations
(decision 0048) are v1beta4's, unchanged and still measured — 124 required
checks, the same list the withdrawn corpus carried, plus what this lane adds.
A withdrawn lane loses its NAME, not its content.

It does not make the numbering contiguous, and it should not. `v1beta2` and
`v1beta3` are burned: the ledger records them as retired with the reason, and a
reader who finds one of those identities in an old document can learn what
happened to it. Renumbering v1beta4 down to v1beta2 would make the ledger lie
about which names have been minted.

## Consequences

One runnable corpus measures the one current lane against the one current
family, and the coupling that forced this — a corpus can only install the
family the registry carries — is now stated rather than discovered.

The cost is that this project's lane numbering has gaps, and that two lanes'
worth of authoring was retired within days of being written. The alternative
was three lanes where one does the work, two of which no host could run, which
is a worse thing to leave in a specification than a gap in a number line.
