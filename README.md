# Takoform Provider

`takoform` is a thin Terraform/OpenTofu provider for portable Service Forms. It gives HCL authors ten statically typed service resources and sends their desired state to any conforming Takoform host. The host—not this provider—selects and operates the concrete backend.

- GitHub: `github.com/tako0614/terraform-provider-takoform`
- Terraform Registry: `registry.terraform.io/tako0614/takoform`
- OpenTofu Registry: `registry.opentofu.org/tako0614/takoform`
- Service Form API: `forms.takoform.com/v1alpha1`

## Usage

```hcl
terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
    }
  }
}

provider "takoform" {
  endpoint = "https://forms.example.com"
  space    = "prod"
}

resource "takoform_object_bucket" "assets" {
  name       = "assets"
  interfaces = ["s3_api"]
}
```

`endpoint`, `space`, and the sensitive bearer `token` can instead be supplied as `TAKOFORM_ENDPOINT`, `TAKOFORM_SPACE`, and `TAKOFORM_TOKEN`.
The canonical provider identity is
`registry.terraform.io/tako0614/takoform`; OpenTofu dual-publishes the same
reviewed provider under the alternative identity
`registry.opentofu.org/tako0614/takoform`. The lifecycle matrix proves both
FQNs independently. They are distinct state identities, so changing between
them requires an explicit `state replace-provider` after updating
`required_providers`; matching bytes never makes them silent aliases. The
provider follows the same-origin versioned endpoint advertised by discovery.
The historical `/v1` facade requires the explicit `compatibility_fallback = true`
setting (or `TAKOFORM_COMPATIBILITY_FALLBACK=true`) and is never selected as an
implicit downgrade.

## Resources

| Resource | Portable intent |
| --- | --- |
| `takoform_edge_worker` | Prebuilt edge Worker artifact |
| `takoform_object_bucket` | Object storage |
| `takoform_kv_store` | Key/value storage |
| `takoform_queue` | Message queue |
| `takoform_sql_database` | Bounded indexed database (`2.0.0`), with historical SQL `1.x` state compatibility |
| `takoform_container_service` | OCI container service |
| `takoform_vector_index` | Vector index |
| `takoform_durable_workflow` | Durable workflow |
| `takoform_stateful_actor_namespace` | Stateful actor namespace |
| `takoform_schedule` | Cron-triggered invocation |

The provider deliberately has no target-pool, backend, credential, pricing, quota, billing, or operator-policy resources. It discovers `features.service_forms` and verifies the exact build-pinned candidate FormRef/package identity against the configured host. Backend placement, admission, and credentials remain host responsibilities; state contains only the canonical resource ID, generation fence, read-only drift status, portability, desired typed fields, and sanitized public outputs.

The `1.0.1` Form definitions also own their portable runtime declaration
contracts. Nine service Forms expose one required open `(name, version)`
descriptor (`http.request@1`, `object.storage@1`, `keyvalue.store@1`,
`sql.query@1`, `queue.messages@1`, `vector.query@1`, `workflow.invoke@1`, or
`actor.invoke@1`). `Schedule` intentionally exposes none because it consumes
`workflow.invoke` through its declared Resource connection. These names carry
no Takosumi Cloud identity, credentials, routing authority, or consumer grant;
the host materializes and authorizes the resulting Interface record.

`SQLDatabase@2.0.0` is a separate, unpublished structural successor candidate
for the same typed resource. Setting `tables` selects its bounded
`data.indexed@1` contract; the immutable `SQLDatabase@1.0.1` identity remains
the default for historical state, reads, deletes, and imports. The successor
pins closed request/response schemas, ascending cross-host ordering, and
tamper-evident live-keyset cursors. It requires the versioned Form host API and
fails before network I/O when historical `/v1` compatibility fallback is
configured. It adds no raw SQL, target, credential, capacity, billing, or host
implementation authority.

See [the portable specification status](spec/README.md), [Form Package contract](spec/form-package/README.md), [interface declaration contract](spec/interface-declaration/README.md), [form inventory](forms/README.md), [conformance status](conformance/README.md), [provider documentation](docs/index.md), and [examples](examples/resources/).

The repository also contains a data-only Form Package library and CLI. It
implements strict UTF-8 I-JSON validation, RFC 8785 canonicalization, exact
FormRef/package-index identity, and closed local-directory verification without
network access or code execution:

