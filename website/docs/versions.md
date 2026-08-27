# Versions and compatibility

Takoform has exactly two domain version axes: the public API/Core release
SemVer and each Form's own `definitionVersion`. API/Core releases are
human-readable compatibility checkpoints; they do not rename the wire or
discovery lane. Provider SemVer, package and schema IDs or digests,
Interface/Binding references, Specification releases, and retained lanes are
artifact/history identities rather than additional domain axes.

The frozen predecessor source remains at
[`spec/versioning.md`](https://github.com/tako0614/terraform-provider-takoform/blob/896fb0e6c94557d97ba7445924fda18a8430ba8f/spec/versioning.md).
These current Provider pages are maintained separately from that historical
file.

## Current design target

| Domain axis | Current identity | Meaning and availability |
| ---------- | ---------------- | ------------------------ |
| API/Core release SemVer | **`1.0.1`** ([public Core release](https://github.com/tako0614/takoform/releases/tag/v1.0.1)) | Public compatibility checkpoint on the `forms.takoform.com/v1` wire/discovery lane. Every compatible `1.y.0` checkpoint remains on `/v1`; Host implementation/support/deployment remains separate. |
| Form `definitionVersion` (active publisher) | **1 family / 16 candidate Forms** | The standalone [`takoform-forms`](https://github.com/tako0614/takoform-forms) source currently carries `edge.forms.takoform.com`; package artifacts are unpublished and each Form advances independently. |

## API/Core release checkpoints

The current public API/Core checkpoint is **`v1.0.1`**, recorded by the
[Takoform Core release](https://github.com/tako0614/takoform/releases/tag/v1.0.1)
at commit `8da857ca21e90d4e46edb2e3f53197dbd1df0f3b`. It uses the existing
`forms.takoform.com/v1` wire and discovery lane. The release number is a
human-readable compatibility checkpoint, not a URL path. Compatible `1.x`
checkpoints remain on `/v1`; Host implementation, deployment, support, and
adoption remain host-owned facts. A protocol-breaking major gets a new lane
only through an explicit Core decision.

The historical **Specification 1.1** is a sealed exact source receipt in the
predecessor release ledger. It is not API release `1.1` or `1.1.0`, does not
create `/v1.1`, and is not an ongoing Specification version stream. Future
checkpoints are API/Core releases, not new Specification numbers.

## Artifact and history identities

| Identity | Current or retained meaning |
| -------- | --------------------------- |
| Specification **1.1** | Sealed historical identity of one exact normative source snapshot. It is not API release 1.1, does not publish or promote the `/v1` wire lane, a Form, a package, or Provider 3, and is not an ongoing version stream. Identity 1.0 was withdrawn before publication and may not be reused. |
| Host API wire/discovery lane **`forms.takoform.com/v1`** | The protocol path used by the API/Core `1.x` compatibility checkpoints; the path is not a third domain version axis. |
| Form Family group | Versionless namespace in each exact FormRef; older versioned groups remain retained or withdrawn history. |
| Form Package envelope **`packages.forms.takoform.com/v1alpha5`** | Manifest-format identifier. Package IDs and content digests identify artifact bytes, and package publication remains independent of Provider publication. |
| Schema IDs and digests | Exact Definition or wire-schema bytes, not a second Form version. |
| Interface/Binding refs and digests | Exact operation-surface and typed-capability contracts referenced by Forms. |
| Provider **3.0.0**, Registry-published | Non-normative Terraform/OpenTofu client distribution retaining the Provider 3 typed mapping. |
| Provider **2.1.1**, Registry-published | Retained Beta `v1beta1` client for the 15 immutable `edge.forms.takoform.com/v1beta1` FormRefs; its identity is recorded in the [provider Form identity ledger](/release/provider-form-identities.json). |
| Providers **2.0.0** and **1.0.3** | Withdrawn pre-Beta client identities; immutable Registry history for exact-pin recovery and migration only. |

Provider distribution is an artifact identity, not a domain axis. **Provider 3.0.0** is the current
Registry-published implementation. **Provider 2.1.1** is retained Beta
`v1beta1` history:
it targets Host API `forms.takoform.com/v1beta1` and the
`edge.forms.takoform.com/v1beta1` family, and remains installable for the exact
FormRefs recorded in the [provider Form identity ledger](/release/provider-form-identities.json).
By contrast, **Provider 2.0.0** and **Provider 1.0.3** are the withdrawn
pre-Beta epochs; their Registry identities remain immutable history for
exact-pin recovery and migration
([decision 0042](/spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.html)).
Provider 3 is a client artifact identity: it cannot block or authorize the
API/Core `1.0.1` release or the sealed Specification 1.1 receipt, and publishing
a new Form `definitionVersion` does not wait for a Provider release.

## Published compatibility mapping

| Client or distribution      | Host API          | Forms and definitions                                               | Status / use                                                                |
| --------------------------- | ----------------- | ------------------------------------------------------------------- | --------------------------------------------------------------------------- |
| Provider 3.0.0 distribution | API/Core `v1.0.1` on Host API wire v1 | 8-family / 31-Form retained typed compatibility mapping | Current Registry-published non-normative Provider history; not the active publisher roster. |
| Provider 2.1.1 distribution | Host API v1beta1  | Edge Form Family v1beta1; 15 immutable historical Form identities | Registry-published retained client; descriptor remains `candidate-only` metadata by design. |
| Provider 2.0.0 distribution | Host API v1alpha2 (withdrawn epoch) | The nine withdrawn v1alpha2 Forms | Immutable Registry history; exact-pin only, no successors. |
| Provider 1.0.3 Legacy       | Host API v1alpha1 (withdrawn epoch) | The withdrawn v1 Form Package identities | Immutable Registry history; recovery and migration only.   |

The API/Core `1.0.1` checkpoint, its Host API wire/discovery path, and the
versionless Form families are not interchangeable labels. A Form's own
`definitionVersion` is the only per-Form domain version; it does not change the
API/Core release or Provider SemVer. The sealed Specification 1.1 receipt does
not silently become API release 1.1 or mint stable Form identities, and a new
Form publication does not require a Provider release.

## Current Edge reference family (16 Experimental Forms)

The current Edge family is versionless and contains these 16 exact
Experimental `0.x` Forms:

- [`takoform_module_worker`](/docs/resources/module_worker.html)
- [`takoform_worker_bundle`](/docs/resources/worker_bundle.html)
- [`takoform_static_asset_bundle`](/docs/resources/static_asset_bundle.html)
- [`takoform_worker_version`](/docs/resources/worker_version.html)
- [`takoform_worker_deployment`](/docs/resources/worker_deployment.html)
- [`takoform_worker_custom_domain`](/docs/resources/worker_custom_domain.html)
- [`takoform_worker_endpoint`](/docs/resources/worker_endpoint.html)
- [`takoform_worker_cron_trigger`](/docs/resources/worker_cron_trigger.html)
- [`takoform_edge_kv_namespace`](/docs/resources/edge_kv_namespace.html)
- [`takoform_sqlite_database`](/docs/resources/sqlite_database.html)
- [`takoform_sqlite_migration_set`](/docs/resources/sqlite_migration_set.html)
- [`takoform_sqlite_migration_application`](/docs/resources/sqlite_migration_application.html)
- [`takoform_at_least_once_queue`](/docs/resources/at_least_once_queue.html)
- [`takoform_queue_consumer`](/docs/resources/queue_consumer.html)
- [`takoform_durable_workflow`](/docs/resources/durable_workflow.html)
- [`takoform_actor_namespace`](/docs/resources/actor_namespace.html)

The independent Registry-published Provider 3 uses these resource names
without making them normative or changing Form maturity:

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = ">= 3.0.0"
    }
  }
}
```

Provider 2.1.1 remains available for the exact retained Beta identities it
shipped; Provider 3's 31 mappings are retained compatibility history and must
not be read as the active standalone publisher's 16-Form candidate set.

## Release cadence and gates

API patch releases are defect-only and may be batched. Minor checkpoints are at
most monthly and may be skipped. A major checkpoint is normally at most annual
and requires a concrete incompatibility and migration plan. Before publication,
the owning source is reviewed and its release gate passes; after publication,
the checkpoint is read-only. Form definition and package releases follow the
publisher's independent cadence, while Provider releases occur only when its
typed adapter/compatibility surface changes.

## Publisher parity at the Form/Package boundary

Official and external/third-party publishers are equal at the Form and Form
Package publication boundary. Only the publisher identity and domain differ;
the contract, verification, trust/admission, lifecycle, and authority rules do
not. An official namespace confers no stronger semantics. Operators explicitly
choose the trusted source, issuer, signature/revocation policy, and Host Support
policy; publisher provenance is not part of FormRef equality.

## Withdrawn epochs

The pre-Beta epochs and their documentation were withdrawn
([decision 0042](/spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.html)).
The withdrawn resource pages no longer exist on this site; the identities are
recorded as retired in the published ledgers, and the bytes stay in the
repository's git history and release tags. Existing users of the withdrawn
resources keep an exact pin (`= 2.0.0`, `= 1.0.3` for v1 state)
or follow the [v2 to v3 migration boundary](/release/migrations/v2-to-v3.html):
stay pinned, remove from state, or destroy while still pinned. Nothing is
migrated automatically, and upgrading past the withdrawal with one of the nine
still in state fails closed before any lifecycle request.

<StatusNote />
