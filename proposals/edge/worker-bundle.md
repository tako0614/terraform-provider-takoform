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

`manifestDigest` is the whole desired state: the immutable identity of the
artifact manifest committed for this build. That manifest names `mainModule`
and lists every module with its relative path, closed media type, exact size,
and canonical sha256 digest; the resource repeats none of it, so the bundle
has exactly one source of truth for its bytes and nothing can drift apart
from anything. Module linking follows the ES module graph rooted at the main
module.

Before it mutates anything, a host resolves the referenced manifest and holds
it to the artifact contract: an uncommitted digest is `artifact_missing`, and
a manifest of another kind, one whose `mainModule` is not declared, one whose
`mainModule` names a module the runtime never imports, one carrying asset
`files`, one naming a media type outside the closed set, or one overrunning
the host's published bundle limit is `artifact_invalid`. The closed set is the
media types `worker.runtime` LOADS plus the ones a bundle CARRIES and the graph
never imports — source maps — and `mainModule` may only be one of the first
([decision 0019](../../spec/decisions/0019-the-module-worker-abi-is-an-exact-contract.md)).

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
`dependency_in_use`. A host must reject a bundle whose `manifestDigest` names
no committed manifest, or names one the artifact contract refuses
(`artifact_missing` / `artifact_invalid`), and must keep a referenced manifest
and its blobs readable for as long as the bundle exists.

## Prior art

The multi-module worker bundle (main module, additional modules, WASM,
source maps) of a proven edge platform, expressed as one verified
content-addressed unit per decision 0012.
