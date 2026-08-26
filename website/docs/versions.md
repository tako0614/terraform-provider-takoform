# Versions and compatibility

Takoform keeps independent Specification, Provider, Host API, Form Family,
Form definition, and Form Package axes. A version on one axis is not a
pre-release label for another axis.

## Current design target

| Axis                  | Current identity                           | Meaning and availability                                                                                                                                                                     |
| --------------------- | ------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Specification         | **Takoform 1.1 candidate**                 | First numbered release; open until one exact committed snapshot of the normative `spec/` tree is recorded. Identity 1.0 was withdrawn before publication and may not be reused. |
| Host API              | **`forms.takoform.com/v1`**                | Stable Specification contract for discovery, exact Form availability, operations, fencing, and errors.                                                                                       |
| Form corpus           | **8 versionless families / 31 Forms**     | Exact current `0.x` FormRefs; all remain Experimental and independently versioned.                                                                                                           |
| Form Package envelope | **`packages.forms.takoform.com/v1alpha5`** | Separate package/distribution identifier. Package artifacts remain unpublished.                                                                                                              |
| Provider              | **3.0.0, Registry-published**              | Independent non-normative reference implementation for all 31 current Forms; Provider 2.1.1 is retained Registry history.                                                                     |

Provider distribution is a separate axis. **Provider 3.0.0** is the current
Registry-published implementation. **Provider 2.1.1**, **Provider 2.0.0**, and
**Provider 1.0.3** remain
installable Registry history for earlier epochs
([decision 0042](/spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.html));
([v2 to v3 migration boundary](/release/migrations/v2-to-v3.html)). Provider 3
cannot block or authorize Specification 1.1.

## Published compatibility mapping

| Client or distribution      | Host API          | Forms and definitions                                               | Status / use                                                                |
| --------------------------- | ----------------- | ------------------------------------------------------------------- | --------------------------------------------------------------------------- |
| Provider 3.0.0 distribution | Host API v1       | 8 versionless families; 31 current Experimental Form identities     | Current Registry-published non-normative reference implementation.          |
| Provider 2.1.1 distribution | Host API v1beta1  | Edge Form Family v1beta1; 15 immutable historical Form identities | Registry-published retained client; descriptor remains `candidate-only` metadata by design. |
| Provider 2.0.0 distribution | Host API v1alpha2 (withdrawn epoch) | The nine withdrawn v1alpha2 Forms | Immutable Registry history; exact-pin only, no successors. |
| Provider 1.0.3 Legacy       | Host API v1alpha1 (withdrawn epoch) | The withdrawn v1 Form Package identities | Immutable Registry history; recovery and migration only.   |

The current Host API v1 contract and versionless Form families are not
interchangeable labels. A Form's own definition SemVer does not change either
the Specification or Provider SemVer, and Specification 1.1 does not silently
mint Form `1.0.0` identities.

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

Provider 2.1.1 remains available for the exact historical identities it
shipped; it must not be read as implementing the current 31-Form corpus.

## Withdrawn epochs

The pre-Beta epochs and their documentation were withdrawn
([decision 0042](/spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.html)).
The withdrawn resource pages no longer exist on this site; the identities are
recorded as retired in the published ledgers, and the bytes stay in the
repository's git history and release tags. Existing users of the withdrawn
resources keep an exact pin (`= 2.0.0` or `= 2.1.1`, `= 1.0.3` for v1 state)
or follow the [v2 to v3 migration boundary](/release/migrations/v2-to-v3.html):
stay pinned, remove from state, or destroy while still pinned. Nothing is
migrated automatically, and upgrading past the withdrawal with one of the nine
still in state fails closed before any lifecycle request.

<StatusNote />
