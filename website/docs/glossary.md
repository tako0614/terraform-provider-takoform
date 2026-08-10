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
| **FormRef** | The exact immutable identity of a Form: API group, `kind`, definition version, and schema digest. Compatibility is never inferred from a name alone. |
| **Form Definition** | The deterministic, data-only JSON document describing one Form: desired/observed/output schemas, immutable fields, lifecycle capabilities, and interface descriptors ([form definition](/spec/form-definition/)). |
| **Form Package** | A closed, data-only distribution of one Form Definition plus its fixtures, bound by a canonical content digest ([form package](/spec/form-package/)). No executable or credential content is allowed. |
| **schemaDigest** | SHA-256 over the RFC 8785 canonical Form Definition bytes. |
| **packageDigest** | SHA-256 over the RFC 8785 canonical package index; it is the publication locator (`sha256-<hex>`) of a package. |
| **Epoch** | The API-group boundary of the specification: `forms.takoform.com/v1alpha1` is the frozen Legacy epoch; `forms.takoform.com/v1alpha2` is retained as the provider-v2 epoch. Current design work uses namespaced Form Family groups instead of a new bare epoch. |
| **Form Family** | A namespaced Form group ([Form Families](/spec/form-families.html)) such as `edge.forms.takoform.com/v1beta1`, the Edge Platform Family: related Forms sharing exact Interface and Binding contracts. Its 15 current Forms are Experimental; family membership alone grants no maturity. |
| **Resource Role** | The closed v1beta1 role enum — `identity`, `revision`, `deployment`, `attachment`, `policy` — deciding one Form's lifecycle mechanics: revisions are immutable, deployments move traffic, attachments activate inward events. |
| **Envelope** | The package API version used to carry a Form: `packages.forms.takoform.com/v1alpha4` for the current family lane; `packages.forms.takoform.com/v1alpha3` retained for provider-v2 packages. |
| **Space** | An opaque, case-sensitive scope identity (`SpaceID`) that scopes Resources, Interface reads, idempotency, and provider configuration. |
| **Host** | Any implementation of the Host API that advertises exact FormRefs, executes the portable lifecycle, and owns placement, credentials, and recovery. |
| **Host API** | The versioned HTTP contract ([host API](/spec/host-api/)): discovery, exact Form availability, preview/apply, read/import/observe/refresh/delete, fencing, and stable errors. The current wire is `forms.takoform.com/v1beta1`; `forms.takoform.com/v1alpha2` is the retained provider-v2 wire and v1alpha3 is immutable retained history. |
| **Discovery** | The versioned well-known endpoint at which a host advertises its API, Forms, and features: `/.well-known/takoform/v1beta1` for the current Host API; `/.well-known/takoform/v1alpha2` retained for provider v2. |
| **Lane** | A usage path through the single provider address: published `v1.0.3` for Legacy state and recovery; published `v2.0.0` for the nine retained provider-v2 Forms; stable target `v2.1.0` (`candidate-only` until owner publication) for the Experimental Edge Platform Family over the v1beta1 Host API. |
| **Substrate** | The concrete backend or cloud an implementation runs on. Substrate operation is never portable Form state. |
| **Interface declaration** | A read-only `(name, version)` descriptor embedded in a Form Definition, projected by the host after the Resource is Ready ([interface declaration](/spec/interface-declaration/)). |
| **Interface contract** | One exact named capability contract a Form's service exposes, such as `edge.kv@1.0.0` ([interface contracts](/spec/interface-contract/)). A service with different semantics is a different contract, never a variant. |
| **Binding contract** | The typed contract behind one outward capability binding a revision holds, such as `module-worker.edge-kv@1.0.0` ([binding contracts](/spec/binding-contract/)). Inward activation (routes, domains, cron, consumption) is an attachment, never a binding. |
| **artifact manifest** | The content-addressed manifest (`artifacts.takoform.com/v1alpha1`) that commits a bundle's modules by exact size and sha256 digest through the [artifact transport](/spec/artifact-transport/) upload API. |
| **uid / generation / revision** | The v1beta1 resource identity triple: `uid` is the host-issued immutable resource identity; `generation` increments only when the portable desired spec changes and fences updates; `revision` increments on any representation change and fences deletes. |
| **Proposal** | Mutable, unversioned design material for a Form that has not earned a public FormRef. |
| **Experimental** | A reproducible public Form on a `0.x` line whose semantics may still change under the documented compatibility policy. |
| **Stable** | An evidence-earned Form contract with independent implementations, interoperability, and operational experience. |
| **Legacy** | An immutable published identity retained for compatibility, recovery, and explicit migration but no longer the basis for new specification work. |
| **Admission** | The retired historical central-evidence process. There is no current central approval or admission. |
| **resource_version** | The retained v2 lane's canonical positive-decimal desired-state version; its quoted value is that lane's optimistic-concurrency fence (`ETag`). On v1beta1 the `ETag` is instead the quoted `revision`. |
| **Idempotency-Key** | A deterministic client-supplied key scoping mutation replay; a retry must reuse the same key and request bytes. |
| **Checkpoint** | A cumulative, hash-chained revocation checkpoint carrying every statement from sequence 1 through the current sequence. |
| **Drift** | A lifecycle outcome derived from validated observed evidence; it is not a separate host operation. |

<StatusNote />
