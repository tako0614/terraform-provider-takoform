# 0012 — Artifacts use content-addressed upload

- Status: accepted
- Date: 2026-08-06
- Owners: Takoform maintainers

## Context

Decision [0002](0002-artifact-urls-are-credential-free-state.md) made artifact
sources credential-free HTTPS URLs fetched by the host. That keeps secrets out
of state, but it forces every deployment to publish a world-readable artifact
endpoint, makes the host's fetch path a hidden availability dependency, leaves
media-type and layout validation implicit, and cannot express a multi-module
worker bundle (main module, additional modules, WASM, source maps) as one
verified unit.

## Decision

The v1alpha3 lane replaces external artifact URLs with a content-addressed
upload API on the host:

```
POST   /artifacts/uploads
PUT    /artifacts/uploads/{uploadId}/blobs/{sha256}
POST   /artifacts/uploads/{uploadId}/commit
GET    /artifacts/{manifestDigest}
HEAD   /artifacts/blobs/{sha256}
DELETE /artifacts/uploads/{uploadId}
```

The client computes blob digests locally, asks the host which blobs are
missing, uploads only those, and commits a canonical manifest. The host
verifies size, digest, and media type per blob, then returns an immutable
manifest digest. Desired state references only that manifest digest; raw code
bytes and upload endpoints never enter Terraform state.

An artifact manifest is a typed, canonical document (for example
`artifacts.takoform.com/v1alpha1` `WorkerBundle` with `mainModule` and a
`modules[]` list of name, media type, size, digest). Media types are closed
per manifest kind. Archives are transport only; a tar or zip byte stream is
never a semantic identity.

"Closed per manifest kind" is closed **against the runtime**, not against a
list this document keeps. A `WorkerBundle` manifest admits exactly the media
types `worker.runtime` can LOAD, plus the ones a bundle may CARRY and the graph
never imports — today source maps alone. A media type in neither is refused,
and an auxiliary media type is refused as `mainModule` while being perfectly
admissible in `modules`. Stating the set anywhere but once, beside the runtime
contract that consumes it, is how the manifest and the ABI came to disagree in
the first place; the reconciled set and its two classes live in
[decision 0019](0019-the-module-worker-abi-is-an-exact-contract.md) and are
enforced in code and conformance under
[decision 0014](0014-published-schemas-are-structural-minima.md), because the
published enum states the union and cannot state the split.

Hosts reject: duplicate module names, absolute paths, `..`, backslashes, NUL,
invalid UTF-8 names, unsupported media types, size or digest mismatches, file
count and total-size overruns, a missing main module, a main module the runtime
never imports, source maps whose target module is absent, and archive bombs.

Uploads are resumable: re-asking for missing blobs and re-committing the same
manifest is idempotent and converges on the same manifest digest.

## Consequences

- v1alpha3 Form Definitions drop `artifactUrl`, `artifactSha256`,
  `artifactMediaType`, and artifact-source attributes; revision Forms carry
  artifact references (`bundleRef`, module lists) resolved through the upload
  API.
- The provider computes digests from local files (`content_file` authoring
  attributes), performs the upload, and persists only manifest digests.
- Decision 0002 remains the retained rule for the frozen v1alpha1/v1alpha2
  lanes; published bytes and their verification are unchanged.
- Conformance gains digest-mismatch, missing-blob, resume, and manifest
  validation checks.

## Rejected alternatives

- **Keep credential-free URLs alongside uploads.** Rejected because two
  ingestion paths double the security surface and keep the availability
  dependency this decision removes.
- **Pre-signed host upload URLs outside the API.** Rejected because the
  contract must stay portable across hosts; pre-signed transport is a host
  implementation detail behind the same API.
- **OCI registry as the required transport.** Rejected because it imports a
  full registry protocol for what one content-addressed manifest API covers;
  an OCI adapter can still implement this contract.
