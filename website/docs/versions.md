---
title: Version model
description: Four independent version streams define the Takoform compatibility boundary.
---

# Version model

Takoform has exactly four current version streams. The first two describe
domain compatibility; the latter two are independent software releases.

| Stream       | Current form                    | Owner and meaning                                                                                                      |
| ------------ | ------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| Host API     | `forms.takoform.com/v1`         | The literal discovery and operation lane. Compatible changes stay inside this lane; there is no `/v1.1` route.         |
| Form         | Each Form's `definitionVersion` | The exact service shape and lifecycle contract. A family is versionless; each Form carries its own definition version. |
| Core library | `v1.1.0`                        | The independently released Core module/library. Its SemVer does not set a Host API or Provider version.                |
| Provider     | `3.0.0`                         | This repository's Registry-published typed mapping. Its SemVer does not publish a Form or host capability.             |

## What is not a version stream

The Specification 1.1 receipt is a one-time historical snapshot of normative
evidence. It is not a current release train and does not create a Host API
lane. The withdrawn Specification 1.0 identity is retained only in history.

Form Package envelopes, schema `$id` values, content digests, record formats,
family labels, and Provider descriptors identify artifacts or publication
state. They do not add a fifth stream.

## Read a Form identity

When a host advertises a Form, read the complete FormRef rather than a package
or Provider label. Its `formId` selects the service shape and its
`definitionVersion` selects that Form's contract revision. A content digest
then identifies the exact bytes that were received. A family label groups
related Forms for discovery but remains versionless.

The Provider compiles a typed surface for the FormRefs it supports. It does
not provide a generic carrier for an unknown Form, and a Provider release does
not silently upgrade a Form definition.

## Provider release history

The following are immutable distribution records, not additional current
streams:

| Provider release | Historical role                                                                 |
| ---------------- | ------------------------------------------------------------------------------- |
| `3.0.0`          | Current Registry-published typed mapping for the current 31 Experimental Forms. |
| `2.1.1`          | Retained compatibility client for the historical Host API v1beta1 lane.         |
| `2.0.0`          | Retained client for the withdrawn v1alpha2 epoch.                               |
| `1.0.3`          | Retained Legacy client for the withdrawn v1alpha1 epoch.                        |

Existing state that depends on a withdrawn epoch remains pinned to its exact
Provider release. Follow the [v2 to v3 migration boundary](/release/migrations/v2-to-v3.html)
when moving that state; there is no automatic migration.

## Version checks in practice

1. Confirm the host's literal Host Support Profile and `forms.takoform.com/v1` endpoint.
2. Confirm the Form's `formId` and `definitionVersion` before writing its fields.
3. Pin the Core library SemVer used by the host integration.
4. Pin the Provider SemVer in Terraform or OpenTofu and inspect the plan.

The [reference landing page](/docs/reference-landing.html) links each current
resource. The [history page](/docs/history.html) explains the retained
Specification evidence and old URLs.

<StatusNote />
