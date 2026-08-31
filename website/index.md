---
layout: home
title: Takoform
description: Portable resource contracts for Terraform and OpenTofu, with an explicit four-stream version model.

hero:
  name: Takoform
  text: Portable contracts for real infrastructure.
  tagline: A typed Provider for Terraform and OpenTofu. The host keeps implementation, placement, credentials, and routing.
  actions:
    - theme: brand
      text: Get started
      link: /docs/
    - theme: alt
      text: Read the version model
      link: /docs/versions.html
---

## One contract, four independent streams

Takoform separates the wire contract, the Form definition, the Core library,
and the Provider release. The same table is the quickest way to orient a new
reader.

| Stream       | Current form                    | What it identifies                                        |
| ------------ | ------------------------------- | --------------------------------------------------------- |
| Host API     | `forms.takoform.com/v1`         | The literal discovery and operation lane.                 |
| Form         | Each Form's `definitionVersion` | The exact service shape a host implements.                |
| Core library | `v1.1.0`                        | The independently released Core module and library.       |
| Provider     | `3.0.0`                         | The Registry-published typed mapping for this repository. |

Form Package envelopes, schema IDs, content digests, and family labels identify
artifacts. Their publication remains unpublished; that availability fact is not
an additional version stream.

## Start with the Provider

Install the Provider from the Registry and point it at a host that publishes a
Host Support Profile. Provider release is implementation metadata; it does not
make a host capability or a Form stable by implication.

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "~> 3.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://host.example.com"
  space    = "prod"
}
```

[Follow the five-minute setup](/docs/) or open the [reference landing
page](/docs/reference-landing.html) for resource links.

## Shape-preserving Forms

Each Form describes one complete service shape: its execution ABI, consistency,
delivery guarantees, update units, and lifecycle. A host supplies placement,
capacity, credentials, routing, recovery, and other operational concerns. Two
implementations with different semantics are different Forms; the host is what
can be exchanged.

The current Provider maps 31 Experimental Forms in eight versionless families.
Open the [reference landing page](/docs/reference-landing.html) for the
family index, then follow an individual resource page for its exact fields.

<details>
<summary>Current resource names</summary>

`takoform_serverless_container_service`, `takoform_container_revision`,
`takoform_container_traffic`, `takoform_container_endpoint`,
`takoform_container_custom_domain`, `takoform_module_worker`,
`takoform_worker_bundle`, `takoform_static_asset_bundle`,
`takoform_worker_version`, `takoform_worker_deployment`,
`takoform_worker_custom_domain`, `takoform_worker_endpoint`,
`takoform_worker_cron_trigger`, `takoform_edge_kv_namespace`,
`takoform_sqlite_database`, `takoform_sqlite_migration_set`,
`takoform_sqlite_migration_application`, `takoform_at_least_once_queue`,
`takoform_queue_consumer`, `takoform_durable_workflow`,
`takoform_actor_namespace`, `takoform_function`, `takoform_function_version`,
`takoform_function_deployment`, `takoform_function_endpoint`,
`takoform_pull_queue`, `takoform_message_schedule`, `takoform_table`,
`takoform_topic`, `takoform_topic_subscription`, `takoform_dense_vector_index`

</details>

## Historical evidence, kept out of the current model

The numbered Specification 1.1 receipt and the withdrawn 1.0 identity are
retained as immutable historical evidence. They are not a live release train,
an API lane, or a fifth stream. The old [Specification routes](/spec/) and
[release evidence](/release/) remain available with a historical notice, while
the [history page](/docs/history.html) explains how to read them.

<StatusNote />
