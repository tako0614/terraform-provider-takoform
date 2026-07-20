# Standard-definition candidate set and legacy inventory

The provider build pins this all-or-nothing set of exact candidate bytes:

| Kind | Provider resource | Standard identity | Required portable Interface |
| --- | --- | --- | --- |
| `EdgeWorker` | `takoform_edge_worker` | `1.0.1 / standard` | `http.request@1` |
| `ObjectBucket` | `takoform_object_bucket` | `1.0.1 / standard` | `object.storage@1` |
| `KVStore` | `takoform_kv_store` | `1.0.1 / standard` | `keyvalue.store@1` |
| `SQLDatabase` | `takoform_sql_database` | `1.0.1 / standard` | `sql.query@1` |
| `Queue` | `takoform_queue` | `1.0.1 / standard` | `queue.messages@1` |
| `VectorIndex` | `takoform_vector_index` | `1.0.1 / standard` | `vector.query@1` |
| `DurableWorkflow` | `takoform_durable_workflow` | `1.0.1 / standard` | `workflow.invoke@1` |
| `ContainerService` | `takoform_container_service` | `1.0.1 / standard` | `http.request@1` |
| `StatefulActorNamespace` | `takoform_stateful_actor_namespace` | `1.0.1 / standard` | `actor.invoke@1` |
| `Schedule` | `takoform_schedule` | `1.0.1 / standard` | none (consumer only) |

[`standard-package-set.json`](standard-package-set.json) pins every exact
`(FormRef, packageDigest)` pair. Each independent package lives under
[`../conformance/form-package-v1/positive/standard/`](../conformance/form-package-v1/positive/standard/)
and contains canonical desired, observed, output, and negative fixtures. The
observed fixture carries only lifecycle/import/drift status. The output fixture
carries only exact kind, name, generation, identity, and portability evidence,
plus SQLDatabase's portable `engine` value required by `sql.query@1`.
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

Host runner reports embed a non-secret execution summary and its RFC 8785
SHA-256 plus, for every positive and negative case, both the exact package
fixture-file digest and the effective canonical input digest. The admission
validator recomputes the summary digest, checks its lifecycle/fixture parity,
re-reads the package, and requires the retained admission desired, observed,
output, and negative documents to equal its fixture closure. Public reports
therefore prove what bytes ran without echoing artifact URLs, local paths,
desired values, or connection documents. Provider reports do not claim this
host-only execution binding.

The published `1.0.0` packages remain immutable structural candidates, but
they are deliberately non-admitted: their EdgeWorker, DurableWorkflow, and
ContainerService desired fixtures contain illustrative artifact locations or
digests and are not real executable artifacts. A host must not substitute
those values and report the run as the canonical fixture.

The active coordinated `1.0.1` candidate instead pins the Takosumi-owned,
host-conformance-only `standard-form-runtime-v1.0.3` EdgeWorker and
DurableWorkflow release identities and their real byte digests plus a public
Docker Hub linux/amd64 OCI manifest by exact digest. Optional
VectorIndex/Workflow/Container/Actor connection, delivery, migration-path, and unsupported
strong-consistency preferences are absent from the canonical desired fixtures;
their portable schema capabilities remain available. EdgeWorker retains one
reviewed `ObjectBucket/edge-assets` connection projected as
`object.binding.v1`; Schedule retains its one required
`DurableWorkflow/ingest` connection. Provider `0.1.1` pins this exact
all-or-nothing set; provider `0.1.0` and every `1.0.0` package remain immutable
historical candidates. Runtime publication, Form Package publication, and
external lifecycle evidence are still required before admission.

[`standard-runtime-artifact-set.json`](standard-runtime-artifact-set.json)
retains the exact public runtime release and OCI readback identities used by
the candidate fixtures. `go run ./cmd/standard-form-conformance
materializability-check` re-downloads those immutable bytes and rejects a
missing, substituted, or digest-mismatched artifact. This is fixture
materializability evidence only; it grants no Resource, Target, manager,
capacity, billing, or admission authority.

