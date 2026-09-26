English: [日本語](ja/getting-started.md)

# Manage a published Form from HCL

This guide introduces a small Terraform/OpenTofu module with one Random
Provider resource and one Takoform resource. The example is
[`examples/getting-started/main.tf`](../examples/getting-started/main.tf).

This Provider is for HCL authors who want to manage an explicit set of
published Takoform Forms. It is an ordinary peer of AWS, Cloudflare, and other
Providers: you declare each source in `required_providers`, then OpenTofu or
Terraform resolves the dependency graph. This Provider does not create a
Host, choose its backend, or provide a catalog of every Form.

## Before you start

Have these ready before planning:

- Terraform or OpenTofu and a compatible Takoform Host.
- The Host's endpoint, exact Space ID, and (if that Host requires it) a bearer
  token from the Host operator. The Host must support the exact FormRef used by
  each resource.
- The Random Provider needs no cloud credentials. Applying the Takoform
  resource changes the selected Host.

The Takoform endpoint and Space are supplied by your selected Host; there is
no default Host or Space. Keep a bearer token out of HCL and variable files.
If needed, provide it to the process as `TAKOFORM_TOKEN` through your normal
secret-injection mechanism. `TAKOFORM_ENDPOINT` and `TAKOFORM_SPACE` are also
supported when you configure the Provider through its environment variables.

## Declare both Providers

Use the checked-in [example module](../examples/getting-started/main.tf). It
declares `registry.terraform.io/tako0614/takoform` beside the
`registry.terraform.io/hashicorp/random` Provider. The Random resource provides
a generated suffix for the `takoform_edge_kv_namespace` name through a normal
HCL reference. The Random Provider has no cloud account to configure; the
Takoform Provider alone manages the namespaced resource on the selected Host.
For a cloud-provider composition example, see the existing
[AWS S3 example](../examples/native-provider-composition/main.tf).

From the example directory, check formatting, initialize the module, and
validate it. After supplying the required inputs and credentials, review the
plan before applying:

```console
cd examples/getting-started
tofu fmt -check
tofu init
tofu validate
tofu plan
```

Use `terraform` in place of `tofu` when working with Terraform. Supply these
two non-secret inputs through your usual variable mechanism: Host endpoint and
Space ID. Reviewing the plan is important because applying it creates a
namespace on the selected Host. Apply only after confirming the target and the
planned changes.

This resource maps to the published [EdgeKVNamespace Form at definition
0.1.0](https://edge.forms.takoform.com/forms/EdgeKVNamespace/0.1.0/). A
compatible Host must advertise support for that exact FormRef before it can
apply the resource. The separate [Host API documentation](https://takoform.com/en/host-api/)
describes the provider-neutral Host contract; it does not supply a hosted
endpoint or credentials.

## Provider versions and Form identities

In the example, `version = "~> 4.0"` constrains the Provider package to the
4.x line. `tofu init` records the selected Provider package and checksums in
`.terraform.lock.hcl`; commit that lock file with a real module. This version
constraint is **not** a Form pin. The Provider release maps each resource type
to an exact FormRef; state records the API version, kind, definition version,
and schema digest used by that resource. For this example, the mapping is
`edge.forms.takoform.com/EdgeKVNamespace`, definition `0.1.0`. See the
[resource reference](resources/edge_kv_namespace.md#exact-formref) for the
complete identity and state behavior.

## Sensitive WorkerVersion inputs

`TAKOFORM_TOKEN` is a credential for the Host API. It is different from
application secrets that Worker code needs at runtime. A
[`takoform_worker_version`](resources/worker_version.md) declares only the
names of required sensitive values in `required_sensitive_vars`. The values
are delivered through the Provider's Apply-only `runtime_inputs` map using an
ephemeral root-variable path supported by the execution runner; the values do
not enter a saved plan or Terraform/OpenTofu state. Do not put these values in
HCL literals, `vars_json`, ordinary `.tfvars` files, command arguments, or
ambient variables. Read [Run-scoped sensitive inputs](resources/worker_version.md#run-scoped-sensitive-inputs)
before configuring this path: the nonce, Provider instance, and Apply-time
map must match, and a runner that does not implement ephemeral delivery cannot
apply those inputs.

## Next

- Browse the [complete resource reference](index.md#resource-reference) and
  [Provider-to-Form mapping inventory](../forms/README.md).
- Read the [Takoform Host API](https://takoform.com/en/host-api/) separately
  when you need the provider-neutral Host contract.
- Before upgrading Provider 3, read the
  [v3-to-v4 migration guide](../release/migrations/v3-to-v4.md).
