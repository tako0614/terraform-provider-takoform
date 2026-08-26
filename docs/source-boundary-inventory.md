# Takoform source and extraction boundary inventory

Status: implementation inventory for TASK-0033 W03
Baseline: `f71d4be4caf5a5e0c4fc97bfadeb6ebb627d1928` (2026-08-25)

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
| `cmd/current-form-source/`                                                                            | Imports all eight catalogs, injects cross-family refs and emits one aggregate                       | `takoform-forms` temporarily                                                     | Replace fixed imports with publisher-owned selection data, then remove this aggregate source path at W08                             |
| `scripts/current-form-families.mjs`                                                                   | Repeats family/Interface/Binding rosters and writes packages, candidate sets, index and Go registry | `takoform-forms` for package production                                          | Split Provider registry generation away; remove hard-coded rosters at W08                                                            |
| `internal/standardforms/current_families.go`                                                          | Repeats the eight-family roster and 31 Provider names for docs/examples                             | `takoform-forms` for family metadata; Provider names move to Provider projection | Delete the cross-authority duplicate at W08                                                                                          |
| `forms/candidates/<group>/`, `forms/candidates/current-family-index.json`                             | Generated current package/candidate selection surface                                               | `takoform-forms`                                                                 | Produced only from publisher source; admitted by neutral Core using exact digests                                                    |
| `interfaces/candidates/v1alpha1/`, `bindings/candidates/v1alpha2/`                                    | Generated current Interface and Binding candidate sets                                              | `takoform-forms`                                                                 | Produced with official family packages; interpreted by Core contracts without an official roster                                     |
| `internal/currentformregistry/registry_v3_generated.go`                                               | Generated exact current/default and retained identity registry                                      | no final owner                                                                   | Remove at W08; Provider projection and Core Snapshot replace its two mixed responsibilities                                          |
| `internal/currentformregistry/target_resolver.go`                                                     | Group-first exact target resolver                                                                   | `takoform` semantics                                                             | Reimplement behind Core Snapshot; remove generated/family-specific registry coupling                                                 |
| `internal/provider/v3_current_forms.go`, `v3_resource_types.go`, `provider.go`                        | 31 typed Provider resources and current family projection                                           | `terraform-provider-takoform`                                                    | Consume an exact Provider-owned projection over Core Snapshot; never feed names back into Core/publisher source                      |
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

### Authoring, registry and conformance consumers

- `cmd/standard-form-conformance/main.go` reaches the family graph indirectly
  through `internal/standardforms`.
- `internal/standardforms/publish_surfaces.go` and
  `publish_surfaces_v3.go` mix current-family metadata with Provider docs and
  examples.
- `internal/runtimeconformance/workerbundle/bundle.go` and
  `internal/workerauthoring/{harness,scenarios}.go` consume the current model,
  registry, Edge catalog, or generated corpus.
- `internal/currentformregistry/{registry_v3_test,target_resolver_test}.go`,
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

These remain Provider-owned. W07 replaces their current catalog/registry input
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
    -> cmd/current-form-source (fixed eight-family aggregate)
    -> scripts/current-form-families.mjs (second fixed roster)
    -> packages / candidate sets / exact generated registry
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

## Duplicate truths to eliminate

1. Family membership/order is repeated in each catalog, the aggregate Go
   command, `internal/standardforms/current_families.go`, JavaScript
   `familySpecs`, and `providerV3CurrentFamilies`.
2. Interface and Binding membership is repeated in family catalogs, the
   aggregate command, JavaScript constant tables and generated candidate sets.
3. Provider Terraform names are repeated in standard-form documentation data
   and `v3_resource_types.go`; they must have one Provider-owned projection.
4. Current exact identities are projected into both candidate artifacts and a
   generated Go registry; Snapshot must consume the artifacts directly.
5. Generic Host rules and Edge-specific family semantics coexist in the
   current conformance implementation and must be assigned to different
   corpora.

Generated files are evidence and distribution surfaces, not independent
authoring authorities. Retained history is intentionally duplicated as frozen
bytes and append-only ledgers; it is not regenerated or deduplicated into the
current family source.

## W03 exit result

Every current family source category, direct matching consumer, generated
public surface, Provider edge, conformance edge and retained-history surface at
baseline `f71d4be4` now has a final authority and migration treatment. The fixed
roster locations are classified for W08 deletion. W04-W07 may therefore prove
the replacement artifact path; this inventory does not itself open W08 or
authorize physical repository extraction.
