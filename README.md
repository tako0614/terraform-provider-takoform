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

## What is published and what is preview

Two tiers, and everything below says which one it belongs to.

| Tier | What it is | How you get it |
| --- | --- | --- |
| **Current published** | Provider `v2.0.0` and the retained nine `forms.takoform.com/v1alpha2` resources | installed from the Terraform Registry |
| **Edge preview** | Provider `v2.1` source candidate and the unpublished `edge.forms.takoform.com/v1alpha1` family; no public host yet; publication blockers open | built from this repository's source only |

The Edge preview tier is source, not a release. Its publication blockers are
data in [`spec/publication-blockers.json`](spec/publication-blockers.json) and
the freeze they enforce is [`spec/publication-freeze.md`](spec/publication-freeze.md).
While any is open, no family Form, Interface, Binding, or provider release
carrying them may be published.

The specification has three explicit lanes:

- `forms.takoform.com/v1alpha1` is the frozen **Legacy Epoch**. Its 34 published
  Form Package identities are immutable Legacy evidence. There is no current
  central approval or admission derived from that history.
- `forms.takoform.com/v1alpha2` is the retained **provider-v2 epoch**. Its nine
  Proposal-derived, unpublished `0.1.0` candidates (`EdgeWorker`,
  `RelationalDatabase`, `ObjectBucket`, `KeyValueStore`, `Queue`, `Schedule`,
  `ContainerService`, `StatefulEntity`, `VectorIndex`) remain reproducible
  provider-v2 preview source. They are superseded for new design work
  ([decision 0013](spec/decisions/0013-v1alpha3-lane-ships-in-provider-v2-1.md)).
- Current design work happens in namespaced **Form Families**
  ([spec/form-families.md](spec/form-families.md)). The first family is the
  Edge Platform Family, `edge.forms.takoform.com/v1alpha1`: `ModuleWorker`,
  `WorkerBundle`, `WorkerVersion`, `WorkerDeployment`, `WorkerCustomDomain`,
  `WorkerEndpoint`, `WorkerCronTrigger`, `EdgeKVNamespace`, `ObjectBucket`,
  `SQLiteDatabase`, `AtLeastOnceQueue`, and `QueueConsumer`, with exact
  Interface and typed Binding contracts under
  `interfaces.takoform.com/v1alpha1` and `bindings.takoform.com/v1alpha1`.
  This is the Edge preview tier: source only, and unpublished.

Retained v1alpha2 package envelopes use `packages.forms.takoform.com/v1alpha3`
because a published v1alpha2 package schema already refers to Legacy FormRefs;
family packages use `packages.forms.takoform.com/v1alpha4`. Published
v1alpha1/v1alpha2/v1alpha3 package bytes remain unchanged.

Host transport follows the same explicit split. Provider v1 uses the frozen
`/.well-known/takoform` and `/apis/forms.takoform.com/v1alpha1` lane. Provider
v2 uses `/.well-known/takoform/v1alpha2` and
`/apis/forms.takoform.com/v1alpha2`. The current v1alpha3 lane (carried by provider v2.1, the next minor) uses
`/.well-known/takoform/v1alpha3` and `/apis/forms.takoform.com/v1alpha3` with
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
  retained nine Form candidates, which are superseded for new design work. Its
  `release/version.json` value `publicationStatus: candidate-only` remains
  source descriptor metadata; it does not describe live Registry state.
- The Edge Platform Family and its Host API `v1alpha3` lane ride provider
  `v2.1.0`, an unpublished source candidate that must be built from source
  ([decision 0013](spec/decisions/0013-v1alpha3-lane-ships-in-provider-v2-1.md)).
  No family Form is published, Experimental, or Stable.
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

## Retained v1alpha2 Forms and Cloud

The retained provider-v2 inventory is generated from the exact nine-candidate
manifest:

- [Form inventory](forms/README.md)
- [Provider reference](docs/index.md)
- [Candidate packages](forms/candidates/v1alpha2/)
- [Lifecycle authority](forms/lifecycle.json)
- [Epoch decision](spec/decisions/0006-v1alpha2-restarts-form-lines.md)

Takosumi Cloud is the first production-shaped host and supplies implementation
feedback for all nine resources. That proves workload relevance and gives the
Forms a concrete host; it does not grant Takoform maturity, publication, or
approval authority. `VerifiedDomain` and `AIGateway` remain Cloud services and
are intentionally not portable Forms.

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
