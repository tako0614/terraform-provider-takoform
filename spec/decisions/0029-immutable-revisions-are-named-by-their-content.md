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

A name derived from content alone then turned out to have the opposite problem,
found by driving TWO `worker-app` instances in one space from byte-identical
build output. They derive one manifest digest, therefore one
`bundle-dbdf1aacff23`, and what happens next is a coin toss. Sometimes the
deterministic create replays the same UID for the same principal and both module
instances silently manage one host address; sometimes the second create loses
its `If-None-Match: *` and the apply fails with *prepare on an existing resource
requires the Takoform-Expected-Generation fence* (`invalid_argument`, 400).
Either way the first destroy is wrong: it is refused because the peer's version
still holds the bundle, or it succeeds and takes the peer's bundle with it. A
digest is a name and not an ownership claim ([0012](0012-artifacts-use-content-addressed-upload.md));
Terraform requires each managed address to have exactly one owner. Nothing in
the content, the space, or the framework says who that owner is — a provider is
never shown its own resource address — so the owner has to be DECLARED.

## Decision

A revision's host name is derived from its content and its declared owner, and
the provider refuses at plan time the replacement that cannot complete.

1. **Derived names.** On a `revision`-role Form the provider's `name` attribute
   is `Optional+Computed`. When the configuration omits it, the provider derives
   `<prefix>-<first 12 hex of the content digest>-<first 12 hex of the owner
   digest>`, where the prefix is the Form's slug with the family's `worker-`
   qualifier removed — so `bundle-<manifest digest prefix>-<owner digest prefix>`
   and `version-<spec digest prefix>-<owner digest prefix>`. The content digest
   is the manifest digest when the Form's whole desired state is a content
   address, and the RFC 8785 canonical digest of the desired spec otherwise.
   Changed content is therefore a different resource NAME, created beside the
   old one, rather than a replacement of the same one.

2. **The owner is declared, and required.** A new provider-side attribute,
   `revision_owner`, names the stable thing this revision belongs to — the
   `ModuleWorker` of the aggregate, in every ordinary case. It is authoring
   input only: no wire member carries it, the host never sees it, and the one
   thing it decides is the derived name. It is REQUIRED whenever the provider
   derives that name, and the provider refuses at plan time to derive a name
   without it (`takoform.provider/revision-owner-missing`) rather than mint one
   two owners can both reach. Pinning `name` and setting `revision_owner`
   together is also refused (`takoform.provider/revision-owner-ignored`): the
   pinned name settles the question and the owner would decide nothing. The
   owner travels as a digest rather than as a literal prefix so the derived name
   has a fixed width; a literal owner would push the longest legal owner name
   past the 63-character portable grammar, and the only way back inside it would
   be to refuse owner names the provider itself accepts.

3. **The plan computes it, and apply agrees byte for byte.** The name is derived
   in `ModifyPlan` from values the plan already knows, and `name` is added to
   `RequiresReplace` there, because attribute plan modifiers have already run by
   the time `ModifyPlan` changes a value. Where the plan cannot resolve the
   content — a build output that does not exist yet — the planned value stays
   unknown on a create and untouched on a replacement, and apply derives it from
   the same function over the same spec.

4. **A same-name replacement is a plan error.** When a revision-role resource
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

5. **The official `worker-app` module is the ordinary surface.** The raw Forms
   stay the low-level one. The module writes no `name` on either revision, sets
   `revision_owner = var.name` on both, and declares `create_before_destroy` on
   both — derived names alone are not enough, because Terraform's default order
   still destroys the old revision first and the destroy is still refused. The
   module's `name` is the right owner because it is already the one name an
   author chooses and it is already unique in the space: two instances of the
   module that collide on it collide on the `ModuleWorker` first, where the host
   refuses. The identity and the deployment deliberately do NOT carry
   `create_before_destroy`: they are the stable things, and the deployment is
   updated in place.