The descriptors are Form-owned data, not host-specific runtime code. Each
descriptor has an exact closed document schema and only portable `output`
mappings. SQLDatabase maps `/id`, `/name`, and `/engine` into `sql.query@1`;
no `takosumi.cloud.*` type is part of the package. A host advertising portable
Interface declarations must materialize every required descriptor before the
Resource can be Ready. InterfaceBinding, consumer authorization, endpoint
routing, and record lifecycle remain host-owned.

Provider publication and Form admission activation are two different
authorities. Phase 1's `v*` provider workflow runs
`candidate-publication-check`: it may publish the exact provider build to the
Public Registry while this inventory remains `external-required` and while no
Form becomes admitted or activated. Phase 2's protected
`forms/admissions/v*` workflow runs `release-check` only after that same
provider version can be installed directly from both canonical Registry FQNs.
That activation gate opens only after every external requirement is
authenticated. It verifies
retained RFC 8785 admission-evidence bytes against an offline Sigstore v0.3
bundle, a digest-pinned trusted root, and role-specific digest-pinned exact
Fulcio publisher policies. It requires a Rekor inclusion proof, a signed
integrated time, and a verified certificate-transparency SCT without
contacting GitHub or another distribution endpoint. The same fail-closed chain
now validates canonical signed host/provider runner reports, the exact five
asset Form Package release manifest/readback for every candidate, and a signed
provider readback backed by the complete direct-Registry OpenTofu/Terraform
lifecycle matrix. These are implemented validators, not generated evidence.
The ten live immutable package releases, their exact release assets, the
production Sigstore trusted-root snapshot, and the digest-pinned package-index
publisher policy are retained under `admission/v1` and pass
`published-package-check`. The final five-role production admission pins,
host/provider/admission reports, direct Registry readback, and
`standard-admission-set.json` do not exist yet, so `release-check` still fails
closed before admission can open.

This ordering is intentional, not a publication bypass: the immutable public
provider is a typed client for structural candidates, while only the separate
signed admission release can classify the exact packages
`portable-standard`. A failed or absent Phase 2 leaves every Form unavailable
for standard activation even though the provider binary is installable.

The local Takosumi host proof and reviewed OpenTofu/Terraform FQN lifecycle
matrix cover the candidate set; shared negative admission fixtures use the
portable API wire code `invalid_argument`. The package verifier's internal
failure name `schema_validation_failed` is not a wire error. An admission
artifact may be accepted only after external runners authenticate that evidence
and bind it to immutable tags, Registry install/readback, Sigstore provenance,
and signed admission evidence.

After an authorized provider publication, generate the direct Registry matrix
without `dev_overrides`:

```bash
go run ./cmd/provider-lifecycle-conformance render-registry-matrix \
  --opentofu tofu --terraform terraform \
  > admission/v1/registry/provider-lifecycle-matrix.json
go run ./cmd/admission-readback registry \
  --matrix admission/v1/registry/provider-lifecycle-matrix.json \
  --provider-release-commit "$(git rev-list -n 1 "$(jq -r .tag release/version.json)")" \
  > admission/v1/registry/provider-readback.json
```

The command pins the provider version from `release/version.json`, runs
`init` against each canonical FQN, hashes the installed provider binary and
schema, and performs the same ten-resource lifecycle. A local matrix produced
by `render-matrix` carries `installationSource: local-dev-override` and is
rejected by Registry readback validation. `admission-readback` strictly
validates the direct matrix and emits RFC 8785 canonical readback bytes; it
does not sign them or change admission state.

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
`VectorIndex@1.0.1` candidate additionally makes `/dimensions` immutable, and
`SQLDatabase@1.0.1` makes `/engine` immutable; the provider schemas enforce
replacement for both fields.

Target pools, credentials, provider selection, backend managers, capacity,
pricing, billing, quota, and execution authority are outside every portable
Form Package and externally supplied admission evidence document.
