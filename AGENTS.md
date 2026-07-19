# AGENTS.md

This repository owns the standalone Takoform Service Form specification and the form-only Terraform/OpenTofu provider.

## Public identities

- Source repository: `github.com/tako0614/terraform-provider-takoform`
- Provider: `registry.terraform.io/tako0614/takoform`
- API group: `forms.takoform.com/v1alpha1`

The HCP Terraform organization used by maintainers is not a public provider namespace.

## Boundaries

- Keep the provider statically typed and limited to the ten resources listed in `forms/README.md` until a specification and conformance change explicitly expands it.
- The only data source is the read-only `takoform_interface`. Descriptor identity is `(name, version)` and runtime selection also uses the space-scoped portable Resource `{kind,name}`; it grants nothing. Never add a host Interface id, declaration resource, binding/permission/token attributes, or another write path.
- Do not add target pools, backend managers, credentials, secrets, prices, billing, quota, accounts, capacity, SLA, or operator-policy resources.
- A host selects and operates concrete implementations. Provider state may contain only sanitized observed evidence and outputs.
- Form definitions and fixtures are data-only. Do not add remotely executable package code. The protected keyless release and revocation delivery lanes are implemented, but do not claim that a package was published, mirrored, installed, or revoked until the corresponding signed live evidence exists.
- Keep the repository independent of Takosumi and all closed Cloud code. A conforming Takosumi host is one consumer of this contract, not its owner.

## Checks

Run `gofmt -w .`, `go vet ./...`, `go test ./...`, `go run ./cmd/form-package conformance`, `go run ./cmd/standard-form-conformance published-package-check`, and `tofu fmt -check -recursive examples` before review. Release changes must also build deterministic provider and Form Package candidate evidence without creating a tag or release.
