# Takoform Provider

Manage exact Takoform Forms with ordinary Terraform or OpenTofu HCL. The
Provider maps typed resources to a compatible Host's contracts; the Host runs
the service. Declare it beside AWS, Cloudflare, Random, or other Providers
and connect resources through the normal HCL dependency graph.

## Current path: Host API v2

| Version | Host API | Resource mappings | Start here |
| --- | --- | --- | --- |
| **5.0.0**, source target / candidate | `forms.takoform.com/v2` | 13 exact Edge Form URLs | [Provider 5 consumer reference](docs/provider-host-api-v2-source.md) |
| **4.1.0**, Registry-published legacy | `forms.takoform.com/v1` | 17 exact Edge Forms | [Legacy getting-started guide](docs/getting-started.md) · [日本語](docs/ja/getting-started.md) |

The default source-built Provider uses Host API v2. Host API v1 and Registry
Provider 4.1.0 are deprecated; the default Provider 5 entrypoint does not
support v1. The 5.0.0 descriptor is a source target, not Registry installation
evidence. Provider 4.1.0 remains a published release; Provider and Host API
release status are independent. **Provider 4 state is not automatically
translated to Provider 5.** Keep an existing v4 workspace on v4 unless a
separate migration is planned and verified.

## Quick start: Provider 5 source

Evaluate the current v2 implementation from source with a fresh module and
private intent store. This does not install a Registry release. Follow the
[Provider 5 consumer reference](docs/provider-host-api-v2-source.md) for the
local plugin build, CLI override, exact configuration, and custody steps.
The Host must independently support `forms.takoform.com/v2` and each exact
Form URL; this Provider does not create a Host or publish Forms.

```console
PROVIDER_DEV_DIR="$(mktemp -d)"
go build -ldflags="-X main.version=5.0.0-dev" \
  -o "$PROVIDER_DEV_DIR/terraform-provider-takoform" .
```

For local custody, choose an absolute private store path and a fixed caller
scope ID, then initialize the store before configuring `store_path`:

```console
go run ./cmd/intent-store-init \
  --path "$INTENT_STORE_PATH" \
  --store-id "$CALLER_SCOPE_ID" \
  --host-api "https://host.example/apis/forms.takoform.com/v2"
```

Use the exact API root supplied by your Host operator. This command initializes
local custody and does not contact the Host. Remote custody uses an
`intent_store_slot` explicitly granted by the execution environment instead.

For the current v2 schema and lifecycle, continue to the
[Provider 5 consumer reference](docs/provider-host-api-v2-source.md). Check the
[Registry listing](https://registry.terraform.io/providers/tako0614/takoform/latest)
and exact-version readback before treating Provider 5.0.0 as published.

## Legacy: Registry Provider 4.1.0 (deprecated)

This published v1 client is retained for existing v4 workspaces and its
[legacy getting-started guide](docs/getting-started.md). It is unsupported by
the default source-built Provider 5 entrypoint. Provider `4.1.0` is published on the Terraform Registry.

You need a Host endpoint, an exact Space ID, and any required authentication
from the Host operator. The Host must advertise and authorize the exact Forms
you use. The Provider does not create a Host or select its backend.

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 4.1.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://forms.example.com"
  space    = "prod"
}

resource "takoform_edge_kv_namespace" "example" {
  name = "example"
}
```

Replace the endpoint and Space with values supplied by your Host operator.
`TAKOFORM_ENDPOINT`, `TAKOFORM_SPACE`, and `TAKOFORM_TOKEN` are the v4
configuration fallbacks. Keep bearer tokens out of checked-in HCL and variable
files. Provider 4.1.0 also accepts `token_file` / `TAKOFORM_TOKEN_FILE` for a
rotating token; its file requirements and supported operating systems are in
the [Provider reference](docs/index.md).

Follow the [getting-started guide](docs/getting-started.md) to initialize,
validate, and plan the [Random + Takoform module](examples/getting-started/main.tf).
Random needs no cloud credentials; applying the Takoform resource changes the
selected Host. For a cloud resource alongside Takoform, see the
[AWS S3 composition example](examples/native-provider-composition/main.tf).

## References and ownership

- [Provider reference](docs/index.md): current v2 source reference and deprecated v4 resource roster.
- [Versions and compatibility](website/docs/versions.md): release evidence and migration history.
- [Host API documentation](https://takoform.com/en/host-api/): the separate provider-neutral API authority.
- [Edge Forms publisher](https://github.com/tako0614/takoform-forms): Form definitions and publication.
- [Conformance](conformance/README.md): executable checks and their coverage limits.
- [Source boundary inventory](docs/source-boundary-inventory.md): current entrypoints and retained extraction history.

Provider versions, Form definitions, and Host support are independent. A
Provider release does not publish Forms or grant Host support. This Provider
selects exact Forms from `tako0614/takoform-forms`; other Providers keep their
own source, version, credentials, resources, and state.

<!-- current-generation:begin -->

The default source-built Provider uses current Host API `forms.takoform.com/v2`. Provider `5.0.0` is a source target/candidate, not Registry publication evidence. The latest Registry-published Provider is `4.1.0`; Provider 4.1.0 remains published legacy v1 history with the 17 Forms selected from `tako0614/takoform-forms`. The default Provider 5 entrypoint does not support v1. Provider release and Host API support are independent. See [Versions and compatibility](website/docs/versions.md).

<!-- current-generation:end -->

## Development

Install dependencies with `bun install --frozen-lockfile`. The complete,
read-only repository gate requires Git, Go/gofmt, Node, Terraform, and OpenTofu:

```console
bun run check
```

These commands are read-only.

The worker-authoring matrix inside that gate uses an explicitly tagged,
retained-v1 plugin and the stable-v1 reference Host as legacy regression
coverage. The reference Host serves no application traffic. That matrix does
not exercise the default v2-only `New()` entrypoint;
`--provider-binary` tests an explicitly supplied binary. The native v2 fixture is a separate check:
`go test -tags native ./native-fixture/mounted-v2`.

Documentation projections are refreshed explicitly with
`bun run sync:website-docs`, `bun run sync:website-spec`, and
`bun run website:build`. The build preserves immutable published bytes.
See [website maintenance](website/README.md) before editing generated pages.

## License

MIT