6. **The sequence is measured, not assumed.** A conformance command
   (`cmd/worker-authoring-conformance`) drives both CLIs against the reference
   host and asserts the exact mutation timeline of a code change —

   ```
   PUT WorkerBundle bundle-<new>-<owner> 201
   PUT WorkerVersion version-<new>-<owner> 201
   PUT WorkerDeployment <name>-deployment 200
   DELETE WorkerVersion version-<old>-<owner> 204
   DELETE WorkerBundle bundle-<old>-<owner> 204
   ```

   — while sampling the `ModuleWorker`'s Ready condition throughout the apply.
   Ready is a claim about SERVICE ([0016](0016-the-worker-aggregate-has-one-active-deployment.md)),
   so an observation of `Ready=False` during the apply would be exactly the
   window where nothing serves. The run must record zero.

7. **Two owners are measured too.** The same command stands up two `worker-app`
   instances in one space from byte-identical build output, asserts that the two
   bundles and the two versions are four distinct names, then moves ONE owner
   forward — the apply that deletes that owner's old bundle and version — while
   sampling the other owner's Ready condition. The untouched owner must still
   hold exactly the revisions it held before, and must never be observed
   unserved.

## Consequences

- A revision's name is no longer something an author chooses, and plans now show
  `bundle-…`/`version-…` names that change on every code change. That is the
  honest rendering of what a revision is; the stable handle an author reads and
  references is the `ModuleWorker` identity, whose name they still choose.
- Two revisions of ONE owner with identical content derive one name. That is
  correct — they are the same revision — and a genuine 48-bit collision between
  different contents fails on the host's own create fence rather than
  overwriting anything. Two DIFFERENT owners no longer do, which is the whole
  point of the owner half.
- A revision name is now 32 or 33 characters rather than 19, and reading it
  takes two glances instead of one: the content half still answers "which
  bytes", and the owner half answers "whose". The content half is unchanged and
  still equals the manifest digest prefix, so an operator comparing a bundle
  name against an artifact manifest reads the same twelve characters as before.
- The raw surface gained a required-in-practice argument. Omitting both `name`
  and `revision_owner` used to work and now fails at plan with a diagnostic
  naming the repair. That is a deliberate trade: what it used to do was mint a
  name a second configuration could reach, and the failure mode of that is a
  destroy that breaks somebody else's worker.
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
- **Model shared bundle ownership explicitly, so two versions may legitimately
  reference one bundle.** This is the other honest reading of the collision: two
  owners with identical bytes really do want the same artifact, and a bundle
  really is just content. It was rejected on what it costs. The provider would
  have to ADOPT a revision it did not create — the create fence
  (`If-None-Match: *`) exists precisely to refuse that, and a create that
  silently adopts cannot tell "my own bytes" from "somebody else's resource that
  happens to match". Deletion would then need reference counting the host does
  not expose and Terraform cannot hold, because each state file knows only its
  own references; the first `terraform destroy` would either leak the bundle
  forever or break the other owner, which is the defect it was meant to fix.
  And it would put two Terraform addresses on one resource permanently, which is
  the one thing Terraform's model does not admit. A discriminator costs one
  attribute and thirteen characters of name.
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
- **Prefix the derived name with the owner literally
  (`counter-bundle-<digest>`).** More readable, and rejected on length. The
  portable grammar admits 63 characters, so a literal owner would leave 42 or 43
  for the owner name, and the only way to keep the derivation inside the grammar
  would be for the module — and the raw surface — to refuse owner names
  `takoform_module_worker` accepts. Narrowing the provider's own grammar is a
  defect this same review found elsewhere in this change, and reproducing it
  here to buy legibility is not a trade worth making.
- **Derive the owner from something the provider already knows.** There is
  nothing. Two colliding resources share a space, share a Form, and share their
  content by construction; a `WorkerVersion`'s spec happens to carry its
  `worker`, but a `WorkerBundle`'s carries nothing at all, and two versions of
  one worker with an identical spec still collide. The Terraform resource
  address would settle it, and the plugin framework never shows a provider its
  own address.
- **Mark the name unknown when the plan cannot resolve the content.** Unknown
  differs from the recorded value, so it would force a replacement on every plan
  of a resource whose build output happened to be absent — a perpetual diff
  produced by the provider rather than by any change. Leaving the planned value
  alone hands the case to the plan error, which states the real problem.
