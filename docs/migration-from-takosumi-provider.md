# Migrate typed resources from Takosumi to Takoform

`takosumi_*` and `takoform_*` are different Terraform resource types owned by
different provider addresses. Do not treat either address as an alias and do
not delete the old provider or lock entry until rollback evidence is complete.

## Preconditions

1. Pin the exact old Takosumi provider artifact and `.terraform.lock.hcl`.
2. Back up state with `tofu state pull` and record its SHA-256.
3. Run an old-provider refresh-only plan and require no changes.
4. Confirm the host has admitted the exact build-pinned FormRef/package identity for
   every resource kind being migrated, as pinned in
   [`forms/standard-package-set.json`](../forms/standard-package-set.json).
5. Before removing any state address, require the operator to backfill every
   existing canonical Resource with that exact FormRef/package identity and a
   ResolutionLock for its currently selected native implementation. This is a
   host-side migration prerequisite: the old provider state does not contain
   enough immutable Form lineage to reconstruct it safely, and a newly resolved
   implementation is not an acceptable substitute.
6. Build or install the reviewed Takoform candidate through a local filesystem
   mirror; a public Registry release is not required.

## Approved transition

For the mappings in
[`conformance/provider-migration-v1/mapping.json`](../conformance/provider-migration-v1/mapping.json):

1. replace the old provider requirement with
   `registry.terraform.io/tako0614/takoform` and replace the resource block type
   with its mapped `takoform_*` type in a reviewed HCL change;
2. keep the old HCL in version control or a rollback copy, but do not leave both
   old and new resource blocks active in one root;
3. record every canonical `SPACE/NAME`, then remove only each old address from
   state (`tofu state rm`), never the remote Resource;
4. import every canonical Resource as `SPACE/NAME` into its new address;
5. after all mapped resources are imported, run a refresh-only plan and require
   no changes;
6. retain the old artifact, lock file, HCL revision, and state backup for the
   rollback window.

Do not run a normal plan or apply between `state rm` and `import`. If migration
is interrupted in that window, restore the backup rather than allowing either
provider to create a replacement.

This `tofu import SPACE/NAME` step adopts the already-existing canonical Form
host Resource into Terraform state. It deliberately does **not** adopt an
unmanaged native backend object and does not call the Deploy API native import
operation. Native backend adoption requires a separately reviewed desired
Resource body plus native ID through `POST .../import`; do not substitute that
operation into this provider-address migration. If exact FormRef and
ResolutionLock backfill has not made the canonical Resource readable first,
stop and restore the state backup.

The old Takosumi provider exposed only `edge_worker`, `object_bucket`,
`kv_store`, `queue`, `sql_database`, and `container_service`. Takoform's
`vector_index`, `durable_workflow`, `stateful_actor_namespace`, and `schedule`
have no old `takosumi_*` resource to migrate; create those only as new
`takoform_*` resources.

Direct `state mv` and `replace-provider` are not presented as aliases: they can
hide schema/type ownership changes. The explicit remove/import path re-reads
the canonical host Resource through its exact FormRef and generation fence.
Do not copy the old `target_pool` argument. The old computed
`selected_implementation`, `target`, and `locked` fields intentionally have no
Takoform state equivalent; the new provider records only `resource_version`,
`drift_status`, portability, typed desired fields, and sanitized public outputs.

## Rollback

Stop before any apply, restore the state backup, restore the pinned old lock
file/provider artifact, restore the old HCL, and run the old-provider
refresh-only plan. It must remain a no-op. If either old or new refresh differs,
do not continue and do not rewrite state by hand.

Run `go run ./cmd/migration-proof` to verify the redacted six-kind legacy
backup, its exact provider/type bijection, state lineage/schema-version
continuity, overlapping desired attributes, canonical identity continuity, and
the backend-free ten-kind Takoform golden state. Publication runs
`go run ./cmd/migration-proof --require-complete`; that mode fails while any
phase is `external-required` or any external blocker remains. The checked-in
report intentionally lists the live old/new and rollback refresh proofs as
external blockers until the pinned old artifact/lock/HCL input and a reachable
operator migration host are supplied, so a release cannot turn structural
evidence into a false lifecycle claim.
