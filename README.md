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
      version = "= 1.0.2"
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
`registry.terraform.io/tako0614/takoform`. The configuration above deliberately
pins the published provider to `1.0.2`. The signed release is installable from
the canonical Terraform Registry, and the retained authenticated Registry
readback proves direct installation with both OpenTofu and Terraform. The local
development lifecycle matrix exercises the exact same FQN through CLI
development overrides in both CLIs; it is a separate source-level check. The
provider follows the same-origin versioned endpoint advertised by discovery.
A host that advertises no versioned endpoint is rejected; there is no
unversioned lane to downgrade into.

`release/version.json` intentionally retains
`publicationStatus: candidate-only` as release-descriptor metadata from the
candidate that minted `v1.0.2`. That field is not live availability state. The
signed release and canonical Registry readback establish provider publication.
The protected admission tag and offline-authenticated retained closure
separately establish Standard Form admission.

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
`features.service_forms` and verifies the exact build-pinned
FormRef/package identity against the configured host. Backend placement,
admission, and credentials remain host responsibilities; state contains only
the exact FormRef/package identity, a locally synthesized `Kind/name` resource
ID, generation fence, desired typed fields, and Form-schema-validated observed
and public output projections. A host-provided opaque ID, undeclared output
key, backend target, credential, or commercial field is rejected before state.

Provider `v1.0.1` is not an in-place state upgrade from `v0.2.1`: every old
exact Form identity changed, and the old writable Interface resource was
removed. Provider v1 starts a new fail-closed resource schema version and has
a diagnostic-only v0.2.1 rejection handler, not an automatic state
transformation. See the
[`v0.2.1` to `v1.0.1` migration boundary](release/migrations/v0.2.1-to-v1.0.1.md)
before changing an existing provider constraint or OpenTofu provider address.

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
superseded its execution-class identity with the provider-neutral
`EdgeWorker@3.0.0`. The resource describes an edge/event runtime, concurrency,
configuration, and an immutable artifact without selecting a vendor or a
machine-hosting model. The new major also makes the persisted artifact URL
credential-free by forbidding userinfo, query, and fragment; the earlier
release source is not reshaped. Provider v1 also tightens the common
resource-name, connection, artifact, and
semantic-value contracts across all 34 portable Forms. Existing Form release
sources remain immutable history.

All 34 exact package identities in
[`forms/release-plan.json`](forms/release-plan.json) are published as signed,
immutable Form Package releases. The protected
`forms/admissions/v1.0.6` identity closes the retained
[`admission/v4/`](admission/v4/) evidence and admits exactly these 10 as
`portable-standard`: `EdgeWorker`, `ContainerService`, `StatefulEntity`,
`Schedule`, `ObjectBucket`, `KeyValueStore`, `RelationalDatabase`, `Queue`,
`VectorIndex`, and `ModelEndpoint`. The other 24 packages are published but
not admitted. Provider reports cover all 34 Forms; the admission decision
additionally binds the selected-ten host report and the provider `v1.0.1`
two-CLI Registry readback. Provider publication, package publication, and
Standard Form admission remain separate authorities.
GitHub Actions supplies distinct keyless publisher identities; all generation,
rederivation, and closure checks remain local Go commands.

## Development

Go 1.25.8 or later is required. Release builds are pinned to Go 1.26.5 or a
newer patched toolchain declared by the release descriptor.

```console
gofmt -w .
go vet ./...
go test ./...
go run ./cmd/portable-host-conformance self-test
go run ./cmd/provider-lifecycle-conformance matrix --opentofu tofu --terraform terraform
go run ./cmd/provider-lifecycle-conformance provider-reports \
  --cli terraform --output-dir /tmp/takoform-provider-reports \
  --source-commit "$(git rev-parse HEAD)"
go run ./cmd/form-package conformance
go run ./cmd/standard-form-conformance verify
go run ./cmd/standard-form-conformance candidate-publication-check
go run ./cmd/standard-form-conformance published-package-check
go run ./cmd/standard-form-conformance retained-ga-core-v1-published-package-check
go run ./cmd/standard-form-conformance current-published-package-check
go run ./cmd/standard-form-conformance current-admission-closure-check
go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
```

`check:public-surfaces`, and therefore `bun run check`, authenticates the exact
retained all-34 publication set and the protected `admission/v4` closure before
deriving any public availability claim. Retained-evidence authentication uses
only source-retained bytes and Git objects: it verifies signatures, immutable
release refs, Registry readback, and the admission tag without contacting a
distribution target or hosted Resource backend. The production entrypoint may
fill its isolated, checksum-verified Go module cache from the pinned public Go
proxy before that authentication runs.
It is a composite of `check:public-authority`, which requires complete isolated
Git authority, and `check:public-snapshot`, which verifies the static website,
schema, copy, and deploy-safety surfaces without requiring repository metadata.
The production website entrypoint runs those halves from separate frozen roots
of the same exact commit.

`matrix` is the local `dev_overrides` regression gate. The separate
`render-registry-matrix` command performs a version-pinned direct Registry
install and exists for signed post-publication readback only. The Terraform
namespace and signing key are registered. Providers `v0.1.1` and `v0.1.2`
remain immutable GitHub Releases. Terraform Registry rejected `v0.1.1`
because its checksum manifest projected SPDX evidence as provider packages,
and rejected `v0.1.2` because it omitted the required Registry metadata
manifest checksum. The exact six-entry `v0.1.3` release is the non-overwriting
successor. The first rebuilt Form set uses immutable provider `v0.2.1`; the
`EdgeWorker@3.0.0` successor is implemented by published provider `v1.0.1`,
whose authenticated Registry readback is retained in the v4 closure.

Provider publication and Standard Form admission are separate authorities.
Provider publication never changes admission status. The current admission is
the source-retained, offline-authenticated v4 closure over package, runner,
Registry, and admission-evidence subjects, identified by the protected
`forms/admissions/v1.0.6` tag. There is no set-wide release artifact or
controller promotion path.

Provider releases use the fail-closed signed `v*` tag workflow documented in
[release/README.md](release/README.md). The signing key is pinned by fingerprint;
the private key remains outside the repository. The `tako0614` public namespace
and pinned signing key are registered. Do not create a new release tag until
the release descriptor and provider compatibility gates are complete. Existing
version paths are immutable, so retired provider `0.1.3`, published provider
`0.2.1`, signed-but-never-published provider `1.0.0`, and published provider
`1.0.1` remain distinct identities. Provider `1.0.1` implements
`EdgeWorker@3.0.0`; provider and Form versions are independent by design.

## License

MIT
