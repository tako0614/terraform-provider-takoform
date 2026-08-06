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
   manifest ([`artifact-manifest-v1alpha1.schema.json`](../schemas/artifact-manifest-v1alpha1.schema.json)).
2. `POST /artifacts/uploads` submits the manifest. The response is an
   `artifactUploadStatus` naming the `uploadId` and the digests of blobs the
   host does not already hold.
3. The client uploads only the missing blobs. Each `PUT` body is the exact
   blob; the host verifies its size and digest on receipt.
4. `POST .../commit` re-verifies every blob against the manifest (size,
   digest, media type, path grammar) and returns the immutable
   `manifestDigest`: the RFC 8785 canonical digest of the manifest bytes.
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
- a source map whose target module is absent;
- archive bombs — archives are transport only and never semantic identity.

Rejections use `artifact_invalid` (400); a commit referencing a blob that was
never uploaded uses `artifact_missing` (404).

## Boundary

- The manifest is data-only: no credentials, endpoints, or host identities.
- A manifest digest is an immutable identity; the same digest MUST resolve to
  the same canonical manifest bytes on every conforming host that holds it.
- Blob storage, deduplication, and retention policy are host-owned.
- The frozen v1alpha1/v1alpha2 lanes keep their credential-free URL contract
  ([decision 0002](../decisions/0002-artifact-urls-are-credential-free-state.md))
  unchanged.
