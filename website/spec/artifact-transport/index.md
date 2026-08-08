# Content-addressed artifact transport (artifacts.takoform.com/v1alpha1)

The v1alpha3 lane replaces external credential-free artifact URLs with a
content-addressed upload API owned by the host
([decision 0012](../decisions/0012-artifacts-use-content-addressed-upload.md)).
Desired state references only immutable manifest digests; raw code bytes,
upload endpoints, and transport details never enter client state.

## Endpoints

Relative to the discovered `endpoints.api` base
(`/apis/forms.takoform.com/v1alpha3`):

```
POST   {api}/artifacts/uploads                      start an upload; body carries the manifest
PUT    {api}/artifacts/uploads/{uploadId}/blobs/{sha256}   upload one missing blob
POST   {api}/artifacts/uploads/{uploadId}/commit    verify and commit the manifest
GET    {api}/artifacts/{manifestDigest}             read a committed manifest
HEAD   {api}/artifacts/blobs/{sha256}               probe blob presence
DELETE {api}/artifacts/uploads/{uploadId}           abandon an incomplete upload
```

## Upload flow

1. The client computes the SHA-256 of every local file and builds a typed
   manifest ([`artifact-manifest-v1alpha1.schema.json`](/schemas/artifacts/v1alpha1/artifact-manifest.schema.json)).
2. `POST /artifacts/uploads` submits the manifest. The response is an
   `artifactUploadStatus` naming the `uploadId` and the digests of blobs the
   host does not already hold.
3. The client uploads only the missing blobs. Each `PUT` body is the exact
   blob; the host verifies its size and digest on receipt.
4. `POST .../commit` re-verifies the manifest and every blob against it
   (size, digest, media type, path grammar, per-kind shape) and returns the
   immutable `manifestDigest`: the RFC 8785 canonical digest of the manifest
   bytes.
5. Desired state (for example a `WorkerBundle` revision) references the
   manifest digest only.

Uploads are resumable: repeating step 2 with the same manifest returns the
still-missing blob set, and committing an already-committed manifest is
idempotent and returns the same digest. An abandoned `uploadId` may be
garbage-collected by the host; committed manifests and their blobs are
retained while any resource references them.

## Validation

A host MUST reject, before commit:

- duplicate module or file names;
- absolute paths, `..` or `.` segments, backslashes, NUL, invalid UTF-8;
- media types outside the manifest kind's closed set;
- size or digest mismatches between manifest and received bytes;
- file-count or total-size overruns of the host's published limits;
- a `WorkerBundle` whose `mainModule` is not listed in `modules`;
- a `WorkerBundle` whose `mainModule` names an auxiliary module;
- a source map whose target module is absent;
- archive bombs — archives are transport only and never semantic identity.

Rejections use `artifact_invalid` (400); a commit referencing a blob that was
never uploaded uses `artifact_missing` (404).

Every rule above is re-verified at commit, because commit is the step that
mints an immutable identity. A host MUST NOT rely on having checked the
manifest only when the upload started.

## Module media types: loadable and auxiliary

A `WorkerBundle` manifest admits exactly five module media types, and they
divide into two classes by what the module graph does with them:

| Class | Media type | The importing module receives |
| --- | --- | --- |
| loadable | `application/javascript+module` | an ES module |
| loadable | `text/plain` | the decoded UTF-8 text |
| loadable | `application/octet-stream` | an `ArrayBuffer` |
| loadable | `application/wasm` | a compiled `WebAssembly.Module`, never an instance |
| auxiliary | `application/source-map+json` | nothing: it is never imported |

**Loadable** is the set the module graph may import, and it is exactly the set
`worker.runtime@1.0.0`'s `loadModule` operation states
([decision 0019](../decisions/0019-the-module-worker-abi-is-an-exact-contract.md)).
**Auxiliary** is what a bundle may carry without ever linking it: today, source-map
evidence about another module. A host MUST refuse an auxiliary module as a
bundle's `mainModule` (`artifact_invalid`, 400), a runtime MUST fail an import
resolving to one (`unsupported_media_type`), and neither may treat the module's
mere presence in `modules` as an error — carrying it is the whole point.

`application/json` is not supported in this ABI version. The published manifest
enum never admitted it, so a runtime that loaded one would promise a module no
conforming bundle can carry.

The published manifest schema states the union and cannot state the split:
its `modules[].mediaType` enum lists all five, and nothing in JSON Schema
relates `mainModule` to the media type of the module it names. The split is
therefore enforced by the host, by the authoring client, and by the required
conformance check `bundle-main-module-is-loadable`
([decision 0014](../decisions/0014-published-schemas-are-structural-minima.md)).
The set has one source of truth, `internal/currentformmodel`, which the runtime
contract, the host validator, and the provider's authoring allowlist all read;
a drift gate holds it to the published enum.

## Per-kind exclusivity

A manifest kind decides which payload members the document may carry, and it
is normative:

- a `WorkerBundle` manifest carries `mainModule` and `modules` and MUST NOT
  carry `files`;
- a `StaticAssetBundle` or `MigrationBundle` manifest carries `files` and MUST
  NOT carry `mainModule` or `modules`.

A manifest carrying both shapes has two meanings, which a content-addressed
identity must never have; violations are `artifact_invalid` (400). The
published manifest schema is the structural minimum for this document and
declares all three members for every kind, so this closure is enforced by the
host and proved by a required conformance check rather than by the schema
([decision 0014](../decisions/0014-published-schemas-are-structural-minima.md)).

## Referencing a manifest from desired state

A bundle-shaped revision resource carries the manifest digest as its whole
desired state; the manifest, not the resource, describes the bytes. Before any
mutation — apply and import alike — a host MUST resolve the referenced
manifest and fail closed when the digest names no committed manifest the
caller's tenant holds (`artifact_missing`, 404), when the stored document does
not canonicalize to the referenced digest, when its `kind` is not the kind the
Form requires, or when it violates any rule above (`artifact_invalid`, 400).

Resolving a manifest on behalf of a request is subject to the same per-tenant
holding rule as reading one
([decision 0018](../decisions/0018-the-host-api-is-deployable-behind-ordinary-infrastructure.md)):
a digest is a name for bytes and entitles nobody to them, whether it is read
from `GET {api}/artifacts/{manifestDigest}` or used as desired state. A manifest
another tenant committed is therefore answered exactly as an uncommitted digest
is — the same `artifact_missing`, indistinguishable from "no such manifest" — and
that answer is re-derived when an accepted `202` commits, not only when the
mutation was accepted. Holding is the tenant's, not one principal's, so the
ordinary pipeline in which one principal uploads a bundle and another references
it is unaffected.

A committed manifest and its blobs MUST stay readable while any resource
references the manifest: abandoning an unrelated upload session, or
collecting staged blobs, MUST NOT make a referenced artifact unresolvable.

## Boundary

- The manifest is data-only: no credentials, endpoints, or host identities.
- A manifest digest is an immutable identity; the same digest MUST resolve to
  the same canonical manifest bytes on every conforming host that holds it.
- Blob storage, deduplication, and retention policy are host-owned.
- The frozen v1alpha1/v1alpha2 lanes keep their credential-free URL contract
  ([decision 0002](../decisions/0002-artifact-urls-are-credential-free-state.md))
  unchanged.
