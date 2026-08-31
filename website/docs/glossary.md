---
page_title: "Takoform glossary"
description: "Common terms used across the Takoform documentation and specification"
---

# Glossary

These definitions summarize the current vocabulary. Normative meaning lives in
the [current Host API and Form/Core contracts](/docs/).

| Term | Meaning |
| --- | --- |
| **Form** | One portable desired-state contract: a named, versioned definition of what a workload needs from any host. |
| **FormRef** | The exact immutable identity of a Form: versionless reverse-DNS family group, `kind`, definition version, and schema digest. Compatibility is never inferred from a name alone. |
| **Form Definition** | Deterministic, data-only JSON describing one Form: desired/observed/output schemas, immutable fields, lifecycle capabilities, and interface descriptors ([contract](/spec/form-definition/)). |
| **Form Package** | A closed, data-only distribution of one Form Definition plus fixtures, bound by a canonical content digest ([contract](/spec/form-package/)). It is a distribution envelope, not a product release axis; no executable or credential content is allowed. |
| **schemaDigest** | SHA-256 over the RFC 8785 canonical Form Definition bytes. |
| **packageDigest** | SHA-256 over the RFC 8785 canonical package index; it locates one package (`sha256-<hex>`). |
| **Form Family** | A versionless namespaced Form group, such as `edge.forms.takoform.com`, whose members share exact Interface and Binding contracts. The current official corpus is one Edge family with 16 Experimental `0.x` FormRefs. |
| **Resource Role** | Closed role enum — `identity`, `revision`, `deployment`, `attachment`, `policy` — deciding one Form's lifecycle mechanics. |
| **Envelope** | The package API version used to carry a Form. Current publisher data uses `packages.forms.takoform.com/v1alpha5`; this is a wire/envelope format, not another product version stream. Retained older envelopes remain immutable history. |
| **Space** | Opaque, case-sensitive scope identity (`SpaceID`) that scopes Resources, Interface reads, idempotency, and Provider configuration. |
| **Host** | Any implementation of the Host API that advertises exact FormRefs, executes the portable lifecycle, and owns placement, credentials, and recovery. |
| **Host API** | Versioned HTTP contract ([Host API](/spec/host-api/)) for discovery, exact Form availability, preview/apply, read/import/observe/delete, fencing, and stable errors. The current stable wire is `forms.takoform.com/v1`. |
| **Discovery** | Versioned well-known endpoint at which a host advertises its API, Forms, and features: `/.well-known/takoform/v1` for the current Host API. |
| **Provider** | Terraform/OpenTofu software tooling with typed mappings for the official Forms it explicitly supports. The canonical `tako0614/takoform` Provider is official-Forms-only, not a generic carrier or universal infrastructure provider. Third-party packages use the same Host API path and package/verification contracts under their own namespaces. Modules may combine the official Takoform Provider with other Takoform or industry-standard Providers. |
| **Official Form** | A Form distributed by the official publisher and represented in the canonical Provider's explicit mappings. Independent third parties may distribute Forms under their own namespaces through the same package and verification path. |
| **Specification receipt** | Numbered archive evidence (including Specification 1.0/1.1). A receipt records history; it does not create an API v1.1 route, a Form `1.0.0`, or a current Provider lane. |
| **Interface contract** | One exact named capability contract a Form's service exposes, such as `edge.kv@1.0.0` ([contracts](/spec/interface-contract/)). Different semantics are a different contract. |
| **Binding contract** | Typed contract behind one outward capability binding a revision holds, such as `module-worker.edge-kv@1.0.0` ([contracts](/spec/binding-contract/)). Inward activation (routes, domains, cron, consumption) is an attachment, never a binding. |
| **Standard service** | A sealed Host-resolved runtime slot identified by opaque `standards.takoform.com/v1` plus a reverse-DNS protocol string. Takoform has no central protocol enum and the slot grants no portable lifecycle over a service Resource. |
| **artifact manifest** | Content-addressed manifest (`artifacts.takoform.com/v1alpha1`) committing a bundle's modules by exact size and sha256 digest through the [artifact transport](/spec/artifact-transport/) API. |
| **uid / generation / revision** | Host API v1 resource identity triple: `uid` is immutable host identity; `generation` increments only when portable desired spec changes and fences updates; `revision` increments on any representation change and fences deletes. |
| **Proposal** | Mutable, unversioned design material for a Form that has not earned a public FormRef. Non-Edge candidate proposals are currently historical/deferred. |
| **Experimental** | Reproducible public Form on a `0.x` line whose semantics may still change under the documented compatibility policy. |
| **Stable** | Evidence-earned Form contract with independent implementations, interoperability, and operational experience. |
| **Legacy** | Lifecycle state after the current line is no longer recommended for new work. Historical bytes stay in repository history. |
| **resource_version** | Withdrawn v2 lane's canonical positive-decimal desired-state version; on Host API v1 the `ETag` is the quoted `revision` and `generation` remains the mutation fence. |
| **Idempotency-Key** | Deterministic client-supplied key scoping mutation replay; a retry must reuse the same key and request bytes. |
| **Checkpoint** | Cumulative, hash-chained revocation checkpoint carrying every statement from sequence 1 through the current sequence. |
| **Drift** | Lifecycle outcome derived from validated observed evidence; it is not a separate host operation. |

<StatusNote />
