---
page_title: "Takoform glossary"
description: "Common terms used across the Takoform documentation and specification"
---

# Glossary

Common terms used across the Takoform documentation. Definitions here are
summaries; the normative meaning always lives in the
[specification](/spec/) documents.

| Term | Meaning |
| --- | --- |
| **Form** | One portable desired-state contract: a named, versioned definition of what a workload needs from any host. |
| **FormRef** | The exact immutable identity of a Form: versionless reverse-DNS family group, `kind`, definition version, and schema digest. Compatibility is never inferred from a name alone. |
| **Form Definition** | The deterministic, data-only JSON document describing one Form: desired/observed/output schemas, immutable fields, lifecycle capabilities, and interface descriptors ([form definition](/spec/form-definition/)). |
| **Form Package** | A closed, data-only distribution of one Form Definition plus its fixtures, bound by a canonical content digest ([form package](/spec/form-package/)). No executable or credential content is allowed. |
| **schemaDigest** | SHA-256 over the RFC 8785 canonical Form Definition bytes. |
| **packageDigest** | SHA-256 over the RFC 8785 canonical package index; it is the publication locator (`sha256-<hex>`) of a package. |
| **Epoch** | The API-group boundary of the specification. The pre-Beta epochs (`forms.takoform.com/v1alpha1` and `/v1alpha2`) were withdrawn ([decision 0042](/spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.html)); current design work uses namespaced Form Family groups instead of a new bare epoch. |
| **Form Family** | A versionless namespaced Form group ([Form Families](/spec/form-families.html)) such as `edge.forms.takoform.com`: related Forms sharing exact Interface and Binding contracts. The current candidate has eight families and 31 Experimental `0.x` FormRefs; Edge contains 16. Family membership and Specification 1.0 grant no Form maturity. |
| **Resource Role** | The closed role enum — `identity`, `revision`, `deployment`, `attachment`, `policy` — deciding one Form's lifecycle mechanics: revisions are immutable, deployments move traffic, attachments activate inward events. |
| **Envelope** | The package API version used to carry a Form. Current versionless-group candidates use `packages.forms.takoform.com/v1alpha5`; retained Provider 2.1.1/v1beta1 packages keep `v1alpha4`, and earlier envelope identities remain immutable history. |
| **Space** | An opaque, case-sensitive scope identity (`SpaceID`) that scopes Resources, Interface reads, idempotency, and provider configuration. |
| **Host** | Any implementation of the Host API that advertises exact FormRefs, executes the portable lifecycle, and owns placement, credentials, and recovery. |
| **Host API** | The versioned HTTP contract ([host API](/spec/host-api/)): discovery, exact Form availability, preview/apply, read/import/observe/delete, fencing, and stable errors. The current Specification wire is `forms.takoform.com/v1`; v1beta4 is a retained pre-v1 design snapshot and v1beta1 remains the immutable Provider 2.1.1 lane. |
| **Discovery** | The versioned well-known endpoint at which a host advertises its API, Forms, and features: `/.well-known/takoform/v1` for the current Host API. |
| **Lane** | One exact Host protocol path. Specification 1.0 defines `forms.takoform.com/v1` independently of Provider publication or Form maturity. Provider `3.0.0` is the Registry-published non-normative reference implementation for the current 31 Forms; Provider `2.1.1` remains on retained v1beta1 history. Neither can block the Specification. |
| **Substrate** | The concrete backend or cloud an implementation runs on. Substrate operation is never portable Form state. |
| **Interface declaration** | The withdrawn v1alpha2 lane's read-only `(name, version)` descriptor surface; superseded by exact digest-bound [Interface contracts](/spec/interface-contract/). |
| **Interface contract** | One exact named capability contract a Form's service exposes, such as `edge.kv@1.0.0` ([interface contracts](/spec/interface-contract/)). A service with different semantics is a different contract, never a variant. |
| **Binding contract** | The typed contract behind one outward capability binding a revision holds, such as `module-worker.edge-kv@1.0.0` ([binding contracts](/spec/binding-contract/)). Inward activation (routes, domains, cron, consumption) is an attachment, never a binding. |
| **Standard service** | A sealed Host-resolved runtime slot identified by opaque `standards.takoform.com/v1` plus a reverse-DNS protocol string. Takoform has no central protocol enum and the slot grants no portable lifecycle over a service Resource. |
| **artifact manifest** | The content-addressed manifest (`artifacts.takoform.com/v1alpha1`) that commits a bundle's modules by exact size and sha256 digest through the [artifact transport](/spec/artifact-transport/) upload API. |
| **uid / generation / revision** | The Host API v1 resource identity triple: `uid` is the host-issued immutable resource identity; `generation` increments only when the portable desired spec changes and fences updates; `revision` increments on any representation change and fences deletes. |
| **Proposal** | Mutable, unversioned design material for a Form that has not earned a public FormRef. |
| **Experimental** | A reproducible public Form on a `0.x` line whose semantics may still change under the documented compatibility policy. |
| **Stable** | An evidence-earned Form contract with independent implementations, interoperability, and operational experience. |
| **Legacy** | The lifecycle state after the current line is no longer recommended for new work. The historical Legacy epoch itself was withdrawn ([decision 0042](/spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.html)); its bytes stay in repository history. |
| **Admission** | The withdrawn historical central-evidence process. There is no current central approval or admission. |
| **resource_version** | The withdrawn v2 lane's canonical positive-decimal desired-state version; on Host API v1 the `ETag` is the quoted `revision` and the generation stays the mutation fence. |
| **Idempotency-Key** | A deterministic client-supplied key scoping mutation replay; a retry must reuse the same key and request bytes. |
| **Checkpoint** | A cumulative, hash-chained revocation checkpoint carrying every statement from sequence 1 through the current sequence. |
| **Drift** | A lifecycle outcome derived from validated observed evidence; it is not a separate host operation. |

<StatusNote />
