# Takoform source and extraction boundary inventory

Status: W08 duplicate-authority deletion completed for TASK-0033
Baseline: `2fbc557265bc52ac2e046b9df0be2bfa3565c3d6` (2026-08-26)

This document classifies the current source graph before repository extraction.
It is not a public contract or a publication record. The target topology is:

| Repository                                        | Sole responsibility after extraction                                                                |
| ------------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| `github.com/tako0614/takoform`                    | Specification, neutral Core, package schemas/verifier, generic conformance, SDK and CLI             |
| `github.com/tako0614/takoform-forms`              | Official family declarations, family corpora, package production and official publisher evidence    |
| `github.com/tako0614/terraform-provider-takoform` | Terraform/OpenTofu projection, schemas, imports, codecs, diagnostics and immutable Provider history |

Physical extraction begins only after the artifact boundary and downstream
parity gates pass. Moving the current fixed roster between repositories is not
the target architecture.

## Counted baseline

- Eight current official family groups contain 31 Forms: Edge 16, Function 4,
  Container 5, Table 1, Queue 1, Topic 2, Schedule 1 and Vector 1.
- The seven non-Edge groups contain 15 Forms.
- The aggregate declares 13 Interfaces and 6 Bindings.
- Current generated family packages contain 306 tracked files; adding
  `forms/candidates/current-family-index.json` makes 307.
- `conformance/takoform-v1/` contains 70 tracked generated files.
- Retained `forms/candidates/edge/v1beta1/` contains 117 tracked files for 15
  historical Forms.

Candidate metadata is explicitly unpublished. These counts characterize the
source graph; they are not evidence of Form Package publication.

## Source and output ownership

