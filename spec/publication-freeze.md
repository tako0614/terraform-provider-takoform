# Publication freeze: the v1alpha3 lane

The Host API `forms.takoform.com/v1alpha3` lane and the
`edge.forms.takoform.com/v1alpha1` Edge Platform Family are
**publication-frozen**.

While this freeze holds, the project MUST NOT:

- publish a Form Package for any family Form;
- publish an Interface Definition or Binding Definition, in any envelope. No
  Interface Package or Binding Package identity exists to publish one in; those
  contracts are documents distributed with this repository
  ([decision 0021](decisions/0021-third-party-forms-and-contract-distribution.md));
- release provider `v2.1.0`, or any provider version that exposes the family
  resource types, to the Terraform Registry;
- transition any family Form to Experimental, Stable, or any other lifecycle
  state in [`../forms/lifecycle.json`](../forms/lifecycle.json);
- describe a family Form, Interface, Binding, or the provider v2.1 line as
  published, Experimental, Stable, standard, admitted, or approved.

The freeze does not restrict repository work. Definitions, schemas, exact
FormRef digests, provider surfaces, and conformance may still change
incompatibly, because nothing in the lane has a published identity to
preserve.

## Why the lane is frozen

Every family Form is `publicationStatus: unpublished`
([`../forms/candidates/edge/v1alpha1/candidate-set.json`](../forms/candidates/edge/v1alpha1/candidate-set.json)),
so today a defect can be corrected by regenerating a digest. After
publication the same defect becomes a compatibility obligation. The
publication blockers are exactly the invariants a consumer would otherwise
have to live with forever.

## What is already published, and therefore immutable

Publication is per artifact, not per lane. The lane's **schema documents**
are published and immutable even though no Form is:

| Published | Consequence |
| --- | --- |
| the schema `$id` URLs recorded in [`../release/public-schema-identities.json`](../release/public-schema-identities.json) | their bytes can never change; a contract change mints a **new** identity |
| provider `v1.0.3` and `v2.0.0` | their Registry releases, tags, and state contracts are fixed |
| the 34 Legacy Form Packages and the retained admission evidence | retained byte-for-byte |

A change to the frozen lane therefore has two different costs. Changing a
Form Definition, a generated candidate, or a provider surface is free.
Changing a published schema document is impossible: the append-only ledger
and the deploy no-overwrite guard both reject it, so the change must mint a
new schema identity instead. Prefer expressing a contract addition through
data the published schema already admits — for example a JSON Schema
`default` inside a Form's desired schema — over minting a new identity.

## Publication blockers

A blocker is an invariant that is cheap to fix now and expensive to fix after
publication. The machine authority is
[`publication-blockers.json`](publication-blockers.json); each entry is also
tracked as an issue labelled `v1alpha3-publication-blocker`. The freeze lifts
for a given Form only when every blocker that touches it is closed, its
evidence exists under [`project-lifecycle.md`](project-lifecycle.md), and a
real host has implemented it end to end.

Prose cannot stop a release, so the ledger is enforced rather than described.
`bun run check` validates its shape on every change and, while any blocker is
open, additionally requires the state the repository would not be in had the
lane published: the family candidate set must still declare itself
`unpublished`, and no family Form may hold a lifecycle record.

A blocker closes by recording evidence that exists. An entry marked closed
with an empty evidence list fails, and so does one naming a path that is not
in the repository, so the freeze cannot be lifted by editing a status field or
by inventing a filename. `bun run assert:publishable` is the assertion a future
lane-publishing path calls; today it refuses and names every open blocker.

The freeze is **scoped to this lane's own artifacts**. It is deliberately not
wired into `check:release-owner-gate`, because that gate also serves the
retained v1alpha1/v1alpha2 packages and the append-only security revocation
path in [`trust/`](trust/). An urgent revocation for an already-published
package must never wait on this lane's product readiness; freezing new
publication and blocking a security withdrawal are opposite obligations.

The first Forms proposed for Experimental publication are the Worker and edge
KV vertical slice: `ModuleWorker`, `WorkerBundle`, `WorkerVersion`,
`WorkerDeployment`, `WorkerCustomDomain`, and `EdgeKVNamespace`. The queue and
schedule Forms follow. `ObjectBucket` and `SQLiteDatabase` come last, because
their Interface contracts still need the most work and publishing them early
would create the largest compatibility surface.

Lifting the freeze is a separate reviewed decision. Passing local gates is not
publication authority, and this document is not evidence that any blocker has
been closed.
