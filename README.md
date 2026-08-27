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

| Identity | Current identity | Meaning |
| --- | --- | --- |
| API/Core release SemVer | `1.0.1` | public Core/API checkpoint on [the Core v1.0.1 release](https://github.com/tako0614/takoform/releases/tag/v1.0.1), using the forms.takoform.com/v1 wire/discovery lane; compatible 1.y.0 checkpoints remain on /v1 |
| Form definitionVersion (active publisher) | `1 family / 16 candidate Forms` | active Edge source set edge.forms.takoform.com, independently owned by [takoform-forms](https://github.com/tako0614/takoform-forms); package artifacts remain unpublished |
| Host API wire/discovery lane | `forms.takoform.com/v1` | protocol path used by API/Core 1.x checkpoints; this path is not a third domain axis, and Host implementation/support/deployment remains host-owned |
| Provider compatibility mapping | `3.0.0 (retained)` | 8 families / 31 typed mappings retained by Provider 3; this Provider history is not the active publisher roster |
| Historical Specification receipt | `1.1` | sealed exact source receipt; not API release 1.1 or 1.1.0, no /v1.1, and no ongoing Specification stream |
| Form Package envelope | `packages.forms.takoform.com/v1alpha5` | package artifacts are unpublished |
| Provider distribution | `3.0.0` | Registry-published Provider 3 retaining the typed compatibility mapping; not the active publisher roster or Specification authority |

Only API/Core release SemVer and per-Form definitionVersion are domain
version axes. The active publisher is the standalone Edge candidate/source
edge.forms.takoform.com (16 Forms); its package artifacts are unpublished.
Provider 3's 31 typed mappings are retained compatibility history, not a
current publisher roster. The historical Specification 1.1 receipt is sealed
and separate: it is not API release 1.1 or 1.1.0, does not create `/v1.1`,
and is not an ongoing Specification stream. Form/package publication and
Provider release remain independent artifact identities.
This table is generated from repository bytes
by `bun run sync:current-generation`; the numbered release ledger derives
historical Specification receipt state without changing any API/Core, Form,
package, or Provider identity.

<!-- current-generation:end -->

## API/Core 1.x, compatibility, and retained Provider history

The current public API/Core checkpoint is **`v1.0.1`**, published by the
[Takoform Core release](https://github.com/tako0614/takoform/releases/tag/v1.0.1)
on the existing `forms.takoform.com/v1` wire and discovery lane. API SemVer is
a human-readable compatibility checkpoint; compatible `1.x` checkpoints remain
on `/v1`. Host implementation, deployment, support, and adoption are separate
host-owned facts. The historical Specification 1.1 is a sealed exact source
receipt, not an API release or current version axis, and it does not create
`/v1.1`.

The active standalone publisher is
[`takoform-forms`](https://github.com/tako0614/takoform-forms). Its
`edge.forms.takoform.com` source set contains 16 Experimental candidate Forms;
package artifacts are unpublished. Form definition and package releases are
independent of Provider releases. The Provider repository retains 31 typed
mappings across eight families for Provider 3 compatibility history; that
projection is not the current publisher roster
([decision 0055](spec/decisions/0055-specification-release-needs-only-normative-source.md)).

The separately generated compatibility report is evidence only. It binds
current Form/Package, Host lifecycle, family/Host support, Interface/Binding/
transport/service, and trust/revocation/lifecycle/version/release identities to
raw source bytes and owning ledgers, but it does not add a domain version axis,
publish a Form, or create a wire lane. The `/v1` path is the API/Core 1.x
wire/discovery lane; identity `1.0` in the historical Specification ledger was
never published, is withdrawn, and is never reused.

The W09 release has four explicit boundaries: C1 freezes the normative tree
and executable tooling while publication-evidence fields stay `null`; C2 is an
evidence-only source-snapshot change; C3 is the authoritative append-only
publication receipt; and C4 is its direct generated-output-only child that
refreshes the released public state. The compatibility report is checked
separately and is never a C2/C3 asset or prerequisite.

W09's historical owner was the existing
`https://github.com/tako0614/terraform-provider-takoform.git` repository. A
future W10 Core owner may define future API/Core checkpoints through an
explicit decision; it must never reissue, rewrite, or retag the sealed
Specification 1.1 receipt.

The retained Provider 3 mapping contains 31 exact Experimental `0.x` Forms in
eight versionless families. The active Edge candidate set contains these 16
Forms and no
`ObjectBucket`: `ModuleWorker`, `WorkerBundle`, `StaticAssetBundle`,
`WorkerVersion`, `WorkerDeployment`, `WorkerCustomDomain`, `WorkerEndpoint`,
`WorkerCronTrigger`, `EdgeKVNamespace`, `SQLiteDatabase`,
`SQLiteMigrationSet`, `SQLiteMigrationApplication`, `AtLeastOnceQueue`,
`QueueConsumer`, `DurableWorkflow`, and `ActorNamespace`. The retained mapping's
other families are Container, Function, Pull Queue, Schedule, Table, Topic, and
Vector. Releasing the historical Specification receipt does not silently mint
stable Form identities; a future stable Form starts only by an explicit
per-Form decision.

Provider SemVer is independent of both domain axes. Provider `v2.1.1` remains immutable,
Registry-readback history for the retained `v1beta1` Host/family identities,
and Provider `v2.0.0` and `v1.0.3` remain earlier published history. The
official Provider 3 implementation retains typed mappings for the Provider
snapshot, but it is non-normative and cannot block API/Core or alter the sealed
Specification receipt. Provider 3 remains typed; it does not add an opaque
generic JSON resource. A Provider release only changes the typed adapter and
its compatibility surface; it does not publish a Form Package.

API patches are defect-only and may be batched; API minors are at most monthly
and may be skipped; a major is normally at most annual and requires a concrete
incompatibility and migration plan. Every public release passes prepublication
review/gates and is read-only after publication. Form definition/package
releases follow their publisher's independent cadence.

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
