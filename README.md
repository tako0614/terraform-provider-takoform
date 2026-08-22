# Takoform

Takoform is an **Experimental specification and tooling project** for
host-neutral desired-state contracts. It is not an industry standard, a cloud
catalog, or a promise that a workload can move between backends without
migration.

Takoform does not define least-common-denominator cloud resources. A Form
fixes the application-visible shape of one proven service primitive —
execution ABI, consistency, delivery guarantees, update units — completely,
and leaves only the vendor's identity, account, placement, and commerce
outside the contract. Hosts are exchangeable; resource semantics are not
([decision 0008](spec/decisions/0008-forms-preserve-service-shape.md)).

## Which identity is current

<!-- current-generation:begin -->

| Axis | Current identity | |
| --- | --- | --- |
| Host API lane | `forms.takoform.com/v1beta1` | the wire a provider speaks |
| Form Family | `edge.forms.takoform.com/v1beta1` | 15 Forms, each `0.1.0` and experimental |
| Form Package envelope | `packages.forms.takoform.com/v1alpha4` | package artifacts are unpublished |
| Provider | `2.1.1` | installed from the Terraform Registry |

These four numbers do not line up, and they are not supposed to: the axes
change for different reasons. They also do not sort the same way — the lane
went `v1alpha3` → `v1beta1`, where the digit falls, while the envelope went
`v1alpha3` → `v1alpha4`, where it rises. **Do not infer which identity is
current from a version word.** This table is generated from the repository
by `bun run sync:current-generation`, and 12 publication obligations in
[`spec/publication-blockers.json`](spec/publication-blockers.json) are open.

<!-- current-generation:end -->

## One epoch, three published provider lines

The specification carries **one epoch**: the Beta Host API lane
`forms.takoform.com/v1beta1`, the Edge Platform Family
`edge.forms.takoform.com/v1beta1`
([spec/form-families.md](spec/form-families.md)), and the
`packages.forms.takoform.com/v1alpha4` package envelope. The family is exactly
15 Forms: `ModuleWorker`, `WorkerBundle`, `StaticAssetBundle`, `WorkerVersion`,
`WorkerDeployment`, `WorkerCustomDomain`, `WorkerEndpoint`, `WorkerCronTrigger`,
`EdgeKVNamespace`, `ObjectBucket`, `SQLiteDatabase`, `SQLiteMigrationSet`,
`SQLiteMigrationApplication`, `AtLeastOnceQueue`, and `QueueConsumer`, with
exact Interface and typed Binding contracts under
`interfaces.takoform.com/v1alpha1` and `bindings.takoform.com/v1alpha1`. All 15
definitions are `0.1.0` and Experimental; their package artifacts remain
unpublished. Experimental is Form maturity, not package-publication or Host-GA
status ([decision 0035](spec/decisions/0035-beta-contracts-ship-in-stable-provider-v2-1.md)).

The two pre-Beta epochs — Legacy `forms.takoform.com/v1alpha1` with its 34
published package identities, and the provider-v2 `forms.takoform.com/v1alpha2`
epoch with its nine retained candidates — were **withdrawn** while Takoform is
pre-Stable ([decision
0042](spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.md)). Their
identities are recorded as retired in
[`release/published-document-lanes.json`](release/published-document-lanes.json)
and [`release/public-schema-identities.json`](release/public-schema-identities.json)
so no address can quietly come back meaning something else, and their bytes
stay in this repository's git history and release tags, where the
[`formpackage`](formpackage/) verifier still verifies them.

Provider SemVer is independent from both. One provider address exists,
`registry.terraform.io/tako0614/takoform`, used identically by Terraform and
OpenTofu, and three published releases on it are immutable Registry history:

- Provider `v2.1.1` is the current published, Registry-verified provider. It
  carries the Beta Host API lane and the exact 15 Experimental Edge Platform
  Forms, plus the nine since-withdrawn v1alpha2 resources it was published
  with. Its 15 embedded Beta FormRefs and definition/package digests are locked
  in [`release/provider-form-identities.json`](release/provider-form-identities.json).
