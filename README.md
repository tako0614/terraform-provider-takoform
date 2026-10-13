# Takoform Provider

Manage exact Takoform Forms with ordinary Terraform or OpenTofu HCL. The
Provider maps typed resources to a compatible Host's contracts; the Host runs
the service. Declare it beside AWS, Cloudflare, Random, or other Providers
and connect resources through the normal HCL dependency graph.

## Choose a Provider version

| Version | Host API | Resource mappings | Start here |
| --- | --- | --- | --- |
| **4.1.0**, Registry-published | `forms.takoform.com/v1` | 17 exact Edge Forms | [Getting started](docs/getting-started.md) · [日本語](docs/ja/getting-started.md) |
| **5.0.0**, source target | `forms.takoform.com/v2` | 13 exact Edge Form URLs | [Provider 5 consumer reference](docs/provider-host-api-v2-source.md) |

Provider `4.1.0` is published on the Terraform Registry. The `5.0.0` descriptor
is a source target, not Registry installation evidence. Check the exact version in the
[Registry listing](https://registry.terraform.io/providers/tako0614/takoform/latest)
before changing a pin. **Provider 4 state is not automatically translated to
Provider 5.** Keep an existing v4 workspace on v4 until a separate migration
is planned and verified.

## Quick start: Provider 4.1.0

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

## Evaluate the Provider 5 source

The default source-built plugin speaks Host API v2 and registers 13 resources.
It needs an exact `host_api_root`, a fixed `store_id`, and exactly one private
intent-store mode. Use an initialized absolute `store_path`, or an
`intent_store_slot` explicitly granted by the Runner/OpenTofu environment.
The Host and private-file capability must be supplied by your environment.

For local custody, initialize the store before configuring the Provider:

```console
go run ./cmd/intent-store-init \
  --path "$INTENT_STORE_PATH" \
  --store-id "$CALLER_SCOPE_ID" \
  --host-api "https://host.example/apis/forms.takoform.com/v2"
```

This initializes local custody; it does not contact a Host or install the
Provider. See the [Provider 5 consumer reference](docs/provider-host-api-v2-source.md)
for source evaluation, typed HCL, remote custody, import, and lifecycle
boundaries. Use the exact Registry version only after publication is verified.

## References and ownership

- [Provider reference](docs/index.md): configuration and the published v4 resource roster.
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

Registry Provider `4.1.0` is the published release at the `tako0614/takoform` source address and registers only the 17 Forms selected from `tako0614/takoform-forms`. Provider `5.0.0` is the candidate at the same source address. Provider `3.0.0` remains immutable 31-Form aggregate history. Core `1.0.1` implements `forms.takoform.com/v1`; no Provider release changes that API. See [Versions and compatibility](website/docs/versions.md).

<!-- current-generation:end -->

## Development

Install dependencies with `bun install --frozen-lockfile`. The complete,
read-only repository gate requires Git, Go/gofmt, Node, Terraform, and OpenTofu:

```console
bun run check
```

These commands are read-only.

The worker-authoring matrix inside that gate uses an explicitly tagged,
retained-v1 plugin and the stable-v1 reference Host. The reference Host serves
no application traffic. That matrix does not exercise the default v2-only
`New()` entrypoint; `--provider-binary` tests an explicitly supplied binary.
The native v2 fixture is a separate check:
`go test -tags native ./native-fixture/mounted-v2`.

Documentation projections are refreshed explicitly with
`bun run sync:website-docs`, `bun run sync:website-spec`, and
`bun run website:build`. The build preserves immutable published bytes.
See [website maintenance](website/README.md) before editing generated pages.

## License

MIT
