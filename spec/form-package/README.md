# Data-only Form Package

A Form Package is a closed local directory with a root `package-index.json` and
exactly the payload files listed by that index. Requirement keywords are used
as described in [`../conformance.md`](../conformance.md). The normative Draft
2020-12 index schema is the current
[`v1alpha5`](../schemas/package-index-v1alpha5.schema.json) profile carrying
versionless namespaced family FormRefs. The occupied
[`v1alpha4`](../schemas/package-index-v1alpha4.schema.json) profile remains the
exact `packages.forms.takoform.com/v1alpha4` envelope used by retained Provider
2.1.1/v1beta1 identities. The
three earlier envelope identities were withdrawn with their epochs
([decision 0042](../decisions/0042-the-pre-beta-epochs-are-withdrawn.md));
the [`formpackage`](../../formpackage/) verifier keeps embedded copies of
every epoch's schemas
([`v1alpha3`](../../formpackage/schemas/package-index-v1alpha3.schema.json),
[`v1alpha2`](../../formpackage/schemas/package-index-v1alpha2.schema.json),
[`v1alpha1`](../../formpackage/schemas/package-index.schema.json)) so package
bytes retained in git history and `forms/*` release tags stay verifiable.

One package MUST contain exactly one Form Definition and therefore exactly one
FormRef. There is no `packageId` and no multi-form `definitions` collection in
this contract. A compatibility set, catalog, or host migration map is an
external data object that points to multiple exact `(FormRef, packageDigest)`
pairs; it is not a wider Form Package. Each Form is distributed independently;
one package never carries a catalog-wide version or multiple definitions.

A Form Package carries a Form Definition and nothing else. There is no
Interface Package and no Binding Package: those contracts are digest-bound
documents distributed with this repository, and this envelope cannot carry
one — the index is closed, fixes `kind` to `FormPackage`, and requires a
`formRef`
([decision 0021](../decisions/0021-third-party-forms-and-contract-distribution.md),
[`../interface-contract/`](../interface-contract/README.md)).

## Index and identity

The current family index has the fixed identity
`packages.forms.takoform.com/v1alpha5` / `FormPackage` with an exact
versionless-group `FormRef`. It carries one `definitionPath` and a
lexicographically sorted `files` array. Every file entry records a canonical
relative slash path, an allowlisted data media type, its byte length, and a
lowercase `sha256:` digest over the exact payload bytes.

The package's semantic identity is SHA-256 over the RFC 8785 canonical index.
The index does not list itself. The `FormRef.schemaDigest` separately covers
the RFC 8785 canonical Form Definition. An archive is only transport: its headers and compression MUST NOT contribute
to either identity.

`packageDigest` is the verifier result used by an external catalog or mapping;
it is not a self-referential field inside `package-index.json`.

A content-addressed profile deliberately has no `packageVersion`: the
publication locator uses the full Package Digest as `sha256-<hex>` while
`FormRef.definitionVersion` remains the only Form compatibility SemVer. The
withdrawn v1alpha1/v1alpha2/v1alpha3 profiles remain accepted by the verifier
only for reading, verifying, and recovering bytes retained in history.

## Local verifier

[`../../formpackage/`](../../formpackage/) and
[`../../cmd/form-package/`](../../cmd/form-package/) implement a library and
CLI verifier. Verification performs no network access and executes no package
content. It rejects:

- duplicate JSON names, invalid UTF-8/Unicode, non-finite numbers, and negative
  zero before RFC 8785 canonicalization;
- a missing, duplicate, unsorted, unlisted, or extra payload;
- digest, byte-size, media-type, FormRef, or definition identity mismatches;
- absolute, traversal, backslash, volume/URI-like, or non-canonical paths;
- symlinks, executable mode bits, executable-code extensions, devices,
  sockets, and pipes;
- non-UTF-8/NUL text and all forbidden Form Definition content classes,
  including boundary-delimited singular and plural sensitive field forms;
- portable-schema object admission that cannot be proven closed, cyclic or
  non-local references, and proofs exceeding 64 graph edges or the combined
  4096 schema-node/local-reference operation budget;
