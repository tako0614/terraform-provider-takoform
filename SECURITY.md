# Security Policy

## Reporting a vulnerability

Use GitHub private vulnerability reporting for `tako0614/terraform-provider-takoform`. Do not disclose a vulnerability, bearer token, endpoint credential, state file, or customer output in a public issue.

Include the affected provider version, a minimal reproduction with all secrets removed, and the expected impact. Maintainers will acknowledge the report and coordinate remediation and disclosure through the private report.

## Supported versions

Until the first tagged release, only the current `main` branch is supported. After releases begin, the latest stable release will receive security fixes.

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

Published Form Package indexes and revocation statements must be verified
against the attached Sigstore v0.3 bundle, the exact GitHub Actions workflow
identity, and `https://token.actions.githubusercontent.com`. Report a missing
transparency-log proof, a changed release asset, an unexpected workflow
identity, or a revocation statement that does not retain package bytes for
observe/delete as a supply-chain vulnerability. No Form Package or revocation
release has been published yet.
