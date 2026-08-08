# 0016 — The Worker aggregate has one active deployment

- Status: accepted
- Date: 2026-08-08
- Owners: Takoform maintainers

## Context

The Edge Platform Family splits one running worker across five Forms: a
`ModuleWorker` identity, immutable `WorkerVersion` revisions, a
`WorkerDeployment` that selects which of those versions serve traffic, and the
`WorkerCustomDomain`, `WorkerCronTrigger`, and `QueueConsumer` attachments that
activate it inward. The split is deliberate — it is what makes rollback a
re-weighting instead of a mutation — but the v1alpha3 lane never stated what
holds the five together, so a conforming host accepted configurations that
cannot serve traffic coherently.

Nothing stopped two differently-named `WorkerDeployment` resources from
targeting one `ModuleWorker`, leaving "which one serves" undefined. A deployment
could weight a version belonging to a different worker, name the same version
twice under two shares, or weight a version that was not ready. The attachment
gate read the wrong evidence: it admitted an attachment when *any* stored
`WorkerVersion` of the worker declared the handler, including a version no
deployment serves, so a cron trigger could exist while nothing scheduled was
deployed. There was no reverse check at all: with that trigger live, the
deployment could move to 100% of a version exporting no `scheduled` handler.
Deleting the deployment was unguarded. And `vars` keys,
`requiredSensitiveVars` entries, and the five binding lists all project into one
runtime environment namespace, where `uniqueItems` rejects only a duplicated
whole object — two bindings may share a name with different targets, and a
binding may collide with a var.

Decision [0015](0015-cross-resource-references-are-uid-pinned-relations.md) made
every reference a resolved, UID-pinned relation and gave a host a reverse index
from target UID to holders. That is the machinery these rules are built on; it
does not decide any of them. Decision
[0014](0014-published-schemas-are-structural-minima.md) settled where they may
live: none of them is expressible in a Draft 2020-12 desired schema — a schema
cannot count the deployments pointing at one worker, read the `/worker` relation
of a version it does not contain, add weights, know which handlers a referenced
version exports, or reach across sibling properties — so they belong in the
host, the authoring model, and the conformance corpus. They are new MUST-level
host semantics, so this repository's `AGENTS.md` requires a decision record.

## Decision

The **Worker aggregate** is one `ModuleWorker` incarnation together with the
Worker Versions pinned to it, the one Worker Deployment that governs its
traffic, and everything activated against it. The rules below are normative;
the operative text lives in [`../host-api/v1alpha3.md`](../host-api/v1alpha3.md)
and [`../form-families.md`](../form-families.md).

1. **One active deployment per worker.** Creating or importing a
   `WorkerDeployment` whose resolved `/worker` UID already has one MUST fail
   `invalid_argument` (400) before any mutation. Re-applying the worker's own
   deployment is not a second one. Traffic moves by re-weighting the deployment
   a worker already has.
2. **Deployment integrity.** Before any mutation a host MUST refuse a
   deployment whose `versions[]`
   - weights a `WorkerVersion` whose stored `/worker` relation targets a
     different worker UID;
   - names one `WorkerVersion` twice, by resolved UID;
   - carries weights that do not sum to exactly 10000 basis points;
   - weights a version that is not Ready, or that an accepted delete is already
     removing.

   Each failure is `invalid_argument` (400): the request is well formed but
   states something untrue about what will run.
3. **Attachments gate on the active deployment.** A `WorkerCustomDomain`
   requires `fetch`, a `WorkerCronTrigger` requires `scheduled`, and a
   `QueueConsumer` requires `queue`. The evidence is the worker's active
   deployment, and EVERY version that deployment weights MUST export the
   handler — a request served by any weighted version has to find it. An absent
   deployment or a missing handler MUST fail `unsupported_capability` (422)
   before any mutation, naming what is missing.
4. **Reverse validation on deployment change.** An apply that would leave a live
   dependent unserved MUST be refused before any mutation, with
   `unsupported_capability` (422). The dependents are a live
   `WorkerCustomDomain` (`fetch`), a live `WorkerCronTrigger` (`scheduled`), a
   live `QueueConsumer` (`queue`), and a live INBOUND service binding — another
   Form's `serviceBindings` entry targeting this worker — which requires
   `fetch`.
5. **Deletion is blocked by dependents.** Deleting a `WorkerDeployment` while
   any of those four dependents lives MUST fail `dependency_in_use` (409).
   Nothing references a deployment, so this is not the relation rule of decision
   0015; it is the same statement about a different edge.
6. **The environment namespace is single.** Within one `WorkerVersion`, the
   union of `vars` keys, `requiredSensitiveVars` entries, and every binding
   `name` across every binding list MUST be unique. A collision MUST fail
   `invalid_argument` (400) before any mutation, and a client SHOULD refuse it
   at plan time so the author sees it without a round trip.
7. **Worker readiness follows the deployment.** A `ModuleWorker` reports
   `Ready=True` only when it has an active deployment whose every weighted
   version exports `fetch`. Before that it reports `Ready=False` with
   `Provisioning` (no deployment) or `UnsupportedCapability` (a deployment that
   serves no `fetch`), and a `hostReason` naming the worker and what is missing.
   The worker exists; it serves nothing.
