# 0056 — Core compiles artifacts into an immutable Snapshot

- Status: Accepted
- Date: 2026-08-25
- Scope: neutral Core, Form publisher, Provider and conformance dependency boundary

## Context

The repository already validates data-only Form Packages and represents exact
FormRefs, but current generation still begins in Go family catalogs. Aggregate
generators, the official Provider, generated registries and parts of
conformance import or enumerate the official families directly. A third-party
package can be verified in isolation yet cannot join the same complete graph
without source changes.

Moving those directories to separate repositories first would leave the same
fixed roster behind an inter-repository build. Loading family callbacks or Go
plugins would make publishers executable extensions and make Core portability
depend on language/runtime identity. A generic Terraform resource carrying
opaque JSON would avoid a roster only by giving up typed schema and state
fidelity.

## Decision

Introduce one artifact-first neutral compiler as specified in
[`../core/`](../core/). The Core-owned verifier issues a non-forgeable package
capability only after checking the complete data-only package closure. The
compiler consumes those capabilities, exact Interface/Binding Definition
artifacts and explicit default pins, then returns either one immutable Snapshot
or stable diagnostics with no Snapshot.

Core imports no official family, Terraform Provider, Host, or conformance
implementation. Official and external publishers enter through the same
artifact interface. Exact identity is always `(group, kind,
definitionVersion, schemaDigest)`; publisher and package provenance do not
change FormRef equality. Same-Kind Forms in different groups coexist, and all
resolution is group-first with no fallback.

Provider and conformance become downstream Adapters. The Provider additionally
requires its own exact projection artifact and retains every historical state
codec it promises. Family semantic corpora and executable Host behavior remain
outside generic Core.

The compiler also owns the closed Host API lane ordering needed to enforce a
Form's `requiresHostApi` lower bound and the generic recursive materialization
of JSON Schema defaults into canonical effective desired bytes. Neither rule is
a family or Provider extension point.

During the current one-repository stage the Go tracer is internal and carries
no public SDK/module identity. Physical extraction and publication occur only
after artifact parity and deletion gates close.

## Consequences

- Zero installed families becomes a first-class valid Snapshot rather than a
  special test configuration.
- Adding or removing a selected family changes artifact/default inputs, not
  Core source.
- Dual old/new execution is temporary zero-diff evidence. It must end by
  deleting the fixed rosters and generators; it is not a compatibility layer.
- Package acquisition remains an Adapter; complete byte/fixture/content closure
  verification belongs to Core's verifier. Signatures, publisher policy and
  revocation remain admission concerns. The compiler accepts only a
  verifier-issued capability and still enforces the caller's digest pin.
- Terraform names, schemas, imports, state and diagnostics stay Provider-owned.
- Provider 3.0.0 public/state/history behavior is locked before the Provider
  Adapter changes.
- Specification 1.1 may record this compatible Core/API boundary independently
  from Forms, packages, Provider, Host adoption and literal Host API v1
  graduation.

## Rejected alternatives

- a built-in official-family registry;
- family callbacks, Go plugins, WASM extensions, or package executable code;
- Kind-only or latest-version resolution;
- publisher identity inside FormRef equality;
- a generic opaque primary Terraform resource;
- repository extraction before dependency parity;
- a permanent fallback to the old generator.
