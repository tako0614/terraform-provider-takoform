# Takoform neutral Core and compiled Snapshot

This document is normative source for the next compatible Takoform
Specification minor. It defines the family-neutral compiler boundary; it does
not publish a Core SDK, assign a Go module path, release a Form Package, select
a publisher, or mint a Host API lane.

## Purpose

The Core turns already acquired, data-only contract artifacts into one closed,
immutable graph that a Host, client adapter, or conformance runner can consume.
It owns the portable meaning that must be identical in all of those consumers:

- RFC 8785 and I-JSON canonicalization;
- complete Form Package closure verification and exact identity checks;
- Definition/schema validation and closed mechanism vocabulary;
- exact cross-Form, Interface, and Binding reference closure;
- closed Host API lower-bound compatibility;
- recursive portable-default materialization;
- explicit default-create selection;
- deterministic ordering, diagnostics, and Snapshot identity.

It owns no official family roster, Terraform resource type, Resource instance,
Host implementation, publisher trust decision, executable extension,
credential, target, activation, availability, price, or capacity.

## Boundary before compilation

Artifact acquisition precedes Core compilation. An acquisition Adapter fetches
or reads an immutable package closure. The Core-owned package verifier proves that the
package index names every payload, that byte sizes and digests match, that the
closure is data-only, and that fixtures satisfy the Definition. An operator or
Host admission Adapter then applies its selected publisher identity, signature,
transparency, and revocation policy.

The verifier issues a non-forgeable immutable package capability only after the
complete check succeeds. Core receives:

- a diagnostics-only origin;
- the expected canonical package-index digest;
- that verified package capability, which owns the validated index, canonical
  Definition, complete file inventory, and copies of every verified payload;
- zero or more digest-pinned exact Interface Definitions; and
- zero or more digest-pinned exact Binding Definitions.

Callers cannot construct or deserialize a valid package capability. Core
rechecks the caller's package digest pin and compiles the issued canonical
Definition. Interface and Binding artifacts are independently checked against
their embedded normative schemas and canonical digest pins. Core does not
repeat publisher admission or execute a payload. Origin and publisher identity
never participate in exact contract equality.

The current implementation tracer accepts this semantic closure in memory.
Directory, embedded, OCI, and HTTP-by-digest acquisition are Adapters; they are
not separate compiler paths.

## Compile input

One complete input contains:

1. the exact Host API lane against which the Snapshot is selected;
2. zero or more verified package capabilities with explicit package digest
   pins;
3. zero or more exact Interface Definition artifacts;
4. zero or more exact Binding Definition artifacts; and
5. zero or one explicit default-create FormRef for every selected
   `(group, kind)`.

Input order has no meaning. A family group is a reverse-DNS identity. The same
Kind MAY occur in multiple groups; every lookup and reference is group-first.
There is no Kind-only, newest-version, compatible-version, or latest fallback.

Zero packages and zero default pins are valid. This is the required proof that
Core has no built-in official family.

## Exact graph compilation

Compilation performs these stages in order:

1. validate that the Host API identity is a known served lane;
2. require a verifier-issued complete package capability and match its package
   digest to the explicit pin;
3. revalidate the canonical Definition and exact FormRef carried by the
   verified package;
4. validate and digest every Interface and Binding Definition against its
   normative schema;
5. reject duplicate exact identities, including duplicates whose origin or
   package provenance differs;
6. index the complete Form, Interface, and Binding exact-identity sets;
7. reject a Form whose `requiresHostApi` lower bound the selected lane cannot
   satisfy;
8. resolve every `x-takoform-target-formrefs`, provided/required Interface, and
   accepted/annotated Binding reference against the complete set;
9. verify that each Binding's exact target Interface and projected operations
   exist;
10. require exactly one explicit default-create pin per selected group+kind;
    and
11. freeze stable views ordered by exact contract identity.

Compilation is not incremental. Any failed stage returns diagnostics and no
Snapshot. A caller must never keep a partial result from the valid subset.

