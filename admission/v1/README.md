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

Passing the package gate proves publication only.
`standard-admission-set.json` now retains the exact signed host, provider, and
admission closure plus the direct two-Registry provider readback. Its Form
definition/package version remains `1.0.1`; the independent immutable admission
activation stream advances to `forms/admissions/v1.0.3` without republishing or
re-signing those exact package/evidence subjects. `admission-closure-check`
proves only the candidate's offline five-role closure. Public `release-check`
remains fail-closed unless the signed tag, completed controller promotion run,
immutable activation Release, exact eight-asset inventory, and retained
controller readback all exist and read back exactly. No revocation statement
or checkpoint is fabricated here.

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

Report promotion is retain-first. A separate reviewed commit stores each exact
23-file signed host/provider candidate under `admission/v1/candidates/`. The
evidence workflow accepts an immutable snapshot commit/tree, both candidate Git
trees, their source commits and workflow run IDs, and exact manifest digests.
It requires the historical Takoform execution commits and snapshot to be
ancestors of protected current main, revalidates the current package closure,
and verifies every retained bundle against the pinned offline trusted root.
It never downloads an expiring Actions artifact and requires no cross-repository
token or secret.

Refresh `trust/trusted-root.json` only as a reviewed rotation from Sigstore TUF:

```console
go run ./cmd/sigstore-trust-root refresh
```

After refresh, update the exact digest pin and rerun the offline publication
gate. Never discover or rotate trust during admission verification.
