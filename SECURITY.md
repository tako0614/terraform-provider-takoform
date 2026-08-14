# Security Policy

## Reporting a vulnerability

Use GitHub private vulnerability reporting for `tako0614/terraform-provider-takoform`. Do not disclose a vulnerability, bearer token, endpoint credential, state file, or customer output in a public issue.

Include the affected provider version, a minimal reproduction with all secrets removed, and the expected impact. Maintainers will acknowledge the report and coordinate remediation and disclosure through the private report.

## Supported versions

The maintained `maintenance/v1` branch and published provider `v1.0.3` receive
security fixes for the provider-v1 line. The pre-v1 provider line is
unsupported unless a security advisory explicitly says otherwise. Historical
release bytes remain available for verification and migration; support never
means replacing them.

`release/version.json` retains `publicationStatus: candidate-only` as
release-descriptor metadata and currently records unissued candidate
`v1.0.4`. It is not live availability state. The append-only identity ledger,
signed release, and authenticated canonical Registry readback establish that
`v1.0.3` is published; the ledger has no `v1.0.4` assignment.

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

Published Form Package indexes and cumulative revocation checkpoints must be
verified against the attached Sigstore v0.3 bundle, the exact GitHub Actions
workflow identity, and `https://token.actions.githubusercontent.com`. Report a
missing transparency-log proof, a changed release asset, an unexpected
workflow identity, a checkpoint rollback/omission/prefix rewrite, or a
revocation that does not retain package bytes for observe/delete as a
supply-chain vulnerability.

All 34 current Form Packages are published as immutable signed releases. The
protected `forms/admissions/v1.0.7` closure admits exactly 10 of them as
`portable-standard`; the remaining 24 are published but not admitted. Report
any mutation or replacement of that admission identity, retained evidence, or
package inventory. Retired package releases remain live and immutable, and
their exact retained release closures continue to pass offline package-index
verification.
