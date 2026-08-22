# Versions and compatibility

Takoform keeps five independent axes: Provider, Host API, Form Family, Form
definition, and Form Package API/status. A version on one axis is not a
pre-release label for another axis.

## Current design target

| Axis                  | Current identity                           | Meaning and availability                                                                                                                                                                     |
| --------------------- | ------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Provider              | **Provider 2.1.1**                         | Registry-published stable client distribution. The repository descriptor remains `candidate-only` metadata by design after owner publication. |
| Host API              | **Host API v1beta1**                       | Beta protocol: discovery, exact Form availability, operations, fencing, and errors.                                                                                                          |
| Form Family           | **Edge Form Family v1beta1**               | Beta family containing the current 15 Experimental Form definitions.                                                                                                                         |
| Form definition       | **definition 0.1.0**                       | The exact definition version used by each of the 15 current Experimental Forms.                                                                                                              |
| Form Package envelope | **`packages.forms.takoform.com/v1alpha4`** | Separate package/distribution schema identifier. Package artifacts remain unpublished.                                                                                                       |

Provider distribution is a separate axis. **Provider 2.1.1** is the current
Registry-published client. **Provider 2.0.0** and **Provider 1.0.3** remain
installable Registry history for the withdrawn pre-Beta epochs
([decision 0042](/spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.html));
the next release from this repository will be the major **3.0.0**
([v2 to v3 migration boundary](/release/migrations/v2-to-v3.html)).

## Published compatibility mapping

| Client or distribution      | Host API          | Forms and definitions                                               | Status / use                                                                |
| --------------------------- | ----------------- | ------------------------------------------------------------------- | --------------------------------------------------------------------------- |
| Provider 2.1.1 distribution | Host API v1beta1  | Edge Form Family v1beta1; 15 Experimental Forms at definition 0.1.0 | Registry-published current client; descriptor remains `candidate-only` metadata by design. |
| Provider 2.0.0 distribution | Host API v1alpha2 (withdrawn epoch) | The nine withdrawn v1alpha2 Forms | Immutable Registry history; exact-pin only, no successors. |
| Provider 1.0.3 Legacy       | Host API v1alpha1 (withdrawn epoch) | The withdrawn v1 Form Package identities | Immutable Registry history; recovery and migration only.   |

The host API and Form family are not interchangeable labels: Host API v1beta1
is the wire protocol, while Edge Form Family v1beta1 names the group of Forms
that use it. Likewise, definition 0.1.0 is a Form identity and does not change
the Provider 2.1.1 SemVer.

## Current design target — Edge Form Family v1beta1 (15 Experimental Forms)

The current stack is the Host API v1beta1 plus these 15 typed resources. Every
resource is an Experimental Form with definition 0.1.0:

- [`takoform_module_worker`](/docs/resources/module_worker.html)
- [`takoform_worker_bundle`](/docs/resources/worker_bundle.html)
- [`takoform_static_asset_bundle`](/docs/resources/static_asset_bundle.html)
- [`takoform_worker_version`](/docs/resources/worker_version.html)
- [`takoform_worker_deployment`](/docs/resources/worker_deployment.html)
- [`takoform_worker_custom_domain`](/docs/resources/worker_custom_domain.html)
- [`takoform_worker_endpoint`](/docs/resources/worker_endpoint.html)
- [`takoform_worker_cron_trigger`](/docs/resources/worker_cron_trigger.html)
- [`takoform_edge_kv_namespace`](/docs/resources/edge_kv_namespace.html)
- [`takoform_edge_object_bucket`](/docs/resources/edge_object_bucket.html)
- [`takoform_sqlite_database`](/docs/resources/sqlite_database.html)
- [`takoform_sqlite_migration_set`](/docs/resources/sqlite_migration_set.html)
- [`takoform_sqlite_migration_application`](/docs/resources/sqlite_migration_application.html)
- [`takoform_at_least_once_queue`](/docs/resources/at_least_once_queue.html)
- [`takoform_queue_consumer`](/docs/resources/queue_consumer.html)

The current target is actionable from the Registry:

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 2.1.1"
    }
  }
}
```

The repository descriptor remains `candidate-only` metadata by design after
owner publication; that metadata does not revoke the Registry client.

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
