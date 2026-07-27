# Takoform Provider

`takoform` is a thin Terraform/OpenTofu provider for portable Service Forms. It gives HCL authors 34 statically typed service resources and sends their desired state to any conforming Takoform host. The host—not this provider—selects and operates the concrete backend.

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
  name          = "assets"
  storage_class = "standard"
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

The rebuilt Forms have no published release, no Registry readback, and no
admission evidence of their own, so every publication gate fails closed and
this build refuses to reissue the retired generation's proofs under a new
provider identity. All candidate generation and verification runs through the
local Go CLI; GitHub Actions is an optional automation and the current keyless
OIDC signer, not a prerequisite for the local gates.

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
non-overwriting successor, and its retained Registry readback belongs to the
Forms it shipped. The rebuilt Form set takes the next minor line, `0.2.0`,
which has no release and no readback of its own yet.

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
version paths are immutable, so the retired coordinated `1.0.1` Form pin keeps
provider `0.1.3` and the rebuilt set starts at `0.2.0`.

## License

MIT