- Provider `v2.0.0` is the published compatibility predecessor for the
  withdrawn v1alpha2 epoch. Existing state can keep it exactly pinned.
- Provider `v1.0.3` is published Legacy for the withdrawn v1alpha1 epoch, for
  recovery and migration only.

The provider **built from this repository** exposes only the 15 Family
resources. Because the nine v1alpha2 resources are gone from it, the next
published release MUST be a major, `3.0.0`; what existing users of the nine do
is written down in [release/migrations/v2-to-v3.md](release/migrations/v2-to-v3.md).
The stable `v2.1.1` release target's `release/version.json` descriptor
intentionally remains `candidate-only` metadata after owner publication; that
value is not live availability state and the owner's release flow assigns the
next version when a release is actually cut. Registry readback proves the
immutable `v2.1.1` publication. The open obligations in
[`spec/publication-blockers.json`](spec/publication-blockers.json) still block
Form Package or public-service publication; provider publication does not grant
Form maturity, Host Support, activation, or Cloud availability. The scoped
policy is [`spec/publication-freeze.md`](spec/publication-freeze.md).

## Using the published provider

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 2.1.1"
    }
  }
}

provider "takoform" {
  endpoint = "https://forms.example.com"
  space    = "prod"
}
```

`endpoint`, `space`, and bearer `token` may instead come from
`TAKOFORM_ENDPOINT`, `TAKOFORM_SPACE`, and `TAKOFORM_TOKEN`.

Host transport is the Beta lane only: discovery at
`/.well-known/takoform/v1beta1` and the API base
`/apis/forms.takoform.com/v1beta1`, with UID/generation/revision identity,
long-running operations, and content-addressed artifact upload
([`spec/host-api/v1beta1.md`](spec/host-api/v1beta1.md)). One discovery
response never points two client generations at an ambiguous API base.

### Run it against a host on your machine

`forms.example.com` above is a placeholder, and a provider with nowhere to point
is not something anyone can try. This repository carries a host you can start:

```console
go run ./cmd/reference-host --addr 127.0.0.1:8080
```

Then, in a directory of your own, with the configuration above pointing at
`http://127.0.0.1:8080` and `token = "reference-primary-token"`:

```console
tofu init
tofu apply
tofu destroy
```

That creates a real `ModuleWorker` under the exact
`edge.forms.takoform.com/v1beta1` FormRef, reports the conditions the host
computes for it — a worker with no deployment is `Ready=False` / `Provisioning`,
and says so — and destroys it. It is the same host `bun run check` drives an
OpenTofu and a Terraform CLI against on every run
(`cmd/worker-authoring-conformance`), so the walk above cannot quietly stop
working.

**What that host is not.** It stores desired state and serves no application
traffic: the worker you create has no isolate, a `WorkerEndpoint`'s address
answers nothing, and a queue delivers no message. This lane drives desired state
and never moves a byte of application data
([`spec/host-api/v1beta1.md`](spec/host-api/v1beta1.md)). It also implements the
runner-only conformance probe headers and its credentials are three constants
compiled into this repository, so keep it on loopback. It is a host to learn and
develop against; measuring a real one is what
[`spec/publication-blockers.json`](spec/publication-blockers.json) is still open
about.

## What a host owns

Using the provider requires a compatible host that independently advertises
exact support and availability; this repository does not own or assert any
hosted service's live catalog. Host evidence, when recorded, does not grant
Takoform maturity, publication, or approval authority.

The provider contains no target-pool, backend, credential, price, quota,
billing, or activation resources. A host owns exact Form support, activation,
placement, credentials, and commercial policy. Provider state retains only the
exact FormRef/package identity, portable desired fields, generation fences,
and schema-validated public output.

The generated inventories:

- [Form inventory](forms/README.md)
- [Provider reference](docs/index.md)
- [Candidate set](forms/candidates/edge/v1beta1/candidate-set.json)

## Development

```console
go test ./...
go run ./cmd/worker-authoring-conformance matrix --opentofu tofu --terraform terraform
go run ./cmd/standard-form-conformance verify
bun run check
```

No check, task, or candidate descriptor publishes a provider, Form Package, or
website. Publication and deployment remain separate explicit operations.

## License

MIT
