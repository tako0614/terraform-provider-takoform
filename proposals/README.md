# Form Proposal workspace

This directory contains mutable design material. A proposal is never itself a
package, release, maturity claim, Host Support decision, or Service Offering.

## Current official family

The current publisher corpus is the one versionless family
`edge.forms.takoform.com`: 16 exact Experimental `0.x` Forms, with 7
Interfaces and 6 Bindings. The source roster is pinned to [`takoform-forms`
commit `3a395e4`](https://github.com/tako0614/takoform-forms/tree/3a395e4d7f9f652a942da52905857fccc41b467e).
The retained Provider candidate projection is
[`forms/candidates/edge.forms.takoform.com/candidate-set.json`](../forms/candidates/edge.forms.takoform.com/candidate-set.json);
it is source history, not a second publisher registry.
The [Edge Platform Family proposals](edge/README.md) mirror that roster and
remain non-normative design notes; exact identity lives in the pinned source.

## Deferred candidate families (historical source)

Function, Container, Table, Pull Queue, Topic, Schedule, and Vector proposals
are retained historical/deferred source. They are **not current Forms**, do
not appear in the Current navigation, and reserve no current identity:

- [Function family (historical/deferred)](function/README.md)
- [Container family (historical/deferred)](container/README.md)
- [Table family (historical/deferred)](table/README.md)
- [Pull Queue family (historical/deferred)](queue/README.md)
- [Topic family (historical/deferred)](topic/README.md)
- [Schedule family (historical/deferred)](schedule/README.md)
- [Vector family (historical/deferred)](vector/README.md)

Their pages remain available for source inspection, migration, and exact
historical-state recovery. The released Provider `3.0.0` projection of those
families is an immutable 8-family/31-resource historical record, not evidence
that the publisher's current corpus has that shape. A future official-only
Provider mapping for Edge16 is a next-major candidate and is not published.

The pre-family flat Proposal set and lifecycle registry were withdrawn with
the pre-Beta epochs; their documents stay in git history as design provenance
and reserve no public identity.

Start new design work by copying `TEMPLATE.md` into a family's subdirectory.
Proposal review does not authorize publication. A withdrawn proposal is
removed unless a decision record needs to retain its design history.

Before review, run the family conformance check:

```bash
go run ./cmd/standard-form-conformance verify
```