8. **An inbound service binding is refused at bind time.** Because
   `worker.service` is provided by the worker IDENTITY and answered by whatever
   its deployment selects, a `module-worker.service` binding to a worker that is
   not serving `fetch` MUST fail `unsupported_capability` (422) before any
   mutation. It is refused rather than stored-and-reported: a stored binding
   that projects nothing is a declared capability no host can keep, and the
   first Forms should be simple to reason about.

Rules 3, 4, and 8 are three views of one fact — an activated worker is what its
deployment selects — so a host that implements any of them in terms of stored
versions rather than the deployment has implemented none of them.

Rule 6 is enforced in three places for three different reasons: the authoring
model proves a Form's own canonical examples are collision-free, the provider
refuses the configuration during plan against the attribute the author wrote,
and the host refuses the spec before mutation because it is the only party that
sees every client.

The rules are proven by the required conformance checks
`deployment-single-active-per-worker`, `deployment-version-ownership`,
`deployment-version-duplicate-rejected`, `attachment-requires-active-deployment`,
`deployment-change-preserves-dependents`,
`deployment-delete-blocked-by-dependent`, and
`binding-name-collision-rejected`, alongside the existing
`deployment-weight-sum-enforced` and `handler-gated-attachments`.

## Consequences

- A worker's Ready condition becomes a claim about SERVICE rather than about the
  existence of a record. That is the honest reading of an identity Form whose
  whole purpose is to be addressed, and it makes "the worker is up" answerable
  from one resource instead of by inspecting the aggregate.
- Two workers cannot service-bind to each other from scratch. Each needs the
  other serving `fetch` before its own version can be applied, so a mutually
  calling pair is built one direction at a time, or the second direction is
  added in a later version. Fail-closed costs this; storing a broken binding
  would cost correctness.
- Removing a worker is now explicitly ordered: attachments and inbound callers
  first, then the deployment, then the versions, then the worker. Every step is
  a refusal with a named dependent rather than a silent degradation, so the
  order is discoverable from the errors.
- A deployment is a claim about what runs, so the versions it weights are held
  to being runnable at the moment it is stored. A host with an asynchronous
  provisioning model refuses a version whose Ready is False for its own reasons
  too, not only for relation drift.
- The environment namespace rule is stated for `WorkerVersion` because it is the
  one Form today that holds bindings alongside `vars` and
  `requiredSensitiveVars`. The binding lists are discovered from the
  `x-takoform-binding` annotation the desired schema already carries, so a Form
  that gains a sixth binding list is covered without a host edit.
- Whether two declarations collide is a property of one instance, not of a Form.
  The authoring model therefore cannot forbid it — a Form declaring five binding
  lists is not wrong, its author may simply write one name in two of them — and
  what the model proves is the collision it owns: the Form's own canonical
  examples.

## Rejected alternatives

- **Allow several deployments per worker and pick one by a rule.** Rejected
  because every candidate rule — newest, lowest name, highest total weight,
  most recently applied — is a fact about host bookkeeping rather than about
  what the author asked for, and none is predictable from the configuration. The
  author would have to know the rule to know what serves, and two hosts choosing
  differently would both be conforming.
- **Gate attachments on any stored `WorkerVersion` that declares the handler.**
  This is what the lane did before this decision, and it is wrong because a
  stored version is not a running one. Versions are immutable revisions kept
  precisely so that old ones remain addressable for rollback, so the set of
  stored versions is a history, not a description of the current service. The
  rule let a cron trigger be created against a `scheduled` handler that existed
  only in a version no deployment selected — a trigger that would fire against
  nothing — and it reported success while doing it. Worse, it made the gate
  unfalsifiable in the direction that matters: once admitted, no later
  deployment change was checked against it.
- **Degrade dependents to not-Ready instead of refusing the change.** Rejected
  because it converts one explicit action into a fan-out of broken resources.
  The author asked to move traffic; they would get a successful apply plus three
  attachments and an unknown number of callers silently reporting failure, with
  no statement of which change caused it and no order in which to repair it.
  Refusing names the dependent, keeps the system serving, and leaves the
  author's next action obvious: remove what depends on the handler, or keep
  exporting it. The degrade behavior is also strictly harder to reverse — the
  broken state is already stored — and reversibility is what the deployment Form
  exists to provide.
- **Require every weighted version to export every handler any attachment could
  use.** Rejected as a blunter version of rule 3 that forbids a worker with only
  a `fetch` handler from ever being deployed. The gate is per attachment because
  the requirement is per attachment.
- **Express the environment namespace in the desired schema.** Rejected because
  it is unexpressible. `uniqueItems` compares whole objects, so two bindings
  agreeing only on `name` are distinct; no keyword relates a property's keys to
  a sibling array's element member; and the published Form Definition schema is
  immutable anyway (decision 0014).
- **Report a not-serving worker as Ready.** Rejected because it makes the
  condition meaningless for the one Form whose purpose is to be addressed by
  others. A caller reading `Ready=True` on a worker that answers nothing has
  been told the opposite of the truth.
