# Takoform Provider

Takoform Provider is a Terraform/OpenTofu client for a compatible Takoform
Host. It maps typed resource configuration to exact Form contracts and keeps
the resulting identity and desired state in Terraform state; the Host runs
the service.

## Quick start

Pin the provider from the canonical Registry address and configure a Host:

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 3.0.0"
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

## Resource model

Each resource is a typed Provider mapping to one current exact FormRef. Form
schemas, canonical bytes, and digests stay in the Form contract; Provider
resource names are adapter metadata.

| Family | Resources |
| --- | ---: |
| Edge | 16 |
| Function | 4 |
| Container | 5 |
| Queue | 1 |
| Schedule | 1 |
| Table | 1 |
| Topic | 2 |
| Vector | 1 |

<!-- current-generation:begin -->

Current releases: Provider `3.0.0` and API/Core `1.0.1` on `forms.takoform.com/v1`. See [Versions and compatibility](website/docs/versions.md) for retained releases and migration.

<!-- current-generation:end -->

The [Provider reference](docs/index.md) lists every resource with its full
FormRef, arguments, state, and import contract. The [mapping inventory](forms/README.md)
lists the roster and each `definitionVersion`.

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
