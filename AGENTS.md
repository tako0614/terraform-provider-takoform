# AGENTS.md

This repository owns the standalone Takoform Service Form specification and the form-only Terraform/OpenTofu provider.

## Public identities

- Source repository: `github.com/tako0614/terraform-provider-takoform`
- Provider: `registry.terraform.io/tako0614/takoform`
- API group: `forms.takoform.com/v1alpha1`

The HCP Terraform organization used by maintainers is not a public provider namespace.

## Boundaries

- Keep the provider statically typed and limited to the ten resources listed in `forms/README.md` until a specification and conformance change explicitly expands it.
- Do not add target pools, backend managers, credentials, secrets, prices, billing, quota, accounts, capacity, SLA, or operator-policy resources.
- A host selects and operates concrete implementations. Provider state may contain only sanitized observed evidence and outputs.
- Form definitions and fixtures are data-only. Do not add remotely executable package code or claim package signing until trust roots, canonicalization, custody, rotation, and revocation are specified and implemented.
- Keep the repository independent of Takosumi and all closed Cloud code. A conforming Takosumi host is one consumer of this contract, not its owner.

## Checks

Run `gofmt -w .`, `go vet ./...`, `go test ./...`, and `tofu fmt -check -recursive examples` before review.
