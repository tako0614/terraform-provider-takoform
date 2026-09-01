# Provider v3 to v4 publisher-set boundary

Provider `3.0.0` is immutable Registry history for a 31-resource aggregate.
Provider `4.0.0` at the same `registry.terraform.io/tako0614/takoform` source
address registers only the 16 exact Forms selected from the
`github.com/tako0614/takoform-forms` publisher in the
`edge.forms.takoform.com` family. It is a candidate descriptor until an
immutable Registry readback records publication.

## 15 withdrawn aggregate resource types

The following Provider 3 resource types are not part of the publisher-selected Provider
roster:

- `takoform_container_custom_domain`
- `takoform_container_endpoint`
- `takoform_container_revision`
- `takoform_container_traffic`
- `takoform_serverless_container_service`
- `takoform_function`
- `takoform_function_deployment`
- `takoform_function_endpoint`
- `takoform_function_version`
- `takoform_pull_queue`
- `takoform_message_schedule`
- `takoform_table`
- `takoform_topic`
- `takoform_topic_subscription`
- `takoform_dense_vector_index`

Their Provider 3 FormRef, codec, package, release, and Registry evidence remains
immutable history. They are not renamed, reoccupied, or described as current
publisher-selected Forms.

## State transition

An operator with any removed resource type in state must choose explicitly
before upgrading:

1. Keep `version = "= 3.0.0"` and continue using the retained Provider binary.
2. Destroy the remote object while pinned to Provider 3, then remove the old
   configuration.
3. Use `tofu state rm` only when intentionally leaving the remote object
   unmanaged.
4. Create or import the replacement with its owning OpenTofu provider, verify
   it, then remove the old Provider 3 state entry.

Do not use `tofu state replace-provider` to turn a removed Takoform resource
into an AWS, Cloudflare, Kubernetes, or other provider resource. Provider
addresses, resource types, schemas, identities, and lifecycle semantics differ;
blind replacement would ask a different provider to decode incompatible state.

Upgrading with a removed resource still in configuration or state fails before
Takoform lifecycle mutation because the next major does not register its type.

## Native provider composition

Industry-standard and provider-native resources stay with their owning
providers. A module may declare `takoform` together with AWS, Cloudflare,
Kubernetes, PostgreSQL, or any other runner-installable provider. OpenTofu owns
provider installation, aliases, graph edges, plan/state, and dependency order.
Takoform neither wraps those resources as Forms nor acts as a provider catalog.

The repository includes an
[AWS plus Takoform example](../../examples/native-provider-composition/main.tf).
