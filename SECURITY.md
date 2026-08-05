# Security Policy

Takoform is an **Experimental project**. Provider `v1.0.3` is the published
Legacy client; provider `v2.0.0` is an unpublished source candidate. The 34
published Form Package identities are immutable **Legacy** evidence, not a
current central approval or admission set.

## Reporting a vulnerability

Use GitHub private vulnerability reporting for `tako0614/terraform-provider-takoform`. Do not disclose a vulnerability, bearer token, endpoint credential, state file, or customer output in a public issue.

Include the affected provider version, a minimal reproduction with all secrets removed, and the expected impact. Maintainers will acknowledge the report and coordinate remediation and disclosure through the private report.

## Supported versions

The current `main` branch receives security fixes while provider `v2.0.0` is
prepared as the current-epoch client. Published provider `v1.0.3` remains the
security-maintained Legacy line for retained state and explicit migration.
Historical release bytes remain available for verification and migration;
support never means replacing them.

`release/version.json` describes the unpublished `v2.0.0` source candidate;
it is not live availability state. The signed immutable release, pinned tag
identity, and canonical Registry listing establish that `v1.0.3` is the last
published provider.

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

Takoform is an Experimental specification project. The 34 published Form
Package identities are immutable Legacy evidence. There is no current central
Takoform approval or admission. The last published admission identity is
`forms/admissions/v1.0.7`; `forms/admissions/v1.0.5` remains permanently
reserved as abandoned. Report mutation of the historical identity
ledger, any pinned tag object or retained tree, or any published package
inventory. Historical bytes remain available for verification and migration;
their old `standard` or `portable-standard` labels do not confer current
maturity, support, or activation.
