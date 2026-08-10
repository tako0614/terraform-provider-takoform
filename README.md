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

## What is published and what is a release candidate

Provider publication, contract channel, Form maturity, and package publication
are independent facts. Everything below names each fact explicitly.

| Tier | What it is | How you get it |
| --- | --- | --- |
| **Current published** | Provider `v2.0.0` and the retained nine `forms.takoform.com/v1alpha2` resources | installed from the Terraform Registry |
| **Beta release candidate** | Stable provider target `v2.1.1`, carrying the Beta Host API `forms.takoform.com/v1beta1` and 15 exact Experimental Forms in `edge.forms.takoform.com/v1beta1`; descriptor status is `candidate-only` until the release owner publishes it | built from this repository's source only |

The provider release candidate is source, not evidence of Registry publication.
Its 15 embedded Beta FormRefs and definition/package digests are already locked
as provider compatibility identities in
[`release/provider-form-identities.json`](release/provider-form-identities.json).
The open obligations in
[`spec/publication-blockers.json`](spec/publication-blockers.json) still block
Form Package or public-service publication and remain later Stable/GA
qualification obligations; they do not turn a stable provider version into a
prerelease. The scoped policy is
[`spec/publication-freeze.md`](spec/publication-freeze.md).

The specification has three explicit lanes:

- `forms.takoform.com/v1alpha1` is the frozen **Legacy Epoch**. Its 34 published
  Form Package identities are immutable Legacy evidence. There is no current
  central approval or admission derived from that history.
- `forms.takoform.com/v1alpha2` is the retained **provider-v2 epoch**. Its nine
  Proposal-derived, unpublished `0.1.0` candidates (`EdgeWorker`,
  `RelationalDatabase`, `ObjectBucket`, `KeyValueStore`, `Queue`, `Schedule`,
  `ContainerService`, `StatefulEntity`, `VectorIndex`) remain reproducible
  provider-v2 preview source. They are superseded for new design work.
- Current design work happens in namespaced **Form Families**
  ([spec/form-families.md](spec/form-families.md)). The first family is the
  Edge Platform Family, `edge.forms.takoform.com/v1beta1`: `ModuleWorker`,
  `WorkerBundle`, `StaticAssetBundle`, `WorkerVersion`, `WorkerDeployment`,
  `WorkerCustomDomain`, `WorkerEndpoint`, `WorkerCronTrigger`,
  `EdgeKVNamespace`, `ObjectBucket`, `SQLiteDatabase`, `SQLiteMigrationSet`,
  `SQLiteMigrationApplication`, `AtLeastOnceQueue`, and `QueueConsumer`, with exact
  Interface and typed Binding contracts under
  `interfaces.takoform.com/v1alpha1` and `bindings.takoform.com/v1alpha1`.
  All 15 definitions are `0.1.0` and Experimental. Their package artifacts
  remain unpublished; Experimental is Form maturity, not package-publication or
  Host-GA status
  ([decision 0035](spec/decisions/0035-beta-contracts-ship-in-stable-provider-v2-1.md)).

Retained v1alpha2 package envelopes use `packages.forms.takoform.com/v1alpha3`
because a published v1alpha2 package schema already refers to Legacy FormRefs;
family packages use `packages.forms.takoform.com/v1alpha4`. Published
v1alpha1/v1alpha2/v1alpha3 package bytes and all v1alpha3 Host API identities
remain unchanged.

Host transport follows the same explicit split. Provider v1 uses the frozen
`/.well-known/takoform` and `/apis/forms.takoform.com/v1alpha1` lane. Provider
v2 uses `/.well-known/takoform/v1alpha2` and
`/apis/forms.takoform.com/v1alpha2`. The retained v1alpha3 identities remain
available as immutable history. The current Beta lane carried by provider
v2.1.1 uses `/.well-known/takoform/v1beta1` and
`/apis/forms.takoform.com/v1beta1` with
UID/generation/revision identity, long-running operations, and
content-addressed artifact upload; one discovery response never points two
client generations at an ambiguous API base.

## Provider lines

There is one provider address:
`registry.terraform.io/tako0614/takoform`. Provider SemVer is independent from
Form definition versions and the Form API epoch.

Terraform and OpenTofu use one provider source and state identity. The
published v2 line and Legacy v1 are both
published at the canonical Terraform Registry address; installation is
verified through CLI rather than inferred
from repository files. `release/version.json` is release-descriptor metadata,
while the signed release, immutable tag identity, and Registry listing
establish publication.

- Provider `v1.0.3` is published and Registry-verified. It is the Legacy
  `v1alpha1` client. Existing Legacy state must pin provider v1.
- Provider `v2.0.0` is published and Registry-verified as the retained
  `v1alpha2` client. The published provider v2.0.0 exposes exactly the
  retained nine Form candidates, which are superseded for new design work.
  Its publication is proved by retained signed Registry-readback evidence,
  not inferred from the current source release descriptor.
- Provider `v2.1.1` is the stable release target for the Beta Host API and the
  exact 15 Experimental Edge Platform Forms. Its descriptor remains
  `candidate-only` until the release owner actually publishes the provider
  ([decision 0035](spec/decisions/0035-beta-contracts-ship-in-stable-provider-v2-1.md)).
  A future `1.0.0` Form line is a new exact identity: refresh never silently
  rewrites Beta state or its codec to that identity.
- Provider v2 fails closed on provider-v1 state. Migration is an explicit
  create/import operation, not an automatic state rewrite.

Published Legacy usage:

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 1.0.3"
    }
  }
}
```

Current published-provider configuration:

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 2.0.0"
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

## Retained v1alpha2 Forms and host responsibility

The retained provider-v2 inventory is generated from the exact nine-candidate
manifest:

- [Form inventory](forms/README.md)
- [Provider reference](docs/index.md)
- [Candidate packages](forms/candidates/v1alpha2/)
- [Lifecycle authority](forms/lifecycle.json)
- [Epoch decision](spec/decisions/0006-v1alpha2-restarts-form-lines.md)

Provider v2.0.0 retains the nine typed resource schemas. Using them requires a
compatible host that independently advertises exact support and availability;
this repository does not own or assert any hosted service's live catalog. Host
evidence, when recorded, does not grant Takoform maturity, publication, or
approval authority.

The provider contains no target-pool, backend, credential, price, quota,
billing, or activation resources. A host owns exact Form support, activation,
placement, credentials, and commercial policy. Provider state retains only the
exact FormRef/package identity, portable desired fields, generation fences,
and schema-validated public output.

## Legacy recovery

The previous 34 provider resources, signed Form Package releases, historical
admission sets, and public provider releases are retained byte-for-byte. They
are compatibility and recovery material, not the current catalog. New Legacy
create should be disabled by default by hosts; read, observe, refresh, delete,
recovery, and explicit migration remain supported lanes.

See [provider v1 to v2 migration](release/migrations/v1-to-v2.md) and the
[retained publication evidence](admission/v4/README.md).

## Development

```console
bun run check:current-form-candidates
go test ./...
go run ./cmd/provider-lifecycle-conformance matrix --opentofu tofu --terraform terraform
go run ./cmd/standard-form-conformance verify
bun run check
```

No check, task, or candidate descriptor publishes a provider, Form Package, or
website. Publication and deployment remain separate explicit operations.

## License

MIT