- portable schemas whose saturating worst-case validation-work estimate exceeds
  16,384 schema evaluations, including repeated expansion through shared local
  `$ref` DAG edges;
- `contentEncoding`, `contentMediaType`, or `contentSchema`, because portable
  Forms do not decode or transform an embedded second document;
- the legacy `dependencies` applicator (use `dependentRequired` or
  `dependentSchemas`); and
- Form Definitions with more than 32 positive conformance fixtures or more
  than 32 negative conformance fixtures. These are independent per-class
  limits, not one combined 32-fixture budget.

Local `$ref` targets are admitted once per canonical JSON Pointer with explicit
`visiting`/`done` states. Shared acyclic schema graphs therefore cost linear
proof work. The separate validation-work estimate still charges every local
reference occurrence because a fixture validator may revisit the same target
through every branch. Before calling the real validator, a second instance-aware
estimate charges `items`, `contains`, `additionalProperties`, `propertyNames`,
and the corresponding unevaluated keywords once per concrete fixture element or
property. Both estimates saturate at the same 16,384-evaluation limit. Desired
and observed schemas are each compiled once before the bounded fixture loop.
Cycles and resource-exhaustion inputs fail closed.

The published JSON Schemas are normative structural minima. The semantic
identity, filesystem closure, canonicalization, reference-proof,
validation-work, fixture, and portable-content rules enforced here are also
normative. Schema validity alone is therefore necessary but not sufficient for
Form Package conformance.

Allowed payload media types are the Form Definition type, JSON Schema, generic
JSON fixture data, Markdown, and plain text. The verifier limits index, file,
and file-count sizes before reading content.

On Darwin, DragonFly BSD, FreeBSD, Linux, NetBSD, and OpenBSD, the verifier
holds the package root directory descriptor and resolves every payload path
component relative to it. Intermediate components are opened as directories
with `O_NOFOLLOW`; the final component uses `O_NOFOLLOW | O_NONBLOCK` and is
then required to be the same inventoried regular, non-executable file. This
contains payload reads beneath the held root and avoids blocking on a file
swapped to a pipe. Inventory and final metadata fences detect ordinary
mutation, but the verifier does not claim to create an atomic filesystem
snapshot against a malicious concurrent writer.

On other operating systems, callers must copy or extract into an immutable,
private staging directory, close the writer, and only then call the verifier.
The pathname, identity, and metadata fences on those systems are defense in
depth and do not replace that immutable-staging precondition.

```console
go run ./cmd/form-package verify PATH
go run ./cmd/form-package canonicalize FILE.json
go run ./cmd/form-package digest FILE.json
go run ./cmd/form-package validate-revocation STATEMENT.json
go run ./cmd/form-package validate-revocation-checkpoint CHECKPOINT.json
```

## Release boundary

Form Packages have no independent release cadence: they publish with the
provider release that embeds them, when the publication blockers clear
([decision 0041](../decisions/0041-form-packages-publish-with-the-provider-release.md)).
[`cmd/form-package-release`](../../cmd/form-package-release/) remains the
tooling that re-verifies, canonicalizes, deterministically archives, describes
(SPDX 2.3, SLSA v1), and signs a package through a keyless Sigstore v0.3
bundle whose identity is bound to the exact repository and protected-main
workflow; the reversible release ID is `k-` plus lowercase unpadded base32 of
the exact FormRef Kind, and the canonical index bytes — not archive metadata —
are the signed semantic subject. The historical epochs' locator and tag
grammars are recorded with their withdrawal
([decision 0042](../decisions/0042-the-pre-beta-epochs-are-withdrawn.md));
their published `forms/*` tags stay immutable. The append-only revocation
delivery lane (`.github/workflows/form-package-revocation.yml`) remains a
standing workflow, because published bytes stay revocable even after their
epoch is withdrawn.

## Deliberate non-goals of the local verifier

The local verifier does not extract archives, fetch/install remote packages,
verify Sigstore publisher identity, consume revocation feeds, activate a Form,
publish a provider, or execute adapters. Those trust operations stay in the
release or host layer. A package is not publishable merely because this local
verifier accepts it, and a checked-in workflow is not proof that a live release
exists.