| Current source or output                                                                              | Observed role                                                                                       | Future owner                                                                     | Future producer / consumer edge                                                                                                      |
| ----------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ |
| `formpackage/`                                                                                        | Exact FormRef, package schemas, canonicalization, closure verification                              | `takoform`                                                                       | Core package verifier; consumed equally for official and external packages                                                           |
| `internal/currentformsnapshot/`                                                                       | Neutral compiler and immutable Snapshot over verified packages and exact Interface/Binding artifacts | `takoform`                                                                       | Core after extraction; production code consumes no family, Provider or Host source                                                   |
| `internal/currentformmodel/`                                                                          | Rich Go authoring DSL used to emit official definitions and fixtures                                | `takoform-forms`                                                                 | Publisher implementation detail; portable formats stay in Core, but this directory is not a Core extension API                       |
| `internal/edgeformcatalog/`                                                                           | 16 current Edge Forms, 7 Interfaces, 6 Bindings and family fixtures                                 | `takoform-forms`                                                                 | Official publisher source; emits data-only artifacts through the common verifier                                                     |
| `internal/functionformcatalog/`                                                                       | 4 Function Forms and `function.runtime@1.0.0`                                                       | `takoform-forms`                                                                 | Official publisher source                                                                                                            |
| `internal/containerformcatalog/`                                                                      | 5 Container Forms and `container.runtime@1.0.0`                                                     | `takoform-forms`                                                                 | Official publisher source                                                                                                            |
| `internal/tableformcatalog/`                                                                          | 1 Table Form and `table.document@1.0.0`                                                             | `takoform-forms`                                                                 | Official publisher source                                                                                                            |
| `internal/queueformcatalog/`                                                                          | 1 PullQueue Form and `queue.pull@1.0.0`                                                             | `takoform-forms`                                                                 | Official publisher source; dependency root for Topic and Schedule                                                                    |
| `internal/topicformcatalog/`                                                                          | 2 Topic Forms and `topic.publish@1.0.0`                                                             | `takoform-forms`                                                                 | Official publisher source; consumes the exact Queue Interface ref                                                                    |
| `internal/scheduleformcatalog/`                                                                       | 1 Schedule Form                                                                                     | `takoform-forms`                                                                 | Official publisher source; consumes exact Queue and Topic Interface refs                                                             |
| `internal/vectorformcatalog/`                                                                         | 1 VectorIndex Form and its Interface                                                                | `takoform-forms`                                                                 | Official publisher source                                                                                                            |
| `cmd/current-form-source/`                                                                            | Imports all eight catalogs, injects cross-family refs and emits the sole publisher composition       | `takoform-forms` temporarily                                                     | Retain as the sole publisher-owned composition until W13; W08 removes competing consumer/Provider paths rather than this producer     |
| `scripts/current-form-families.mjs`                                                                   | Writes packages, candidate sets and the selected index from the Go publisher source document         | `takoform-forms` for package production                                          | Contains no independent family/contract roster and no Provider-registry producer; retain the reproducibility check until W13         |
| `internal/standardforms/current_families.go`                                                          | Groups exact Provider projection entries for generated docs/examples                                | `terraform-provider-takoform`                                                    | Contains neither a family roster nor a Terraform-name map; documentation consumes the same projection as Provider registration       |
| `forms/candidates/<group>/`, `forms/candidates/current-family-index.json`                             | Generated current package/candidate selection surface                                               | `takoform-forms`                                                                 | Produced only from publisher source; admitted by neutral Core using exact digests                                                    |
| `interfaces/candidates/v1alpha1/`, `bindings/candidates/v1alpha2/`                                    | Generated current Interface and Binding candidate sets                                              | `takoform-forms`                                                                 | Produced with official family packages; interpreted by Core contracts without an official roster                                     |
| deleted `internal/currentformregistry/`                                                               | Former mixed current/default/retained registry and publisher resolver                               | no owner                                                                         | Removed at W08; Provider projection plus Core Snapshot own consumption, and the authoring resolver lives in `currentformmodel`        |
| `internal/provider/v3_snapshot_assembly.go`, `v3_resource_types.go`, `provider.go`                   | 31 typed Provider resources and current family projection                                           | `terraform-provider-takoform`                                                    | Consume an exact Provider-owned projection over Core Snapshot; never feed names back into Core/publisher source                      |
| `internal/provider/v3_lifecycle.go`, `v3_codec.go`                                                    | Provider lifecycle request/response and exact state codec dispatch                                  | `terraform-provider-takoform`                                                    | Preserve Provider 3 behavior and consume exact Snapshot identities                                                                   |
| `scripts/takoform-v1-derive.mjs`, `cmd/portable-host-conformance/`, `internal/portableconformancev3/` | Mixed generic, family and composition conformance derivation/execution                              | split by corpus authority                                                        | Generic Core/Host corpus and runner go to `takoform`; family fixtures go to `takoform-forms`; selected composition is explicit input |
| `conformance/takoform-v1/`                                                                            | Generated Host API v1 corpus selected from all eight current families                               | split generated output                                                           | Generic output is produced by Core conformance; each official family corpus is produced by its publisher                             |
| `cmd/reference-host/`, `internal/clientv3/`                                                           | Reference Host and client adapter                                                                   | `takoform`                                                                       | Consume neutral Core/Snapshot; do not author family semantics                                                                        |
| `internal/retainededgeformcatalog/`, `internal/provider/testdata/v3-retained-*`                       | Frozen Provider 2.1.1 identities and codecs                                                         | `terraform-provider-takoform`                                                    | Immutable readable state history only; never regenerated from current official catalogs                                              |
| `forms/candidates/edge/v1beta1/`, `release/provider-form-identities.json`                             | Frozen package bytes and append-only Provider identity record                                       | `terraform-provider-takoform` history custody                                    | Retained exact bytes/provenance; ObjectBucket remains recorded without a Provider 3 mapping or codec                                 |
| `docs/resources/`                                                                                     | Provider resource reference pages                                                                   | `terraform-provider-takoform`                                                    | Generated/maintained from Provider-owned schemas plus exact Form metadata; not normative Core semantics                              |

## Direct consumer audit

The table above assigns authorities; this section closes the mechanical
consumer inventory at the baseline instead of implying that its representative
rows are an exhaustive file list. The audit searched tracked source for family
catalog/model/registry imports, current-family indexes, family generators,
Provider family tables, and the `takoform-v1` corpus. It found 131
non-generated direct matching paths. Sixty-five were outside the representative
rows above; the indirect standard-form command and the VitePress status module
make 67 additional source/consumer paths that extraction must migrate or
delete.

### Baseline authoring, registry and conformance consumers

- `cmd/standard-form-conformance/main.go` reaches the family graph indirectly
  through `internal/standardforms`.
- At the baseline, `internal/standardforms/publish_surfaces.go` and
  `publish_surfaces_v3.go` mixed current-family metadata with Provider docs and
  examples; W08 moved the current projection authority behind Provider.
- `internal/runtimeconformance/workerbundle/bundle.go` and
  `internal/workerauthoring/{harness,scenarios}.go` consume the current model,
  registry, Edge catalog, or generated corpus.
- The now-deleted
  `internal/currentformregistry/{registry_v3_test,target_resolver_test}.go`,
  `internal/projectpolicy/provider_migration_test.go`,
  `spec/artifact_media_types_test.go`, and
  `modules/worker-app/variables.tf` encode current-source assumptions that
  must become artifact/projection assertions.

### Provider consumers

