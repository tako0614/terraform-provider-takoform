# Contributing

Contributions are welcome. Open an issue before changing the public Service Form contract or adding a resource kind.

## Local checks

Use Go 1.25 or later and run:

```console
gofmt -w .
go vet ./...
go test ./...
go run ./cmd/conformance verify
go run ./cmd/form-package conformance
go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
go run github.com/google/go-licenses@v1.6.0 check ./... \
  --allowed_licenses=Apache-2.0,BSD-2-Clause,BSD-3-Clause,ISC,MIT,MPL-2.0
```

A change is ready for review only when formatting, vet, and tests pass and any schema change includes documentation and an example.

## Provider boundary

Keep this provider thin and host-neutral. It may expose only the ten documented typed Service Forms unless the public specification and conformance suite are updated first. Do not add target-pool, backend, credential, secret, pricing, billing, quota, account, or operator-policy resources. Never log bearer tokens or returned secret material.

## Release changes

The release workflow is intentionally limited to signed `v*` tags whose exact
version and signer match `release/version.json`. Candidate tooling cannot
publish. Changes to the signing fingerprint, public key, Registry asset naming,
platform matrix, or GitHub release permissions require maintainer security
review and a rotation plan. Never test release changes by overwriting an
existing version; use a new semver prerelease.

Form Package release sources live at
`forms/releases/<release-id>/<packageVersion>/`, where the release ID is the
reversible base32 encoding defined in `release/form-packages.md`, and use the
disjoint `forms/<release-id>/v<packageVersion>` tag namespace. Security
revocation statements and cumulative checkpoints live under
`forms/revocations/`, are append-only, and use
`forms/revocations/v<statementVersion>`. Both workflows dispatch from protected
main, are keyless, and
must not reference provider GPG secrets. Test the builders only with
`--allow-untagged-candidate`; never create a real tag or GitHub Release to test
a pull request.
