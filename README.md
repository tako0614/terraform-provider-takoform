# Takoform Provider

`takoform` is a thin Terraform/OpenTofu provider for portable Service Forms. It gives HCL authors ten statically typed service resources and sends their desired state to any conforming Takoform host. The host—not this provider—selects and operates the concrete backend.

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
  name       = "assets"
  interfaces = ["s3_api"]
}
```

`endpoint`, `space`, and the sensitive bearer `token` can instead be supplied as `TAKOFORM_ENDPOINT`, `TAKOFORM_SPACE`, and `TAKOFORM_TOKEN`.

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

The provider deliberately has no target-pool, backend, credential, pricing, quota, billing, or operator-policy resources. It discovers `features.service_forms` and the supported form kinds from the configured host. Backend placement and credentials remain host responsibilities; computed fields only report sanitized resolution evidence.

See [the portable specification status](spec/README.md), [form inventory](forms/README.md), [conformance status](conformance/README.md), [provider documentation](docs/index.md), and [examples](examples/resources/).

## Development

Go 1.23 or later is required.

```console
gofmt -w .
go vet ./...
go test ./...
```

Provider releases use the fail-closed signed `v*` tag workflow documented in
[release/README.md](release/README.md). The signing key is pinned by fingerprint;
the private key remains outside the repository. Do not create a release tag
until the release descriptor, Registry public-key registration, and provider
compatibility gates are complete.

## License

MIT
