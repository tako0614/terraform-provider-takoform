# WorkerBundle — `takoform_worker_bundle`

## Workload and consumer

A build pipeline publishes the exact bytes of one worker build: a main
module plus additional ES modules, WASM modules, text, data, and source maps.
Worker Versions consume the bundle by name; hosts resolve its bytes through
the content-addressed artifact upload API (decision 0012).

## Role

`revision`. A bundle is an immutable snapshot: different bytes are a
different bundle resource, never an update.

## Observable semantics

`mainModule` names the module the runtime instantiates first and must name a
declared module. `modules` lists every module with its relative path, closed
media type, exact size, and canonical sha256 digest — the same facts the
committed artifact manifest carries, so the desired state and the uploaded
bytes cannot drift apart. Module linking follows the ES module graph rooted
at the main module.

## Why this is one Form

The bundle is the unit of build identity: digest-pinned bytes plus their
linking roles. Any consumer restoring or auditing a version needs exactly
this closure.

## What would require a separate Form

A different packaging model — an OCI image, a source tree compiled by the
host, or an archive whose bytes are the identity — changes the trust and
linking semantics and is a separate Form.

## Provided Interfaces

None.

## Accepted Bindings

None. Bindings belong to [WorkerVersion](worker-version.md), which binds
capabilities to executable snapshots, not to raw bytes.

## Lifecycle risks

Deleting a bundle still referenced by a Worker Version must fail with
`dependency_in_use`. A host must reject a bundle whose declared digests do
not match committed artifact blobs (`artifact_invalid` / `artifact_missing`).

## Prior art

The multi-module worker bundle (main module, additional modules, WASM,
source maps) of a proven edge platform, expressed as one verified
content-addressed unit per decision 0012.
