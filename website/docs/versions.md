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
Registry-readback client, **Provider 2.0.0** is the published compatibility
predecessor for the retained surface, and **Provider 1.0.3** remains the
published Legacy client for existing v1 state.

## Published compatibility mapping

| Client or distribution      | Host API          | Forms and definitions                                               | Status / use                                                                |
| --------------------------- | ----------------- | ------------------------------------------------------------------- | --------------------------------------------------------------------------- |
| Provider 2.1.1 distribution | Host API v1beta1  | Edge Form Family v1beta1; 15 Experimental Forms at definition 0.1.0 | Registry-published current client; descriptor remains `candidate-only` metadata by design. |
| Provider 2.0.0 distribution | Host API v1alpha2 | Retained provider-v2 Forms at their retained definition identities  | Registry-published compatibility client.                                    |
| Provider 1.0.3 Legacy       | Host API v1alpha1 | The immutable v1 Form Package identities                            | Registry-published migration and recovery client.                           |

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

## Published compatibility / Migration / History

<details>
<summary>Retained Provider 2.0.0 and Legacy Provider 1.0.3</summary>

### Provider 2.0.0 / Host API v1alpha2 {#provider-2-0-0}

The retained provider-v2 surface remains reachable through the existing
resource URLs. These nine resources are kept as compatibility history, not as
the current Edge Form Family:

- [`takoform_edge_worker`](/docs/resources/edge_worker.html)
- [`takoform_relational_database`](/docs/resources/relational_database.html)
- [`takoform_object_bucket`](/docs/resources/object_bucket.html)
- [`takoform_key_value_store`](/docs/resources/key_value_store.html)
- [`takoform_queue`](/docs/resources/queue.html)
- [`takoform_schedule`](/docs/resources/schedule.html)
- [`takoform_container_service`](/docs/resources/container_service.html)
- [`takoform_stateful_entity`](/docs/resources/stateful_entity.html)
- [`takoform_vector_index`](/docs/resources/vector_index.html)
- [Interface data source](/docs/data-sources/interface.html)

Provider 2.0.0 discovers exact retained FormRefs at
`/.well-known/takoform/v1alpha2`. The retained package index is
`packages.forms.takoform.com/v1alpha3`; neither identity is silently remapped
to the current v1beta1 stack.

### Provider 1.0.3 / Host API v1alpha1 {#provider-1-0-3}

Provider 1.0.3 remains the immutable Legacy client. Keep it pinned for existing
v1 state, refresh/delete/recovery, and any migration step that still needs the
v1 wire. Its discovery boundary is `/.well-known/takoform/v1alpha1`, and the
published v1 Form Package identities remain historical evidence.

### Migration

Migration is explicit rather than an automatic state rewrite:

1. Pin Provider 1.0.3 and refresh the Legacy resource.
2. Record non-secret desired configuration and required public outputs.
3. Create under an exact Host API v1alpha2 FormRef, or import only with host
   conformance proof, using Provider 2.0.0.
4. Move consumers, observe the result, and delete Legacy only after rollback is
   no longer needed.

See the [Provider 1 to 2 migration guide](/release/migrations/v1-to-v2.html)
for the operational sequence. Published identities are immutable: compatibility
means retaining the old URL and meaning, not relabeling it as v1beta1.

</details>

<StatusNote />
