# Retained production admission inputs

This directory separates live package publication proof from portable-standard
admission authority.

`published-package-set.json` is the source-reviewed snapshot of the ten live
`1.0.0` GitHub Releases. Each entry binds the exact candidate FormRef/package
digest to its immutable Git tag and commit, protected-main tooling commit,
GitHub Release ID and publication time, release-manifest digest, `SHA256SUMS`
digest, and retained seven-asset inventory under `releases/`. The set itself
pins `trust/published-package-trust.json` by exact byte digest.

The trust document pins the TUF-authenticated Sigstore Public Good Instance
TrustedRoot snapshot and two currently approved, mutually distinct workflow
policies:

- `package-index`: protected `.github/workflows/form-package-release.yml`;
- `registry-readback`: protected
  `.github/workflows/standard-admission-release.yml`.

The second policy is retained for the next stage; no Registry readback subject
or bundle exists yet. The `admission-evidence`, `host-report`, and
`provider-report` publisher identities remain explicitly unsettled and no
policy is synthesized for them.

Run the offline publication gate with:

```console
go run ./cmd/standard-form-conformance published-package-check
```

It verifies candidate closure, all release semantics, archives, SPDX, SLSA,
checksum closure, Git refs, TUF-root pin, Fulcio identity, Rekor inclusion proof
and integrated time, and certificate-transparency SCT without a network lookup.
`go run ./cmd/published-package-set snapshot` is a maintainer-only online
readback tool: it requires every GitHub Release to report `immutable=true` and
requires the remote names, sizes, and SHA-256 digests to equal the retained
seven files before replacing the snapshot.

Passing this gate proves publication only. `forms/standard-package-set.json`
therefore remains `structural-candidate` / `external-required`, and
`standard-admission-set.json` is intentionally absent. The final
`release-check` remains fail-closed until the three unsettled role authorities,
signed host/provider/admission subjects, direct two-Registry provider readback,
an immutable `forms/admissions/v*` activation release, and real live revocation
chain proof exist. No revocation statement or checkpoint is fabricated here.

The provider lifecycle runner can generate unsigned per-kind
`takoform.standard-runner-report@v1` subjects from these exact retained
archives:

```console
go run ./cmd/provider-lifecycle-conformance provider-reports \
  --cli terraform --output-dir /tmp/takoform-provider-reports
```

The command refuses an output path below this `admission/` tree. Generated
subjects are evidence candidates only; they are not retained here, signed,
published, or accepted by `release-check`.

Refresh `trust/trusted-root.json` only as a reviewed rotation from Sigstore TUF:

```console
go run ./cmd/sigstore-trust-root refresh
```

After refresh, update the exact digest pin and rerun the offline publication
gate. Never discover or rotate trust during admission verification.
