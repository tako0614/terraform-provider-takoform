---
title: History
description: Retained Specification receipts, withdrawn epochs, and immutable Provider releases.
---

# History

This page is historical evidence, not a second version model. Current readers
should start with the [four-stream version model](/docs/versions.html).

## Specification receipt — historical evidence only

The numbered **Specification 1.1** receipt records one immutable snapshot of
normative evidence. It remains useful when auditing an old decision or
reproducing a historical build. It is not a current release train, does not
promote a Form, and does not create a Host API `/v1.1` lane.

The pre-publication **Specification 1.0** identity was withdrawn. Neither
number is a current user-facing version stream. The retained [Specification
source](/spec/) is grouped separately in navigation and carries the same
historical notice on every old URL.

## Provider distributions

Provider packages remain installable under exact pins so existing state can be
recovered or migrated without rewriting published identity:

| Release | Retained role                                                 |
| ------- | ------------------------------------------------------------- |
| `3.0.0` | Current Registry-published mapping for 31 Experimental Forms. |
| `2.1.1` | Historical Host API v1beta1 compatibility client.             |
| `2.0.0` | Historical client for the withdrawn v1alpha2 epoch.           |
| `1.0.3` | Historical Legacy client for the withdrawn v1alpha1 epoch.    |

Old packages and resource identities are not reassigned to a new Form. Pin the
release that owns existing state, then follow the [v2 to v3 migration
boundary](/release/migrations/v2-to-v3.html). There is no automatic migration.

## Preserved routes

The old routes are deliberately still addressable:

- `/spec/` and its nested contract pages preserve the historical source tree.
- `/release/` preserves publication and migration evidence.
- `/docs/reference.html` remains the generated legacy reference projection.

Each route now points back to the current version model from a historical
banner. The current [reference landing page](/docs/reference-landing.html) is
the entry point for the live Provider surface.

<StatusNote />
