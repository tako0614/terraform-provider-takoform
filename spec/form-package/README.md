# Data-only Form Package v1alpha1

A Form Package is a closed local directory with a root `package-index.json` and
exactly the payload files listed by that index. The normative Draft 2020-12
index schema is
[`../../formpackage/schemas/package-index.schema.json`](../../formpackage/schemas/package-index.schema.json).

One package contains exactly one Form Definition and therefore exactly one
FormRef. There is no `packageId` and no multi-form `definitions` collection in
this contract. A compatibility set, catalog, or host migration map is an
external data object that points to multiple exact `(FormRef, packageDigest)`
pairs; it is not a wider Form Package. For example, extracting the current ten
provider candidates requires ten independent packages, using valid SemVer such
as `0.0.0-legacy.1`, rather than one package carrying ten definitions or a
non-SemVer `legacy-v1` version.

## Index and identity

The index has the fixed identity
`packages.forms.takoform.com/v1alpha1` / `FormPackage`, a SemVer
`packageVersion`, an exact `FormRef`, one `definitionPath`, and a
lexicographically sorted `files` array. Every file entry records a canonical
relative slash path, an allowlisted data media type, its byte length, and a
lowercase `sha256:` digest over the exact payload bytes.

The package's semantic identity is SHA-256 over the RFC 8785 canonical index.
The index does not list itself. The `FormRef.schemaDigest` separately covers
the RFC 8785 canonical Form Definition. An archive is only transport and its
headers or compression do not contribute to either identity.

`packageDigest` is the verifier result used by an external catalog or mapping;
it is not a self-referential field inside `package-index.json`.

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
- Form Definitions with more than 32 conformance fixtures.

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
go run ./cmd/form-package conformance
```

## Release boundary

The repository-owned release tooling and protected workflows are documented in
[`../../release/form-packages.md`](../../release/form-packages.md). A package
source under `forms/releases/<release-id>/<packageVersion>/` is re-verified,
canonicalized, deterministically archived, described by SPDX 2.3 and SLSA v1
evidence, and signed through a keyless Sigstore v0.3 bundle whose identity is
bound to the exact repository and protected-main workflow. The reversible
release ID is `k-` plus lowercase unpadded base32 of the exact FormRef Kind;
the dispatcher separately verifies the exact `forms/<release-id>/v<semver>`
source tag and approved commit before signing.
The canonical index bytes—not archive metadata—remain the signed semantic
subject.

## Deliberate non-goals of the local verifier

The local verifier does not extract archives, fetch/install remote packages,
verify Sigstore publisher identity, consume revocation feeds, activate a Form,
publish a provider, or execute adapters. Those trust operations stay in the
release or host layer. A package is not publishable merely because this local
verifier accepts it, and a checked-in workflow is not proof that a live release
exists.
