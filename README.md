# Takoform Provider

Takoform Provider is a Terraform/OpenTofu client for a compatible Takoform
Host. It maps typed resource configuration to exact Form contracts and keeps
the resulting identity and desired state in Terraform state; the Host runs
the service.

## Quick start

The `registry.terraform.io/tako0614/takoform` source address maps an explicit
set of Forms published by `github.com/tako0614/takoform-forms`. Its current
major registers only the 17 exact Forms in `edge.forms.takoform.com`. Provider
`3.0.0` remains immutable Registry history for the former 31-Form aggregate; it
is not the roster that future releases extend.

Pin the Provider from the canonical Registry address and configure a Host:

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "~> 4.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://forms.example.com"
  space    = "prod"
}

resource "takoform_module_worker" "api" {
  name = "api"
}
```

`endpoint`, `space`, and bearer `token` may instead come from
`TAKOFORM_ENDPOINT`, `TAKOFORM_SPACE`, and `TAKOFORM_TOKEN`.

The repository's reference Host is for conformance only and serves no
application traffic.

Provider `4.0.0` is published on the Terraform Registry at that address. The
signed tag, the immutable GitHub Release, and the Registry readback that prove
it are the `4.0.0` entry of the
[Provider release identity ledger](release/provider-release-identities.json).
Users staying on Provider `3.0.0` keep an explicit `= 3.0.0` pin and cross the
[v3-to-v4 migration boundary](release/migrations/v3-to-v4.md) when they upgrade.

## Resource model

Each resource is a typed Provider mapping to one current exact FormRef. Form
schemas, canonical bytes, and digests stay in the Form contract; Provider
resource names are adapter metadata.

| Family | Resources |
| ------ | --------: |
| Edge   |        17 |

<!-- current-generation:begin -->

Registry Provider `4.0.0` is the published release at the `tako0614/takoform` source address and registers only the 17 Forms selected from `tako0614/takoform-forms`. Provider `3.0.0` remains immutable 31-Form aggregate history. Core `1.0.1` implements `forms.takoform.com/v1`; no Provider release changes that API. See [Versions and compatibility](website/docs/versions.md).

<!-- current-generation:end -->

The [Provider reference](docs/index.md) lists every resource with its full
FormRef, arguments, state, and import contract. The [mapping inventory](forms/README.md)
lists the roster and each `definitionVersion`.

## Native OpenTofu provider composition

Takoform does not wrap or re-publish resources owned by AWS, Cloudflare,
Kubernetes, PostgreSQL, or other provider ecosystems. Declare those providers
beside `takoform` in the same module and connect them with ordinary HCL
references or dependency edges. Provider installation, version selection,
state, aliases, and credentials remain native OpenTofu concerns.

See the [AWS plus Takoform example](examples/native-provider-composition/main.tf).

## Development

```console
go test ./...
go run ./cmd/worker-authoring-conformance matrix --opentofu tofu --terraform terraform
go run ./cmd/standard-form-conformance verify
bun run check
```

Generated resource pages and examples are reproducible from the checked-in
Form catalog. These commands are read-only.

## License

MIT