The stable diagnostic codes for this boundary are:

- `invalid_input`;
- `invalid_artifact`;
- `digest_mismatch`;
- `duplicate_identity`;
- `unresolved_reference`;
- `ambiguous_default`; and
- `missing_default`; and
- `unsupported_host_api`.

Diagnostics sort by subject, JSON Pointer, code, then message. Code is the
automation contract; human-readable message text may improve without changing
the code. Reordering otherwise identical input MUST produce byte-identical
Snapshot identity and diagnostics.

## Immutable Snapshot

A successful Snapshot owns copies of all bytes and exposes only copied views.
It contains:

- exact FormRefs and selected package digests;
- canonical Form Definition bytes and compiled desired schemas;
- exact Interface and Binding refs plus canonical Definition bytes;
- the closed exact-reference graph across all three contract types;
- explicit default-create pins; and
- the selected Host API lane.

Its digest is calculated over a canonical normalized projection of those
facts. Diagnostics-only origin and publisher identity are excluded. Package
digest remains Snapshot selection/provenance even though it is not part of
FormRef equality.

The minimum operations are:

- enumerate Forms in stable order;
- enumerate Interfaces and Bindings in stable order;
- retrieve canonical Definition bytes by exact FormRef;
- resolve a create default by exact group+kind; and
- validate desired I-JSON against the schema of an exact FormRef; and
- recursively materialize schema defaults into canonical effective desired
  bytes before validation.

A wrong group, version, or schema digest is a miss. No operation falls back to
another FormRef.

## Downstream Adapters

- A Terraform Provider consumes a Snapshot plus a Provider-owned projection
  that names resource types, schemas, state versions, imports, codecs, and
  every readable historical FormRef. A new package never changes a running
  Provider's schema dynamically.
- A conformance runner consumes the same Snapshot plus explicitly selected
  generic, family, Interface/Binding, and composition corpora. Family semantics
  stay outside generic Core.
- A Host consumes a Snapshot only after its own package admission. It separately
  decides implementation support and activation and owns all Resources.

Official Forms and external Forms use the same compile path. An official
namespace is a publisher-policy fact, not a Core enum or import.

## Compatibility and release boundary

This contract is compatible Specification-minor work because it fixes how
existing exact package and Definition identities are compiled; it does not
change their bytes or the closed Host API v1 wire. Specification 1.1 may record
this boundary without minting a Form, package, Provider, Host adoption, or
literal Host API identity.

Adding a required field to a published package or Definition envelope, changing
FormRef equality, adding an executable family hook, or requiring independently
installable Interface/Binding package kinds is a new identity decision and
cannot be backfilled into a closed v1 contract.

During the one-repository stage the implementation is internal. No neutral SDK
is released from `github.com/tako0614/terraform-provider-takoform`. A public SDK
may be released only after the final `github.com/tako0614/takoform` repository,
module, single Specification writer, signer, workflow, and history-transfer
evidence are established.

## Conformance witnesses

The Core boundary is not complete until all of these remain green:

- zero-family compilation;
- two synthetic external groups using the same Kind without collision;
- cross-family exact-reference closure and wrong-digest rejection;
- input-permutation equality for Snapshot and diagnostics;
- no partial Snapshot on failure;
- complete-package forgery resistance and defensive-copy ownership;
- unknown and insufficient Host API refusal;
- exact Interface/Binding closure and immutable Definition views;
- omitted and explicitly written defaults producing identical canonical bytes;
- the official publisher's 31 Forms, 13 Interfaces, and 6 Bindings compiled by
  an integration test outside neutral Core;
- an external package compiled in the same Snapshot with those official
  artifacts, without a Core source change; and
- Provider 3's independently locked public/state/history surface remains
  unchanged.

The non-normative inventory of current sources, generated surfaces and their
post-extraction owners is in
[`../../docs/source-boundary-inventory.md`](../../docs/source-boundary-inventory.md).
