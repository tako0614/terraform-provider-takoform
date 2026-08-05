# Takoform

Takoform is an **Experimental specification and tooling project** for
host-neutral desired-state contracts. It is not an industry standard, a cloud
catalog, or a promise that a workload can move between backends without
migration.

The specification has two explicit epochs:

- `forms.takoform.com/v1alpha1` is the frozen **Legacy Epoch**. Its 34 published
  Form Package identities are immutable Legacy evidence. There is no current
  central approval or admission derived from that history.
- `forms.takoform.com/v1alpha2` is the current **Specification Epoch**. It starts
  with nine Proposal-derived, unpublished `0.1.0` candidates. Each requires an
  explicit lifecycle transition before it can become Experimental. They are backed by real
  Takosumi Cloud resource implementations: `EdgeWorker`,
  `RelationalDatabase`, `ObjectBucket`, `KeyValueStore`, `Queue`, `Schedule`,
  `ContainerService`, `StatefulEntity`, and `VectorIndex`.

Current package envelopes use `packages.forms.takoform.com/v1alpha3` because a
published v1alpha2 package schema already refers to Legacy FormRefs. Published
v1alpha1/v1alpha2 package bytes remain unchanged.

Host transport follows the same explicit split. Provider v1 uses the frozen
`/.well-known/takoform` and `/apis/forms.takoform.com/v1alpha1` lane. Provider
v2 uses `/.well-known/takoform/v1alpha2` and
`/apis/forms.takoform.com/v1alpha2`; one discovery response never points both
client generations at an ambiguous API base.

## Provider lines

There is one provider address:
`registry.terraform.io/tako0614/takoform`. Provider SemVer is independent from
Form definition versions and the Form API epoch.

Terraform and OpenTofu use one provider source and state identity. Both current
v2 and Legacy v1 are published at the canonical Terraform Registry address;
installation is verified through CLI rather than inferred from repository
files. `release/version.json` is release-descriptor metadata, while the
signed release, immutable tag identity, and Registry listing establish publication.

- Provider `v1.0.3` is published and Registry-verified. It is the Legacy
  `v1alpha1` client. Existing Legacy state must pin provider v1.
- Provider `v2.0.0` is published and Registry-verified as the current
  `v1alpha2` client. It exposes exactly the nine current Form candidates. Its
  `release/version.json` value `publicationStatus: candidate-only` remains
  source descriptor metadata; it does not describe live Registry state.
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

## Current Forms and Cloud

The current inventory is generated from the exact nine-candidate manifest:

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
