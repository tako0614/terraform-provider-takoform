# 0029 — An immutable revision is named by its content

- Status: accepted
- Date: 2026-08-09
- Owners: Takoform maintainers

## Context

The Edge Platform Family splits one running worker across five Forms, and two
of them — `WorkerBundle` and `WorkerVersion` — carry the `revision` role
([0009](0009-form-families-and-namespaced-api-versions.md)). A host refuses
every update to a revision-role resource, so any desired change to one is a
REPLACEMENT rather than a mutation. In Terraform, a replacement of a resource
whose `name` the author pinned lands under the name the old resource still
holds.

That was believed to deadlock. It was an inference from the spec, so it was
driven end to end against the deterministic v1alpha3 reference host with the
real `tofu` (1.12.3) and `terraform` (1.15.8) CLIs, over a live worker
aggregate — a bundle a version executes, a version a deployment weights — with
only the module bytes changed between applies. What actually happens:

- **Destroy-then-create**, Terraform's default replacement order, fails on the
  FIRST step. `takoform_worker_bundle.app: Destroying...` returns
  `dependency_in_use` (409): *resource is the target of 1 live relation
  holder(s) held by WorkerVersion counter-version*. Nothing is mutated and the
  apply exits non-zero.
- **`create_before_destroy = true`** fails on the first step too, from the other
  side: `takoform_worker_bundle.app: Creating...` returns `invalid_argument`
  (400) — *prepare on an existing resource requires the Takoform-Expected-
  Generation fence*. The name is still occupied, a create carries no generation
  fence, and the host refuses before the mutation. Nothing is mutated.
- A third fact the run exposed is worse than the deadlock and was not part of
  the hypothesis. The plan did not replace the `WorkerVersion` at all — only the
  bundle. A version references its bundle by NAME, the pinned name did not
  change, so the version had no diff. Even if the bundle replacement had
  somehow completed, the deployment would still serve the old version, pinned by
  UID to the old bundle incarnation ([0015](0015-cross-resource-references-are-uid-pinned-relations.md)),
  and the new code would never reach traffic.

So the deadlock is real, it is symmetric, and the configuration that produces it
is the obvious one. There is also no plan that repairs it: the destroy is
refused while the holder lives, and the holder is not being replaced.

## Decision

A revision's host name is derived from its content, and the provider refuses at
plan time the replacement that cannot complete.

1. **Derived names.** On a `revision`-role Form the provider's `name` attribute
   is `Optional+Computed`. When the configuration omits it, the provider derives
   `<prefix>-<first 12 hex of the content digest>`, where the prefix is the
   Form's slug with the family's `worker-` qualifier removed — so
   `bundle-<manifest digest prefix>` and `version-<spec digest prefix>`. The
   content digest is the manifest digest when the Form's whole desired state is
   a content address, and the RFC 8785 canonical digest of the desired spec
   otherwise. Changed content is therefore a different resource NAME, created
   beside the old one, rather than a replacement of the same one.

2. **The plan computes it, and apply agrees byte for byte.** The name is derived
   in `ModifyPlan` from values the plan already knows, and `name` is added to
   `RequiresReplace` there, because attribute plan modifiers have already run by
   the time `ModifyPlan` changes a value. Where the plan cannot resolve the
   content — a build output that does not exist yet — the planned value stays
   unknown on a create and untouched on a replacement, and apply derives it from
   the same function over the same spec.

3. **A same-name replacement is a plan error.** When a revision-role resource
   already exists, a plan replaces it for any reason other than the
   relation-drift recovery marker of
   [0015](0015-cross-resource-references-are-uid-pinned-relations.md), and the
   planned name equals the recorded one, the provider raises

   ```
   This immutable revision cannot be safely replaced under the same host name.
   Use a new revision name or the official worker-app module.
   ```

   naming both refusals above, the exact FormRef, and the pointer
   `/metadata/name`.

4. **The official `worker-app` module is the ordinary surface.** The raw Forms
   stay the low-level one. The module writes no `name` on either revision and
   declares `create_before_destroy` on both — derived names alone are not
   enough, because Terraform's default order still destroys the old revision
   first and the destroy is still refused. The identity and the deployment
   deliberately do NOT carry `create_before_destroy`: they are the stable
   things, and the deployment is updated in place.

5. **The sequence is measured, not assumed.** A conformance command
   (`cmd/worker-authoring-conformance`) drives both CLIs against the reference
   host and asserts the exact mutation timeline of a code change —

   ```
   PUT WorkerBundle bundle-<new> 201
   PUT WorkerVersion version-<new> 201
   PUT WorkerDeployment <name>-deployment 200
   DELETE WorkerVersion version-<old> 204
   DELETE WorkerBundle bundle-<old> 204
   ```

   — while sampling the `ModuleWorker`'s Ready condition throughout the apply.
   Ready is a claim about SERVICE ([0016](0016-the-worker-aggregate-has-one-active-deployment.md)),
   so an observation of `Ready=False` during the apply would be exactly the
   window where nothing serves. The run must record zero.

## Consequences

- A revision's name is no longer something an author chooses, and plans now show
  `bundle-…`/`version-…` names that change on every code change. That is the
  honest rendering of what a revision is; the stable handle an author reads and
  references is the `ModuleWorker` identity, whose name they still choose.
- Two revisions with identical content derive one name. That is correct — they
  are the same revision — and a genuine 48-bit collision between different
  contents fails on the host's own create fence rather than overwriting
  anything.
- An author who pins a name keeps it, and gets the plan error instead of a
  failed apply. Pinning remains legitimate: an imported revision has a name the
  host already assigned, and the raw surface must be able to express it.
- Deleting the old revisions is now part of the ordinary apply rather than a
  separate cleanup, so a long-lived worker does not accumulate them.
- `create_before_destroy` is a Terraform meta-argument, invisible to a provider.
  The plan error therefore fires for both orders, because both fail; the
  provider cannot tell them apart and does not need to.

## Rejected alternatives

- **Let the author keep pinning names and document the ordering.** This is the
  status quo, and the measurement above is what it costs: the first apply after
  a code change fails, in either order, with an error that names a dependency
  rather than the naming decision that caused it. No amount of documentation
  makes a deadlocked apply recoverable from inside Terraform.
- **Make the provider delete the holder and re-create it.** A provider that
  deletes a resource it was not asked to delete is a provider that can lose a
  deployment. It would also have to sequence three resources' lifecycles from
  inside one resource's apply, which Terraform's graph exists to do.
- **Suffix the name with a serial (`-1`, `-2`).** A serial is state the provider
  would have to keep and reconcile, it is not reproducible from the
  configuration, and two workspaces applying the same bytes would produce
  different names. A content digest is a function of the thing itself.
- **Derive the bundle name from the spec digest rather than the manifest
  digest.** Rejected because the manifest digest IS the bundle's content
  address; hashing a one-field document that already contains it adds a level of
  indirection and makes the same bytes derive different names under different
  spec spellings.
- **Fire the plan error for the relation-drift replacement too.** A revision
  whose relation target moved is replaced under the same name by design
  ([0015](0015-cross-resource-references-are-uid-pinned-relations.md) rule 7),
  and that path already states its own remedy. Refusing it here would remove a
  recovery without offering a better one.
- **Mark the name unknown when the plan cannot resolve the content.** Unknown
  differs from the recorded value, so it would force a replacement on every plan
  of a resource whose build output happened to be absent — a perpetual diff
  produced by the provider rather than by any change. Leaving the planned value
  alone hands the case to the plan error, which states the real problem.
