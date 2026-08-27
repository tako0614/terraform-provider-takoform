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
| Specification | `1.1` | released; one exact committed normative source snapshot is release authority |
| Host API candidate | `forms.takoform.com/v1` | unpublished-candidate; separate protocol identity |
| Current Form corpus | `forms/candidates/current-family-index.json` | 8 versionless families, 31 exact `0.x` experimental Forms |
| Form Package envelope | `packages.forms.takoform.com/v1alpha5` | package artifacts are unpublished |
| Provider distribution | `3.0.0` | current Registry-published typed reference implementation; not Specification authority |

These identities are independent. A Specification 1.1 release does not
publish or promote the separate Host API v1 candidate, relabel any current
Form as `1.0.0`, publish a Form Package, or release the non-normative Provider.
This table is generated from repository bytes
by `bun run sync:current-generation`; the numbered release ledger derives
the Specification row as `candidate-open` or `released` without changing any
Host API, Form, package, or Provider identity.

<!-- current-generation:end -->

## Specification 1.1, compatibility, and retained Provider history

Takoform Specification 1.1 is the current Specification. Its release status is
derived from the append-only `release/specification-releases.json` ledger and
reflected in the generated `takoform-site.json` status document. Its normative
source references the literal Host API v1 contract. Host API v1 is a separate,
unpublished protocol candidate; publishing Specification 1.1 does not publish
or promote it. The Specification's sole release prerequisite is an exact
committed snapshot of the normative `spec/` tree. Candidate Forms, reference
conformance, Providers, external Hosts, products, deployments, signers, and
operators are implementation or adoption evidence, not Specification authority
([decision 0055](spec/decisions/0055-specification-release-needs-only-normative-source.md)).

The separately generated [five-class compatibility report](release/specification-compatibility.json)
binds current Form/Package, Host lifecycle, family/Host support,
Interface/Binding/transport/service, and trust/revocation/lifecycle/version/
release identities to raw source bytes and owning ledgers. It byte-pins the
literal Host API v1 candidate and records candidates without publishing them.
It is compatibility evidence only, not Specification release evidence, an
asset, or a prerequisite. Specification 1.1 has no Host API, Form publication,
or Provider effect; it does not mint a `/v1.1` lane, a v2 lane, schema, tag, or
receipt. Identity `1.0` was never published, is withdrawn, and is never reused.

The W09 release has four explicit boundaries: C1 freezes the normative tree
and executable tooling while publication-evidence fields stay `null`; C2 is an
evidence-only source-snapshot change; C3 is the authoritative append-only
publication receipt; and C4 is its direct generated-output-only child that
refreshes the released public state. The compatibility report is checked
separately and is never a C2/C3 asset or prerequisite.

W09's current owner is the existing
`https://github.com/tako0614/terraform-provider-takoform.git` repository. A
future W10 Core owner may write later releases only; it must never reissue,
rewrite, or retag Specification 1.1.

The current corpus contains 31 exact Experimental `0.x` Forms in eight
versionless families. The Edge family contains these 16 Forms and no current
`ObjectBucket`: `ModuleWorker`, `WorkerBundle`, `StaticAssetBundle`,
`WorkerVersion`, `WorkerDeployment`, `WorkerCustomDomain`, `WorkerEndpoint`,
`WorkerCronTrigger`, `EdgeKVNamespace`, `SQLiteDatabase`,
`SQLiteMigrationSet`, `SQLiteMigrationApplication`, `AtLeastOnceQueue`,
`QueueConsumer`, `DurableWorkflow`, and `ActorNamespace`. The other current
families are Container, Function, Pull Queue, Schedule, Table, Topic, and
Vector. Releasing Specification 1.1 does not silently mint Form `1.0.0`
identities; a future stable Form starts at `1.0.0` only by an explicit
per-Form decision.

Provider SemVer is independent. Provider `v2.1.1` remains immutable,
Registry-readback history for the retained `v1beta1` Host/family identities,
and Provider `v2.0.0` and `v1.0.3` remain earlier published history. The
official Provider 3 implementation may reference the exact current `0.x`
Forms, but it is non-normative and cannot block Specification 1.1. Provider 3
remains typed; it does not add an opaque generic JSON resource.

## Using retained Provider 2.1.1 history

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

Provider 2.1.1 speaks its retained Beta lane: discovery at
`/.well-known/takoform/v1beta1` and the API base
`/apis/forms.takoform.com/v1beta1`, with UID/generation/revision identity,
long-running operations, and content-addressed artifact upload
([`spec/host-api/v1beta1.md`](spec/host-api/v1beta1.md)). One discovery
response never points two client generations at an ambiguous API base.

### Run the current reference Host on your machine

`forms.example.com` above is a placeholder. Independently, this repository
carries a reference implementation of the current Host API v1 contract:

```console
go run ./cmd/reference-host --addr 127.0.0.1:8080
```

The frozen reference suite executes through
`go run ./cmd/portable-host-conformance suite --manifest
conformance/takoform-v1/manifest.json`. Provider 2.1.1 compatibility and the
current Host v1 reference are independent surfaces; their presence in one
repository is not a claim that the retained Provider release implements the
new Specification lane.

**What that host is not.** It stores desired state and serves no application
traffic: the worker you create has no isolate, a `WorkerEndpoint`'s address
answers nothing, and a queue delivers no message. This lane drives desired state
and never moves a byte of application data
([`spec/host-api/v1.md`](spec/host-api/v1.md)). It also implements the
runner-only conformance probe headers and its credentials are three constants
compiled into this repository, so keep it on loopback. It is a host to learn and
develop against. Evidence from a real product or production Host is optional
adoption evidence, not Specification release authority.

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
- [Current family index](forms/candidates/current-family-index.json)

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