```console
go run ./cmd/form-package conformance
go run ./cmd/standard-form-conformance verify
go run ./cmd/standard-form-conformance published-package-check
go run ./cmd/form-package verify conformance/form-package-v1/positive/example-store
```

The protected Form Package release lane now builds deterministic package
evidence, keyless-signs the canonical index with Cosign v3, verifies its
Sigstore transparency bundle, attaches SPDX 2.3 and SLSA v1 evidence, and
publishes only an exact immutable GitHub Release inventory. A separate
append-only lane signs cumulative, hash-chained checkpoints for exact-digest
security revocations. See
[the Form Package release boundary](release/form-packages.md).

All ten `1.0.1` structural-candidate Form Packages now have immutable live
GitHub Releases. Their exact seven-asset inventories, Git refs, production
Sigstore trusted root, and package-index workflow policy are retained under
[`admission/v1/`](admission/v1/) and pass the offline
`published-package-check`. Signed host/provider/admission reports and the exact
direct Registry readback are retained as a five-role closure, but those source
bytes alone do not admit a Form. Only the matching immutable
`forms/admissions/v1.0.3` activation Release can activate them. The public
`release-check` verifies that immutable Release, its exact eight assets, the
completed controller promotion run, and the retained controller readback;
candidate-only `admission-closure-check` grants no activation. The Release binds the
existing definition/package `1.0.1` bytes; these independent version streams
avoid republishing or re-signing an unchanged package closure. The
active provider `0.1.3` source candidate pins that complete `1.0.1` Form set,
whose executable fixture
references resolve to a separate Takosumi-owned
`standard-form-runtime-v1.0.3` host-conformance runtime release.

The candidate's exact EdgeWorker, DurableWorkflow, and ContainerService bytes
are pinned in `forms/standard-runtime-artifact-set.json`. Run
`go run ./cmd/standard-form-conformance materializability-check` to read those
immutable release/OCI identities back before building any `1.0.1` package.
This verifies fixture bytes only and grants no host lifecycle or admission.
All candidate generation, verification, and closed ten-package assembly is
available through the local Go CLI. GitHub Actions is an optional automation
and current keyless OIDC signer, not a prerequisite for running the immutable
local gates.

## Development

Go 1.25.8 or later is required. Release builds are pinned to Go 1.26.5 or a
newer patched toolchain declared by the release descriptor.

```console
gofmt -w .
go vet ./...
go test ./...
go run ./cmd/conformance verify
go run ./cmd/migration-proof
go run ./cmd/provider-lifecycle-conformance matrix --opentofu tofu --terraform terraform
go run ./cmd/provider-lifecycle-conformance provider-reports \
  --cli terraform --output-dir /tmp/takoform-provider-reports \
  --source-commit "$(git rev-parse HEAD)"
go run ./cmd/form-package conformance
go run ./cmd/standard-form-conformance verify
go run ./cmd/standard-form-conformance candidate-publication-check
go run ./cmd/standard-form-conformance published-package-check
go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
```

`matrix` is the local `dev_overrides` regression gate. The separate
`render-registry-matrix` command performs a version-pinned direct Registry
install and exists for signed post-publication readback only. The Terraform
namespace and signing key are registered. Providers `v0.1.1` and `v0.1.2`
remain immutable GitHub Releases. Terraform Registry rejected `v0.1.1`
because its checksum manifest projected SPDX evidence as provider packages,
and rejected `v0.1.2` because it omitted the required Registry metadata
manifest checksum. The exact six-entry `v0.1.3` release is the
non-overwriting successor. Direct Terraform `1.15.8` and OpenTofu `1.12.1`
Registry installs now pass the full lifecycle matrix, and the canonical
unsigned readback is retained for the separately authenticated admission
candidate.

Provider publication and Standard Form admission are separate releases. The
provider `v*` workflow can publish only while the descriptor and inventory are
candidate-only; this never changes admission status. A later protected
`forms/admissions/v*` workflow runs `admission-closure-check` over real signed
package, runner, Registry, and admission evidence. After controller-authorized
promotion, public `release-check` requires the completed protected workflow and
exact immutable Release readback; only that combined authority activates admission.

Provider releases use the fail-closed signed `v*` tag workflow documented in
[release/README.md](release/README.md). The signing key is pinned by fingerprint;
the private key remains outside the repository. The `tako0614` public namespace
and pinned signing key are registered. Do not create a new release tag until
the release descriptor and provider compatibility gates are complete. Existing
version paths are immutable; the coordinated `1.0.1` Form pin therefore uses
provider `0.1.3`.

## License

MIT
