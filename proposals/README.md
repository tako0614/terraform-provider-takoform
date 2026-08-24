# Form Proposal workspace

This directory contains mutable design material. Some notes accompany exact
unpublished candidate FormRefs; others describe possible future Forms or
retained history. A document here is never itself a package, release, maturity
claim, Host Support decision, or Service Offering.

Family design work lives in per-family subdirectories, authored under the
shape-preserving boundary of
[decision 0008](../spec/decisions/0008-forms-preserve-service-shape.md) and
selected by [decision 0043](../spec/decisions/0043-forms-target-popular-vendor-locked-primitives.md):

- [Edge Platform Family](edge/README.md) — the first family; its candidates
  are tracked by
  [`../forms/candidates/edge.forms.takoform.com/candidate-set.json`](../forms/candidates/edge.forms.takoform.com/candidate-set.json).
  The group carries no version
  ([decision 0049](../spec/decisions/0049-a-form-versions-alone.md)), so a
  candidate is proposed in this tree and joins that one set rather than
  waiting for a generation to be minted around it.
- [Regional Function Family](function/README.md) — regional FaaS, 4 current candidates.
- [Serverless Container Family](container/README.md) — OCI-image services, 5 current candidates.
- [Table Family](table/README.md) — key-addressed document tables, 1 current candidate.
- [Pull Queue Family](queue/README.md) — pull-based at-least-once queues, 1 current candidate.
- [Topic Family](topic/README.md) — fanout topics and queue subscriptions, 2 current candidates.
- [Schedule Family](schedule/README.md) — standalone cron invocation, 1 current candidate.
- [Vector Family](vector/README.md) — fixed-dimension vector indexes, 1 current candidate.

All eight current versionless family candidate sets are closed by
[`../forms/candidates/current-family-index.json`](../forms/candidates/current-family-index.json):
31 exact Experimental `0.x` FormRefs in total. That generated index and the
candidate bytes it names own the current identities. Proposal prose remains
non-normative and reserves no additional identity.
The pre-family flat Proposal set and its lifecycle registry were withdrawn
with the pre-Beta epochs
([decision 0042](../spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.md));
their documents stay in git history as design provenance and reserve no public
identity.

Start a new family Proposal by copying [`TEMPLATE.md`](TEMPLATE.md) into the
family's subdirectory under a descriptive lowercase filename. A Form enters
the family only through the generated candidate set; the public-surface gate
holds every hand-written inventory to that roster, so a Proposal without a
candidate entry cannot leak into the published surface.

Before review, run:

```bash
go run ./cmd/standard-form-conformance verify
```

Proposal review does not authorize publication. When a Proposal is withdrawn,
remove its document unless a decision record needs to retain the design
history; no public identity is reserved by a withdrawn Proposal.
