# 0036 — An artifact's name states the lane it describes, not its place in a sequence

- Status: Accepted
- Date: 2026-08-21

## Context

Takoform's version axes are independent on purpose, and
[`versioning.md`](../versioning.md) says their numbers MUST NOT be aligned to
imply a shared release. Independence is correct and it is not the problem this
record answers.

The problem is that some names are **absolute** and some are **relative**.
`forms.takoform.com/v1beta1` names one lane on the day it is written and
afterwards. `current`, and the Nth of a sequence, name a position: they are true
only until the thing they point at moves, nothing announces the move, and the
correction has to be made by hand.

The correction was missed. The conformance corpora were numbered by generation,
and for three generations the counter and the lane agreed — `portable-host-v1`
measured `v1alpha1`, `-v2` measured `v1alpha2`, `-v3` measured `v1alpha3`. When
[decision 0035](0035-beta-contracts-ship-in-stable-provider-v2-1.md) moved the
current lane to `forms.takoform.com/v1beta1`, the absolute names were minted
correctly: `host-api/v1beta1.md`, `host-api-wire-v1beta1.schema.json`,
`host-discovery-v1beta1.schema.json`, and the rest arrived beside their retained
v1alpha3 counterparts, exactly as 0035 requires. `conformance/portable-host-v3`
was rewritten in place instead.

That made one published address answer about a different contract than it had
answered about the day before:

```
https://takoform.com/conformance/portable-host-v3/contract.json
  before: forms.takoform.com/v1alpha3, 114 required checks
  after:  forms.takoform.com/v1beta1,  116 required checks
```

which is precisely what 0035 forbids — every `v1alpha3` schema, specification,
operation table, public URL, and byte remains retained history. Comparing the
827 published JSON documents that existed on both sides of that change finds
exactly two whose declared lane moved, and both are inside that corpus. The
retention rule held everywhere a name was absolute and failed at the one place
a name was relative.

No gate saw it, because every corpus check read the corpus and compared it
against itself. A corpus that changes lanes agrees with itself perfectly.

## Decision

**A new artifact whose name carries a version word names the lane it describes.**
`conformance/portable-host-v1beta1` rather than a fourth generation number; a
Go package, script, or directory minted for a lane says which lane.

**Already-published relative names are retained history and stay.**
`portable-host-v1`, `-v2`, and `-v3` are published addresses. Renaming them would
be the same defect in the other direction. `-v3` holds the v1alpha3 bytes again,
restored byte-for-byte, so the address that always named them names them once
more.

**A retained corpus is retained bytes, not a runnable corpus.** The v1alpha3
runner became the v1beta1 runner rather than being copied, so no runner in this
repository loads `portable-host-v3`, and reviving one would prove nothing about
a lane no host is being measured against. What is guaranteed is that the bytes
do not change.

**`current` is not one word, and documents say which one they mean.** The current
lane and family are `v1beta1`. The retained central Form epoch is
`forms.takoform.com/v1alpha2`, and that is what `formpackage.CurrentFormAPIVersion`,
`forms/lifecycle.json`'s `currentEpoch`, and `internal/currentform*` name.
`forms/lifecycle.schema.json` pins that value as a `const` and
`internal/standardforms` enforces it, so it is deliberate; the word alone does
not say so.

## Enforcement

Two checks in `bun run check`, because the defect had two halves.

`release/published-document-lanes.json` records the lane each published document
declares, and `check:published-document-lanes` refuses a change to one and
refuses one disappearing. It reads the built site rather than the sources,
because the thing that must not move is the address a reader fetches.

`checkCorpusNamesStateTheirLane` in `scripts/check-public-surfaces.mjs` refuses a
corpus directory whose name disagrees with the lane its contract declares. The
three published generation-named corpora are listed one by one rather than
matched by a pattern, so a fourth is an edit somebody has to justify.

Both were driven backwards before being trusted: on a replay of the original
defect, on a published address disappearing, on a new corpus misnamed by
generation, and on a retained address made to serve another lane.

## Consequences

The rule is about naming and never about values. Every published value stays
exactly where it is: the lane, the family, the fifteen exact FormRefs and their
digests are frozen by Registry-published provider `v2.1.1`; every
`spec/schemas/` filename is frozen by the append-only public schema identity
ledger; `packages.forms.takoform.com/v1alpha4` is frozen inside published schema
bytes. Nothing in this decision moves any of them.

Internal package and identifier names that are still relative —
`internal/portableconformancev3`, `internal/clientv3`, `internal/currentform*`,
`internal/provider/v3_*.go` — are free to be made absolute and are not changed
here. This record is what a later change of that kind cites.

Earlier decisions name `conformance/portable-host-v3` because that path was the
v1alpha3 corpus when they were written. Restoring those bytes makes those
references true again. A decision written during the Beta lane that names the
same path means what is now `conformance/portable-host-v1beta1`; decision records
are history and are not rewritten to match a later layout.
