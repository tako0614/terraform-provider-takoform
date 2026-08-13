---
page_title: "Provider: Takoform"
description: |-
  Reference for the Takoform provider-v1 maintenance line, published v1.0.3, the v1.0.4 candidate, 34 portable Service Form resources, and the read-only Interface data source.
---

# Takoform Provider

Takoform is a thin, statically typed Terraform/OpenTofu provider for the
`forms.takoform.com/v1alpha1` Service Form API. Its canonical provider address
is `registry.terraform.io/tako0614/takoform`.

## Version and scope

- **This documentation is version-bound to the provider-v1 maintenance line.**
  The canonical distribution is
  `registry.terraform.io/tako0614/takoform`; consumers should pin an exact
  published version and let Terraform or OpenTofu verify its signed checksums
  during installation. Unissued `v1.0.4` candidate behavior is identified below.
- **Availability is verified, not declared by this immutable documentation.**
  Provider `v1.0.3` has a signed release and retained authenticated direct
  installation readback from both Terraform and OpenTofu. Check the canonical
  [Registry version endpoint](https://registry.terraform.io/v1/providers/tako0614/takoform/versions)
  for exact version `1.0.3`, then run `terraform init` or `tofu init` with the
  exact pin. A source tag, documentation page, or local build alone is not
  Registry publication or installation evidence.
- **The API remains `forms.takoform.com/v1alpha1`.** Provider SemVer and API
  stability are independent; provider `v1.0.3` does not graduate the API to
  `v1`.
- **All 34 Service Form Packages below are published and immutable.** The
  protected `forms/admissions/v1.0.6` closure admits exactly 10 as
  `portable-standard`: `EdgeWorker`, `ContainerService`, `StatefulEntity`,
  `Schedule`, `ObjectBucket`, `KeyValueStore`, `RelationalDatabase`, `Queue`,
  `VectorIndex`, and `ModelEndpoint`. The remaining 24 are published but not
  admitted.
- **The read-only `takoform_interface` data source is independent of the
  34-Form resource inventory.** Forms declare generic non-secret runtime
  Interfaces in their Form Definitions, the host materializes them, and the
  provider may read them. The provider has no Interface write resource.

`release/version.json` retains `publicationStatus: candidate-only` as
release-descriptor metadata, not live availability state. It does not override
the signed provider release and canonical Registry readback that establish
provider publication. The descriptor currently records unissued candidate
`v1.0.4`, while the latest published provider remains `v1.0.3`. The
append-only provider identity ledger has no `v1.0.4` assignment. The protected
admission tag and offline-authenticated retained closure separately establish
Standard Form admission.

See [versioning](../spec/versioning.md), the
[generated Form inventory](../forms/README.md), and
[conformance status](../conformance/README.md) for the exact evidence boundary.

## Ownership and boundaries

| Surface | Owns | Does not own |
| --- | --- | --- |
| Form | Portable desired-state schema, immutable fields, declared runtime interfaces, and exact versioned identity | Backend selection, placement, credentials, pricing, or host policy |
| Provider | Typed HCL schema, state translation, discovery checks, and Service Form lifecycle calls | Concrete infrastructure, secrets issued by a backend, admission decisions, or runtime authorization |
| Host | Discovery, exact Form availability and admission, realization, observation, backend selection, credentials, policy, and authorization | The portable Form or provider identity |
| Interface declaration | A Form-defined generic `(name, version)` descriptor, non-secret document, and deterministic public value mappings that the host materializes | A provider write path, consumer permission, tokens, bindings, or invocation through the provider |

The provider control-plane `endpoint` is not a runtime service endpoint.
Applications discover an Interface from the host, obtain host-governed
authorization when required, and invoke the resolved runtime endpoint directly.

Every `connections[*].resource` value is only `Kind/name`; it does not carry a
Space. The host resolves it in the source Resource's exact `space`. A target
that exists only in another Space is treated as missing, and apply fails with
`resource_not_found` before changing the source Resource. Cross-Space
composition, if a host offers it, is outside portable Resource desired state.

## Configuration

The following configuration pins published provider `v1.0.3` in the maintained
`v1` compatibility line. The `v1.0.4` candidate is not an installability claim.

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 1.0.3"
    }
  }
}

provider "takoform" {
  endpoint = "https://forms.example.com"
  space    = "prod"
}

