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
| `takoform_sql_database` | SQL database |
| `takoform_container_service` | OCI container service |
| `takoform_vector_index` | Vector index |
| `takoform_durable_workflow` | Durable workflow |
| `takoform_stateful_actor_namespace` | Stateful actor namespace |
| `takoform_schedule` | Cron-triggered invocation |

The provider deliberately has no target-pool, backend, credential, pricing, quota, billing, or operator-policy resources. It discovers `features.service_forms` and verifies the exact build-pinned candidate FormRef/package identity against the configured host. Backend placement, admission, and credentials remain host responsibilities; state contains only the canonical resource ID, generation fence, read-only drift status, portability, desired typed fields, and sanitized public outputs.

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

All ten `1.0.0` structural-candidate Form Packages now have immutable live
GitHub Releases. Their exact seven-asset inventories, Git refs, production
Sigstore trusted root, and package-index workflow policy are retained under
[`admission/v1/`](admission/v1/) and pass the offline
`published-package-check`. Publication still does not admit a Form: host and
authenticated provider lifecycle reports, direct Registry readback, signed
admission evidence, the distinct remaining role authorities, and a live
revocation-chain proof remain external. No `forms/admissions/v*` activation or
revocation release has been published. Unsigned provider-report subjects for a
provider candidate are generated only from that same candidate's exact
reviewed release-source fixtures; historical published-package authentication
remains a separate proof and cannot relabel a newer provider execution. Those
unsigned subjects grant no admission. The active provider `0.1.1` source
candidate pins a new complete `1.0.1` Form set whose executable fixture
references resolve to a separate Takosumi-owned
host-conformance runtime release. The retained `1.0.0` publication proof stays
independently verifiable while the new package releases and admission evidence
are prepared.

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
  --cli terraform --output-dir /tmp/takoform-provider-reports
go run ./cmd/form-package conformance
go run ./cmd/standard-form-conformance verify
go run ./cmd/standard-form-conformance candidate-publication-check
go run ./cmd/standard-form-conformance published-package-check
go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
```

`matrix` is the local `dev_overrides` regression gate. The separate
`render-registry-matrix` command performs a version-pinned direct Registry
install and exists for signed post-publication readback only. Provider `0.1.0`
has completed direct OpenTofu installation; Terraform Registry indexing still
depends on namespace Terms acceptance.

Provider publication and Standard Form admission are separate releases. The
provider `v*` workflow can publish only while the descriptor and inventory are
candidate-only; this never changes admission status. A later protected
`forms/admissions/v*` workflow runs `release-check` over real signed package,
runner, Registry, and admission evidence and is the only activation authority.

Provider releases use the fail-closed signed `v*` tag workflow documented in
[release/README.md](release/README.md). The signing key is pinned by fingerprint;
the private key remains outside the repository. The `tako0614` public namespace
and pinned signing key are registered. Do not create a new release tag until
the release descriptor and provider compatibility gates are complete. Existing
version paths are immutable; the coordinated `1.0.1` Form pin therefore uses
provider `0.1.1`.

## License

MIT
