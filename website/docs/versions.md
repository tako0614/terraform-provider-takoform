# Versions and compatibility

Takoform has four independent current version streams: the Host API, each
Form definition, Core/library software, and Provider software. A version on
one axis is not a maturity label for another axis. The Form Package API
identifier is a wire/envelope format, not a fifth product stream.

## Current publisher corpus

The official current corpus is one versionless family,
`edge.forms.takoform.com`, with 16 exact Experimental `0.x` Forms, 7
Interfaces, and 6 Bindings. The source roster is pinned to [`takoform-forms`
commit `3a395e4`](https://github.com/tako0614/takoform-forms/tree/3a395e4d7f9f652a942da52905857fccc41b467e).

| Axis | Current identity | Meaning |
| --- | --- | --- |
| Host API | **`forms.takoform.com/v1`** | Stable wire contract for discovery, exact Form availability, operations, fencing, and errors. |
| Form | Each Form's **`definitionVersion`** | Exact independently versioned contract; the official corpus has 16 Experimental Forms in one family. |
| Core/library | Independent software SemVer | SDK, verifier, compiler, and CLI releases; they do not version the Host API or a Form. |
| Provider | Independent software SemVer | Typed tooling for Forms it explicitly supports; Provider publication does not widen the Form corpus. |
| Form Package | **`packages.forms.takoform.com/v1alpha5`** | Data envelope/wire format for Form packages, not a product release axis. |

The canonical `registry.terraform.io/tako0614/takoform` Provider is an
official-Forms-only tool. Third-party Form packages use the same Host API path
and package/verification contracts under their own namespaces, with explicit
Provider mappings. A module can combine the official Takoform Provider with
other Takoform or industry-standard Providers.

## Provider release history and publication boundary

Provider `3.0.0` is the released Registry implementation. Its immutable
projection contains the historical 8-family/31-resource aggregate; that
release record is not the current publisher corpus. The official-only mapping
for the current Edge16 source is a next-major candidate and is not published,
so no future Provider install claim is made here. Examples requiring
`>=3.1.0` are candidate/unpublished until a release record says otherwise.

| Distribution | Host API | Form projection | Status |
| --- | --- | --- | --- |
| Provider 3.0.0 | Host API v1 | Historical 8-family/31-resource Provider projection | Released Registry artifact; immutable history, not the current publisher roster. |
| Future official-only Provider | Host API v1 | Edge16 publisher corpus | Next-major candidate; unpublished and not installable. |
| Provider 2.1.1 | Host API v1beta1 | Retained Edge v1beta1 identities | Immutable historical Registry client; exact pin only. |
| Provider 2.0.0 / 1.0.3 | Withdrawn pre-Beta epochs | Retired identities | Immutable history for recovery and migration. |

Provider release, Form publication, and host availability are independent
facts. A published Provider mapping does not publish a Form Package or change
Form maturity.

## Current Edge Form links

The 16 current Forms are:

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

## Specification receipts and withdrawn lanes

Specification 1.0/1.1 receipts are archive evidence, not a current version
lane. They do not mint an API v1.1 route, Form `1.0.0`, or Provider release.
The pre-Beta Host API epochs and their Provider clients remain available only
under exact historical pins for recovery and migration; see the
[historical release records](/release/) and the
[v2 to v3 migration boundary](/release/migrations/v2-to-v3.html).

<StatusNote />
