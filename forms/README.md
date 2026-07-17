# Standard-definition candidate set and legacy inventory

The provider build pins this all-or-nothing set of exact candidate bytes:

| Kind | Provider resource | Standard identity |
| --- | --- | --- |
| `EdgeWorker` | `takoform_edge_worker` | `1.0.0 / standard` |
| `ObjectBucket` | `takoform_object_bucket` | `1.0.0 / standard` |
| `KVStore` | `takoform_kv_store` | `1.0.0 / standard` |
| `SQLDatabase` | `takoform_sql_database` | `1.0.0 / standard` |
| `Queue` | `takoform_queue` | `1.0.0 / standard` |
| `VectorIndex` | `takoform_vector_index` | `1.0.0 / standard` |
| `DurableWorkflow` | `takoform_durable_workflow` | `1.0.0 / standard` |
| `ContainerService` | `takoform_container_service` | `1.0.0 / standard` |
| `StatefulActorNamespace` | `takoform_stateful_actor_namespace` | `1.0.0 / standard` |
| `Schedule` | `takoform_schedule` | `1.0.0 / standard` |

[`standard-package-set.json`](standard-package-set.json) pins every exact
`(FormRef, packageDigest)` pair. Each independent package lives under
[`../conformance/form-package-v1/positive/standard/`](../conformance/form-package-v1/positive/standard/)
and contains canonical desired, observed, output, and negative fixtures. The
observed fixture carries only lifecycle/import/drift status. The output fixture
carries only exact kind, name, generation, identity, and portability evidence.
Desired configuration, runner-local paths, and connection topology are echoed
into neither contract.
This repository does not emit passed Standard Form admission evidence from
those files.

Run both local structural gates after changing a package or provider schema:

```bash
go run ./cmd/standard-form-conformance generate
go run ./cmd/standard-form-conformance verify
```

`generate` creates independent data-only definitions and fixtures; `verify`
runs the package verifier and inspects actual provider constructors, attribute
coverage, import support, and selected `RequiresReplace` modifiers. These
checks do not execute a Terraform protocol lifecycle or a Takosumi host. The
inventory therefore says `classification: structural-candidate`,
`localConformance: structural-only`, `admissionStatus: external-required`, and
`publicationReady: false`. Observed and output fixtures expose only lifecycle
status/identity; neither echoes the desired document, connection topology, or
runner-local artifact locations.

The provider tag workflow runs
`go run ./cmd/standard-form-conformance release-check`. That gate intentionally
fails closed while these are the only available claims, so candidate FormRefs
cannot silently become a public provider release identity. It can be opened
only by a separately reviewed implementation that authenticates every external
requirement and the exact signed admission evidence.

The local Takosumi host proof and reviewed OpenTofu/Terraform FQN lifecycle
matrix cover the candidate set; shared negative admission fixtures use the
portable API wire code `invalid_argument`. The package verifier's internal
failure name `schema_validation_failed` is not a wire error. An admission
artifact may be accepted only after external runners authenticate that evidence
and bind it to immutable tags, Registry install/readback, Sigstore provenance,
and signed admission evidence.

Each definition keeps `status: standard` so the exact proposed final bytes can
be exercised and digest-pinned without a later status mutation. That field does
not admit the package set. Only externally authenticated Takosumi host and
Terraform provider lifecycle reports can produce admission evidence classified
`portable-standard`; until then this inventory is neither admitted nor
conformant.

## Legacy compatibility identities

The historical packages under
[`../conformance/form-package-v1/positive/legacy/`](../conformance/form-package-v1/positive/legacy/)
remain immutable `0.0.0-legacy.1 / compatibility-candidate` identities. They
were not edited or promoted into this definition candidate set. Their exact digests remain in
[`legacy-package-set.json`](legacy-package-set.json), and the historical wire
conversion remains in
[`legacy-takosumi-wire-mapping.md`](legacy-takosumi-wire-mapping.md).

Only `/name` is asserted immutable in the legacy definitions. The independent
`VectorIndex@1.0.0` candidate additionally makes `/dimensions` immutable, and
`SQLDatabase@1.0.0` makes `/engine` immutable; the provider schemas enforce
replacement for both fields.

Target pools, credentials, provider selection, backend managers, capacity,
pricing, billing, quota, and execution authority are outside every portable
Form Package and externally supplied admission evidence document.