The Provider boundary is broader than the five representative production
files in the table. The extraction/parity gate covers these 26 additional
tracked consumers:

```text
internal/provider/provider_test.go
internal/provider/v3_artifact.go
internal/provider/v3_claim_test.go
internal/provider/v3_continuity_test.go
internal/provider/v3_current_family_test.go
internal/provider/v3_defaults_test.go
internal/provider/v3_diagnostics.go
internal/provider/v3_diagnostics_test.go
internal/provider/v3_edge_app_test.go
internal/provider/v3_environment_namespace.go
internal/provider/v3_fake_host_test.go
internal/provider/v3_file_artifact.go
internal/provider/v3_host_support.go
internal/provider/v3_import_identity.go
internal/provider/v3_lifecycle_test.go
internal/provider/v3_outputs_test.go
internal/provider/v3_recovery_test.go
internal/provider/v3_relations_test.go
internal/provider/v3_release_projection.go
internal/provider/v3_resource_types_test.go
internal/provider/v3_resources.go
internal/provider/v3_retained_codec_test.go
internal/provider/v3_revision_names.go
internal/provider/v3_schema_evolution_test.go
internal/provider/v3_state.go
internal/provider/v3_w0_types_test.go
```

These remain Provider-owned. W08 replaced their current catalog/registry input
with a Provider-owned projection over Snapshot; it does not move Terraform
semantics into Core.

### Release, generation and public-surface consumers

- `.github/workflows/quality.yml`, `package.json`,
  `cmd/provider-release/main.go`, and
  `release/published-document-lanes.json` orchestrate generation or publication
  checks.
- `scripts/{check-public-surfaces,current-form-families.test,publication-blockers,publication-evidence,publication-evidence.test,release-deploy,sync-website-spec}.mjs`
  read current indexes, corpora, implementation roots, or legacy allowlists.
- `website/.vitepress/site-status.mjs` exposes the candidate-set and family-index
  locations to the site build.
- README/spec/proposal references, `spec/publication-blockers.json`, and
  `spec/publication-evidence.json` are documentation or evidence consumers;
  they migrate with their owning authority instead of being used as code
  authority.

Generated mirrors are counted separately and regenerated after their source
owner moves: 194 matching files under `website/public/`, 15 under
`website/static/`, and 14 mirrored website Markdown files. They are outputs,
not migration sources. `CONTRIBUTING.md`, the historical comment in
`formpackage/schema.go`, and the legacy strip rule in
`scripts/sync-website-spec.mjs` are classified as comment/legacy references and
must be deleted or rewritten, not converted into runtime dependencies.

## Current dependency graph

```text
formpackage
    -> neutral currentformsnapshot tracer

currentformmodel
    -> per-family catalogs
    -> cmd/current-form-source (sole publisher composition)
    -> scripts/current-form-families.mjs (source-document projection)
    -> packages / candidate sets / selected index
       -> currentformselection -> immutable Snapshot
          -> Provider projection + codecs
          -> conformance derivation + reference Host

retainededgeformcatalog + frozen bytes
    -> Provider retained codec/state history only
```

The extraction target reverses no authority:

```text
takoform Core + verifier
    <- admitted data-only packages from any publisher
    -> immutable Snapshot
       -> Provider-owned exact projection
       -> Host/conformance adapters

takoform-forms publisher
    -> official data-only packages and family corpora
    -> the same Core/verifier path as an external publisher
```

## W08 duplicate-authority result

1. `cmd/current-form-source` is the one remaining publisher composition path;
   its source document carries all family, Form, Interface and Binding metadata.
2. `scripts/current-form-families.mjs` derives every selected artifact from
   that document and no longer emits a Provider registry.
3. `internal/currentformselection` verifies the digest-pinned selected graph
   and compiles one neutral immutable Snapshot without catalog imports.
4. Provider registration, state dispatch, codecs and Terraform naming consume
   one embedded Provider projection. Generated docs consume that projection as
   well and keep no roster or name map.
5. reference Host and worker conformance acquire exact current identities from
   `currentformselection`; the mixed `internal/currentformregistry` package is
   deleted.
6. the publisher-only group-first target resolver lives in
   `internal/currentformmodel`; it is not a neutral Core public API.

Generated files are evidence and distribution surfaces, not independent
authoring authorities. Retained history is intentionally duplicated as frozen
bytes and append-only ledgers; it is not regenerated or deduplicated into the
current family source.

## W08 exit result

The competing consumer and Provider rosters, generated registry, aggregate
consumer lookup and dual writers are removed. Official catalog and rendering
source remains behind one publisher composition until W13; physical repository
extraction remains W16 work rather than an implicit consequence of W08.
