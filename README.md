# Takoform Provider

`takoform` is a thin Terraform/OpenTofu provider for portable Service Forms. It gives HCL authors 34 statically typed service resources and sends their desired state to any conforming Takoform host. The host—not this provider—selects and operates the concrete backend.

- GitHub: `github.com/tako0614/terraform-provider-takoform`
- Terraform Registry: `registry.terraform.io/tako0614/takoform`
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
  name          = "assets"
  storage_class = "standard"
}
```

`endpoint`, `space`, and the sensitive bearer `token` can instead be supplied as `TAKOFORM_ENDPOINT`, `TAKOFORM_SPACE`, and `TAKOFORM_TOKEN`.
There is one provider source and state identity:
`registry.terraform.io/tako0614/takoform`. Both OpenTofu and Terraform install
that exact FQN from the Terraform Registry, and the lifecycle matrix proves
both CLI executions independently. The provider follows the same-origin
versioned endpoint advertised by discovery.
A host that advertises no versioned endpoint is rejected; there is no
unversioned lane to downgrade into.

## Resources

The portable Form set covers compute and application, data and storage,
analytics and inference, network and delivery, and operations and integration.
The complete list — every kind, its resource type, its version, its declared
runtime interfaces, and its immutable fields — is generated into
[the Form inventory](forms/README.md), and each resource has its own
[reference document](docs/resources/) and [example](examples/resources/).

Every Form is declared once, as data, in `internal/formcatalog`. The Terraform
schema, the wire spec, the Draft 2020-12 desired schema inside the Form
Definition, the conformance fixtures, the examples, and the reference docs are
all derived from that single declaration, so a Form cannot exist on one surface
and be missing from another.

The provider deliberately has no target-pool, backend, credential, pricing,
quota, billing, or operator-policy resources. It discovers
`features.service_forms` and verifies the exact build-pinned candidate
FormRef/package identity against the configured host. Backend placement,
admission, and credentials remain host responsibilities; state contains only
the canonical resource ID, generation fence, read-only drift status,
portability, desired typed fields, and sanitized public outputs.

A Form may declare the runtime interfaces its service exposes, with open
author-defined `(name, version)` identities such as `http.request@1`,
`object.storage@1`, or `model.invoke@1`. A declaration says what exists. It
carries no credential and grants no consumer anything: the host materializes
the record and authorizes its use.

See [the portable specification status](spec/README.md), [Form Package contract](spec/form-package/README.md), [interface declaration contract](spec/interface-declaration/README.md), [form inventory](forms/README.md), [conformance status](conformance/README.md), [provider documentation](docs/index.md), and [examples](examples/resources/).

The repository also contains a data-only Form Package library and CLI. It
implements strict UTF-8 I-JSON validation, RFC 8785 canonicalization, exact
FormRef/package-index identity, and closed local-directory verification without
network access or code execution:

```console
go run ./cmd/form-package conformance
go run ./cmd/standard-form-conformance verify
go run ./cmd/standard-form-conformance published-package-check
go run ./cmd/standard-form-conformance retained-ga-core-v1-published-package-check
go run ./cmd/form-package verify conformance/form-package-v1/positive/example-store
```

The protected Form Package release lane now builds deterministic package
evidence, keyless-signs the canonical index with Cosign v3, verifies its
Sigstore transparency bundle, attaches SPDX 2.3 and SLSA v1 evidence, and
publishes only an exact immutable GitHub Release inventory. A separate
append-only lane signs cumulative, hash-chained checkpoints for exact-digest
security revocations. See
[the Form Package release boundary](release/form-packages.md).

The previously published generation is retired, not erased. Its ten `1.0.1`
Form Packages have immutable live GitHub Releases whose exact asset
inventories, Git refs, production Sigstore trusted root, and package-index
workflow policy are retained under [`admission/v1/`](admission/v1/) and still
pass the offline `published-package-check`. Those bytes are never rewritten,
re-signed, or reshaped; [`forms/retired-package-set.json`](forms/retired-package-set.json)
records exactly what they were.

The first rebuilt ten-Form publication snapshot under
[`admission/v3/`](admission/v3/) remains immutable history. Product review
found that its `HttpService@1.0.0` name described the `http.request@1`
Interface rather than the requested execution class. The active successor is
the provider-neutral `EdgeWorker@2.0.0`: it keeps the neutral runtime,
concurrency, configuration, and immutable-artifact contract without restoring
Cloudflare compatibility fields. Its package and provider `v0.3.0` remain
unpublished candidates, so current admission is intentionally fail-closed
until new package, provider, host, dual-Registry, and admission evidence is
retained under the distinct [`admission/v4/`](admission/v4/) `ga-core-v2`
lane. The successor lane verifies provider reports over all 34
`portable-v1` Forms before selecting the exact ten successor identities.
GitHub Actions supplies distinct keyless publisher identities; all generation,
rederivation, and closure checks remain local Go commands.

## Development

Go 1.25.8 or later is required. Release builds are pinned to Go 1.26.5 or a
newer patched toolchain declared by the release descriptor.

```console
gofmt -w .
go vet ./...
go test ./...
go run ./cmd/provider-lifecycle-conformance matrix --opentofu tofu --terraform terraform
go run ./cmd/provider-lifecycle-conformance provider-reports \
  --cli terraform --output-dir /tmp/takoform-provider-reports \
  --source-commit "$(git rev-parse HEAD)"
go run ./cmd/form-package conformance
go run ./cmd/standard-form-conformance verify
go run ./cmd/standard-form-conformance candidate-publication-check
go run ./cmd/standard-form-conformance published-package-check
go run ./cmd/standard-form-conformance retained-ga-core-v1-published-package-check
go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
```

After the successor packages are published,
`current-published-package-check` authenticates the exact `admission/v4`
readback. It is intentionally outside `bun run check` because live publication
is a separate operator cadence.

`matrix` is the local `dev_overrides` regression gate. The separate
`render-registry-matrix` command performs a version-pinned direct Registry
install and exists for signed post-publication readback only. The Terraform
namespace and signing key are registered. Providers `v0.1.1` and `v0.1.2`
remain immutable GitHub Releases. Terraform Registry rejected `v0.1.1`
because its checksum manifest projected SPDX evidence as provider packages,
and rejected `v0.1.2` because it omitted the required Registry metadata
manifest checksum. The exact six-entry `v0.1.3` release is the non-overwriting
successor. The first rebuilt Form set uses immutable provider `v0.2.1`; the
`EdgeWorker@2.0.0` successor is implemented by the candidate provider
`v0.3.0`, whose Registry readback can exist only after publication.

Provider publication and Standard Form admission are separate authorities.
Provider publication never changes admission status. Current admission is a
source-retained, offline-authenticated closure over package, runner, Registry,
and admission-evidence subjects; there is no set-wide release artifact or
controller promotion path.

Provider releases use the fail-closed signed `v*` tag workflow documented in
[release/README.md](release/README.md). The signing key is pinned by fingerprint;
the private key remains outside the repository. The `tako0614` public namespace
and pinned signing key are registered. Do not create a new release tag until
the release descriptor and provider compatibility gates are complete. Existing
version paths are immutable, so retired provider `0.1.3`, published provider
`0.2.1`, and candidate successor `0.3.0` remain distinct identities.

## License

MIT
