# Security Policy

## Reporting a vulnerability

Use GitHub private vulnerability reporting for `tako0614/terraform-provider-takoform`. Do not disclose a vulnerability, bearer token, endpoint credential, state file, or customer output in a public issue.

Include the affected provider version, a minimal reproduction with all secrets removed, and the expected impact. Maintainers will acknowledge the report and coordinate remediation and disclosure through the private report.

## Supported versions

The current `main` branch and the latest published provider line receive
security fixes. A version in `release/version.json` with
`publicationStatus: candidate-only` is not a published release.

When provider `v1.0.1` is published, `v1` becomes the stable provider line and
the latest `v1.x` release receives fixes. The pre-v1 provider line then becomes
unsupported unless a security advisory explicitly says otherwise. Historical
release bytes remain available for verification and migration; support never
means replacing them.

## Provider release trust

Release checksums are signed by the key pinned in `release/version.json` and
`release/keys/`. Report an unexpected signer, unsigned checksum, digest drift,
or replaced GitHub release immediately through private vulnerability reporting.
Key rotation is additive: pin and review the new public key, register it with
the Terraform Registry, and publish only a new semver. Never replace bytes for
an existing version.

Form Packages use the separate keyless publisher identity and revocation rules
in [`spec/trust/`](spec/trust/). A Form Package must never reuse the provider
GPG key or the separately owned Takosumi legacy/admin provider trust root.

Published Form Package indexes and cumulative revocation checkpoints must be verified
against the attached Sigstore v0.3 bundle, the exact GitHub Actions workflow
identity, and `https://token.actions.githubusercontent.com`. Report a missing
transparency-log proof, a changed release asset, an unexpected workflow
identity, a checkpoint rollback/omission/prefix rewrite, or a revocation that
does not retain package bytes for observe/delete as a supply-chain
vulnerability. The retired `1.0.0` and `1.0.1` Form Package releases are live and immutable,
and their exact retained release closures pass offline package-index
verification. No admission activation or revocation release has been published
yet.