resource "takoform_object_bucket" "assets" {
  name             = "assets"
  storage_class    = "standard"
  versioning       = true
  access_protocols = ["s3_api"]
}
```

To verify the source and portable conformance corpus independently:

```console
git clone https://github.com/tako0614/terraform-provider-takoform.git
cd terraform-provider-takoform
git checkout --detach v1.0.3
bun run check
```

The owner gate requires the Go, OpenTofu, and Terraform toolchains documented in
the repository. It tests both CLI lifecycles locally; it is not Registry
installation evidence.

## Provider arguments

- `endpoint` (String, optional) — origin of a conforming Service Form host.
  Falls back to `TAKOFORM_ENDPOINT`; one of the two is required.
- `space` (String, optional) — default space for resources. Falls back to
  `TAKOFORM_SPACE`.
- `token` (String, optional, sensitive) — bearer token for the host. Falls back
  to `TAKOFORM_TOKEN`.

The endpoint must advertise `features.service_forms = true`, API version
`forms.takoform.com/v1alpha1`, its same-origin versioned endpoint, and exact
availability for every build-pinned FormRef used by the
configuration. There is no unversioned fallback.

## Recorded Form identity maintenance candidate

Provider `v1.0.4` retains the exact DB2 and Edge3 references and closed codecs
written by provider `v1.0.2`. Read, ordinary Update, and Delete dispatch only
through all five Form identity fields in state; an unknown or incomplete
identity fails before a Resource host request. Neither a kind match nor SemVer
selects a replacement codec.

Only `takoform_relational_database` adds the optional closed declaration
`form_transition = "relational-database-v2-to-v3"`. It requests exact
DB2-to-DB3 same-resource transition from a host that advertises
`resource_form_transition`. State remains DB2 until exact committed operation,
request, Resource, desired-spec, revision, Form-pair, and native-identity proof
is returned. An uncertain acknowledgement is reconciled by readback and is
never permission to repeat the POST blindly. A fresh database create and state
already proved as DB3 treat the marker as inert. Edge artifact-only updates
remain on recorded Edge3. See the
[`v1.0.2` recorded-Form transition guide](../release/migrations/v1.0.2-to-v1.0.4-recorded-form-transition.md).

## Compute and application

| Resource | Intent |
| --- | --- |
| [`takoform_edge_worker`](resources/edge_worker.md) | Edge/event-driven application from a prebuilt immutable artifact |
| [`takoform_container_service`](resources/container_service.md) | OCI container service pinned to an immutable image digest |
| [`takoform_compute_instance`](resources/compute_instance.md) | Long-running machine from digest-bound boot-image bytes |
| [`takoform_static_site`](resources/static_site.md) | Static asset site from a prebuilt immutable artifact |
| [`takoform_workflow`](resources/workflow.md) | Durable workflow definition and instance-state lifecycle |
| [`takoform_stateful_entity`](resources/stateful_entity.md) | Addressable persistent entities implemented by digest-bound application bytes |
| [`takoform_schedule`](resources/schedule.md) | Cron lifecycle invoking exactly one connected Resource |

## Data and storage

| Resource | Intent |
| --- | --- |
| [`takoform_object_bucket`](resources/object_bucket.md) | Object storage with a portable default storage class |
| [`takoform_object_lifecycle_rule`](resources/object_lifecycle_rule.md) | One expiration or storage-transition action |
| [`takoform_key_value_store`](resources/key_value_store.md) | Key/value state with an optional consistency preference |
| [`takoform_cache_cluster`](resources/cache_cluster.md) | In-memory cache sized by an open capability token |
| [`takoform_relational_database`](resources/relational_database.md) | Relational database addressed through an open engine token |
| [`takoform_indexed_store`](resources/indexed_store.md) | Bounded item store with declared queryable attributes |
| [`takoform_queue`](resources/queue.md) | At-least-once asynchronous delivery |
| [`takoform_stream_topic`](resources/stream_topic.md) | Published event stream for independent consumers |
| [`takoform_search_index`](resources/search_index.md) | Full-text index over declared document fields |
| [`takoform_vector_index`](resources/vector_index.md) | Vector index with lifecycle-fixed dimensions |

## Analytics and inference

| Resource | Intent |
| --- | --- |
| [`takoform_analytics_dataset`](resources/analytics_dataset.md) | Append-oriented dataset queried for analysis |
| [`takoform_model_endpoint`](resources/model_endpoint.md) | Inference endpoint serving digest-bound model bytes |

## Network and delivery

| Resource | Intent |
| --- | --- |
| [`takoform_dns_zone`](resources/dns_zone.md) | Authoritative DNS zone |
| [`takoform_dns_record`](resources/dns_record.md) | DNS record in one connected zone |
| [`takoform_tls_certificate`](resources/tls_certificate.md) | Managed TLS certificate; key material stays with the host |
| [`takoform_http_route`](resources/http_route.md) | Hostname/path binding to a connected Resource |
| [`takoform_load_balancer`](resources/load_balancer.md) | Listener distributing connections across backends |
| [`takoform_private_network`](resources/private_network.md) | Private address space for attached Resources |

## Operations and integration

| Resource | Intent |
| --- | --- |
| [`takoform_container_registry`](resources/container_registry.md) | OCI artifact registry namespace |
| [`takoform_log_sink`](resources/log_sink.md) | Structured application-log destination |
| [`takoform_metric_sink`](resources/metric_sink.md) | Numeric time-series destination |
| [`takoform_email_sender`](resources/email_sender.md) | Outbound mail identity for one verified domain |
| [`takoform_webhook_endpoint`](resources/webhook_endpoint.md) | Inbound HTTP endpoint forwarding to a connected Resource |
| [`takoform_identity_client`](resources/identity_client.md) | OIDC relying-party registration |
| [`takoform_feature_flag`](resources/feature_flag.md) | Named runtime switch with a complete enabled percentage |
| [`takoform_rate_limit_policy`](resources/rate_limit_policy.md) | Request budget applied to a connected Resource |
| [`takoform_backup_policy`](resources/backup_policy.md) | Scheduled copy and retention rule |

## Generic Interface data source

- [`takoform_interface` data source](data-sources/interface.md) — reads one
  exact host-materialized Interface declaration, failing closed when selection
  is ambiguous.

Interface names and versions are open and author-defined. They are not a central
allowlist. A Form declares them in its Form Definition; the provider does not
create them. Documents and resolved values must remain non-secret, and the host
retains all lifecycle and authorization authority.

## Import

Every Service Form resource supports both forms:

```console
terraform import takoform_object_bucket.assets NAME
terraform import takoform_object_bucket.assets SPACE/NAME
```

The second form records the resource space explicitly.
