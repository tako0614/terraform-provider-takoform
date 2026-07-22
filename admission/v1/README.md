# Retained production admission inputs

This directory separates live package publication proof from portable-standard
admission authority.

`published-package-set.json` is the source-reviewed snapshot of the ten live
`1.0.1` GitHub Releases. Each entry binds the exact candidate FormRef/package
digest to its immutable per-kind Git tag and shared `standard-forms/v1.0.1`
aggregate release commit,
GitHub Release ID and publication time, release-manifest digest, `SHA256SUMS`
digest, and retained seven-asset inventory under `releases/`. The set itself
pins `trust/published-package-trust.json` by exact byte digest.

The trust document pins the TUF-authenticated Sigstore Public Good Instance
TrustedRoot snapshot and two currently approved, mutually distinct workflow
policies:

- `package-index`: version-bound aggregate
  `.github/workflows/standard-form-package-set-release.yml` at
  `refs/tags/standard-forms/v1.0.1`;
- `registry-readback`: protected
  `.github/workflows/standard-admission-release.yml`.

The full admission trust layer additionally retains
`offline-sigstore-pins.json` and exact, mutually distinct policies for
`admission-evidence`, `host-report`, and `provider-report`. These policies
approve only the corresponding protected workflow identity; they do not turn
an unsigned or missing report into evidence. The canonical Registry readback
subject exists, while its bundle is intentionally produced only by the
protected admission candidate workflow after fresh two-Registry comparison.
The three newly settled identities are exact protected-main workflow subjects:

- `admission-evidence`: Takoform `standard-admission-evidence.yml`;
- `provider-report`: Takoform `standard-provider-report.yml`;
- `host-report`: Takosumi `standard-form-host-report.yml`.

The five-role pin verifier rejects any reused `(issuer, certificate identity)`
pair, policy-byte drift, or unpinned trust-root change.

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
The create-only download mode stages all ten exact seven-asset closures into a
new output root and leaves no partial final root on failure; it never overwrites
retained evidence.

```console
GITHUB_TOKEN=... go run ./cmd/published-package-set download \
  --output-root /tmp/takoform-published-package-snapshot
```

Passing this gate proves publication only. `forms/standard-package-set.json`
therefore remains `structural-candidate` / `external-required`, and
`standard-admission-set.json` is intentionally absent. The final
`release-check` remains fail-closed until signed host/provider/admission
subjects, the authenticated direct two-Registry provider readback,
an immutable `forms/admissions/v*` activation release, and real live revocation
chain proof exist. No revocation statement or checkpoint is fabricated here.

The provider lifecycle runner can generate unsigned per-kind
`takoform.standard-runner-report@v1` subjects from these exact retained
archives:

```console
go run ./cmd/provider-lifecycle-conformance provider-reports \
  --cli tofu \
  --output-dir /tmp/takoform-provider-reports \
  --source-commit "$(git rev-parse HEAD)"
```

The command refuses an output path below this `admission/` tree. The protected
`standard-provider-report.yml` workflow revalidates the exact ten-report,
eleven-file unsigned artifact
in a separate signer job before signing it. Generated or signed subjects remain
evidence candidates until the admission-material workflow authenticates the
complete host/provider closure and a reviewed source change retains the exact
bytes; no workflow in this phase publishes a release.

Refresh `trust/trusted-root.json` only as a reviewed rotation from Sigstore TUF:

```console
go run ./cmd/sigstore-trust-root refresh
```

After refresh, update the exact digest pin and rerun the offline publication
gate. Never discover or rotate trust during admission verification.
