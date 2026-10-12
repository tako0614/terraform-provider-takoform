# Takoform Provider

Use ordinary Terraform/OpenTofu HCL to manage exact Takoform resources served
by a compatible Host. This is a typed Provider you declare beside AWS,
Cloudflare, or other Providers—not a universal Takoform API client or a
provider catalog. Each Provider keeps its own source, version, credentials,
resources, and state.

For Registry-published Provider 4.1.0 (Host API v1), start with the
[HCL author getting-started guide](docs/getting-started.md) and its
[Random + Takoform example](examples/getting-started/main.tf). The Random
resource needs no cloud credentials; the Takoform Provider still needs the
selected Host's endpoint, Space, and any required authentication. That guide
uses the published [EdgeKVNamespace Form, definition
0.1.0](https://edge.forms.takoform.com/forms/EdgeKVNamespace/0.1.0/)
and keeps that resource under the selected Host. For an AWS S3 example, see
[native provider composition](examples/native-provider-composition/main.tf).

The compatible Host supplies the endpoint, Space, and (if required) bearer
token. Provider 4.1.0 neither selects nor creates a Host. For the separate
provider-neutral contract and Host API, see the [Takoform Host API
documentation](https://takoform.com/en/host-api/).

## Quick start: Registry-published Provider 4.1.0 (Host API v1)

This quick start is for the Registry-published Provider 4.1.0 and its Host API
v1 contract. It registers the 17 exact Forms selected from
`github.com/tako0614/takoform-forms`. Provider `3.0.0` remains immutable
Registry history for the former 31-Form aggregate; it is not the roster that
future releases extend. The current source target is Provider 5.0.0, described
separately below; it is not the configuration shown here.

Pin the Provider from the canonical Registry address and configure a Host:

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

resource "takoform_module_worker" "api" {
  name = "api"
}
```

`endpoint`, `space`, and bearer `token` may instead come from
`TAKOFORM_ENDPOINT`, `TAKOFORM_SPACE`, and `TAKOFORM_TOKEN`.
Replace the example endpoint and Space with values supplied for your chosen
Host; do not put a bearer token in HCL or a checked-in variable file.

Provider 4.1.0 adds `token_file` / `TAKOFORM_TOKEN_FILE` for a rotating
run-local bearer token. It reopens the file before every Host request. Use a
regular file owned by the provider process user, without group/other access,
symlinks, parent traversal, or a trailing newline. Configure either a static
token or a token file, never both. Existing 4.0.0 static-token consumers need
not change their pin. Secure token-file access is supported on Linux, macOS,
FreeBSD, OpenBSD, and NetBSD; Windows keeps static-token configuration only.
Provider 4.1.0 is Registry-published; verify the exact Registry version when
changing a consumer pin. The release descriptor alone does not establish publication.

The repository's reference Host is for conformance only and serves no
application traffic.

Provider `4.1.0` is published on the Terraform Registry at that address. The
signed tag, the immutable GitHub Release, and the Registry readback that prove
it are the `4.1.0` entry of the
[Provider release identity ledger](release/provider-release-identities.json).
Users staying on Provider `3.0.0` keep an explicit `= 3.0.0` pin and cross the
[v3-to-v4 migration boundary](release/migrations/v3-to-v4.md) when they upgrade.

## Current source target: Provider 5.0.0 (Host API v2)

The current source checkout targets a fresh-install Provider 5.0.0 that uses
Host API `forms.takoform.com/v2` and maps 13 exact published Edge Form URLs.
The source target is distinct from Registry availability: install Provider
5.0.0 from the Registry only after its exact version appears in the live
Registry listing and the version readback is verified. Until then, use a source
checkout or local build for evaluation, not Registry installation. This
repository does not establish that any Host supports the v2 API or these Forms.
Provider 4.1.0 remains the published v1 release, and Provider 4 state is not
automatically translated to Provider 5.

The [Provider 5 consumer reference](docs/provider-host-api-v2-source.md) lists
all 13 resource-to-Form URL mappings and lifecycle boundaries. Its v2
configuration requires the exact Host API root and exactly one private
intent-store mode. Select either an initialized absolute
`store_path`, or an explicit `intent_store_slot` backed by a Runner/OpenTofu
environment that grants generic private-file access for that slot. The remote
capability is not provisioned by the Provider and is not implied by a Cloud
runtime or ambient credentials. `store_id` is a local label, not
authentication or a Host principal. For local custody, initialize the store
with:

```console
go run ./cmd/intent-store-init \
  --path "$INTENT_STORE_PATH" \
  --store-id "$CALLER_SCOPE_ID" \
  --host-api "https://host.example/apis/forms.takoform.com/v2"
```

This command initializes local store custody; it does not contact a Host or
install the Provider. A local-mode source-built v5 Provider block has this
shape; it is not a Registry installation declaration:

```hcl
provider "takoform" {
  endpoint      = "https://host.example"
  host_api_root = "https://host.example/apis/forms.takoform.com/v2"
  token         = var.host_token
  store_path    = var.absolute_private_store_path
  store_id      = var.fixed_caller_scope_id
  space         = "prod"
}

resource "takoform_module_worker" "api" {
  name = "api"
}
```

Remote custody uses the same fixed `store_id` and exact Host API root, but only
when the execution environment explicitly grants private-file access for the
selected opaque slot. During Configure, before resource planning, the Provider
may initialize an absent slot with an empty snapshot bound to that identity
using compare-and-swap. This is private-store traffic, not a Host API request;
an existing mismatch or unresolved store commit stops before Host mutation.
For example, set `intent_store_slot = var.runner_private_slot` instead of
`store_path` only when the Runner/OpenTofu environment grants that exact slot.
For example:

```hcl
provider "takoform" {
  endpoint          = "https://host.example"
  host_api_root     = "https://host.example/apis/forms.takoform.com/v2"
  token             = var.host_token
  intent_store_slot = var.runner_private_slot
  store_id          = var.fixed_caller_scope_id
  space             = "prod"
}
```

The local initializer above applies only to `store_path` mode.

The Host must advertise and authorize the exact Form/API contract used; this
repository does not claim that a Host does. Do not use this v2 configuration in
an existing v4 workspace or rely on automatic state migration.

## Resource model

In the published Provider 4.1.0/v1 model, each resource maps to one exact
FormRef; its v4 state records the Form version, kind, definition version, and
schema digest. Provider 5/v2 instead maps each resource to one exact publisher
Form URL, recorded in the `form_url` state attribute. Form schemas and
canonical bytes remain in their respective Form contracts; resource names are
adapter metadata.

The published v4.1.0 resource table is:

| Family | Resources |
| ------ | --------: |
| Edge   |        17 |

<!-- current-generation:begin -->

Registry Provider `4.1.0` is the published release at the `tako0614/takoform` source address and registers only the 17 Forms selected from `tako0614/takoform-forms`. Provider `5.0.0` is the candidate at the same source address. Provider `3.0.0` remains immutable 31-Form aggregate history. Core `1.0.1` implements `forms.takoform.com/v1`; no Provider release changes that API. See [Versions and compatibility](website/docs/versions.md).

<!-- current-generation:end -->

The [Provider reference](docs/index.md) documents the published v4.1.0
resources. The [Provider 5 consumer reference](docs/provider-host-api-v2-source.md)
documents the 13 source-v5 resource mappings. The [mapping inventory](forms/README.md)
lists the retained v4 roster and each `definitionVersion`.

In the v4.1.0 example, `version = "= 4.1.0"` constrains the Provider package;
it is not a Form-version pin. Each resource type maps to a specific FormRef,
and v4 resource state records
the API version, kind, definition version, and schema digest in use. See
[Form identity and state](docs/resources/edge_kv_namespace.md#exact-formref)
for one mapping.

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
Form catalog. These commands are read-only. The worker-authoring matrix uses an
explicitly tagged, local retained-v1 plugin to test the published v1 HCL and
stable-v1 reference Host; it does not exercise the shipping v2-only `New()`
entrypoint. `--provider-binary` instead tests the exact caller-supplied binary
without substituting that retained plugin. The native v2 fixture is a separate
tagged check (`go test -tags native ./native-fixture/mounted-v2`).

## License

MIT
