# Form Package release boundary

The repository has two keyless, data-only release lanes. They do not share the
Terraform provider's GPG key, `v*` tag namespace, or `provider-release`
Environment.

## Package source and tag

A release source is one already-valid closed package directory:

```text
forms/releases/<form-slug>/<packageVersion>/
  package-index.json
  <exact payload closure listed by the index>
```

The tag is `forms/<form-slug>/v<packageVersion>`. The builder requires exact
SemVer equality, derives the slug from the FormRef kind, verifies the complete
package, and requires the tag to point at a clean `HEAD`. Local tests may use
the explicit `--allow-untagged-candidate` switch; its manifest remains
`publicationReady=false`.

Tag protection selects an immutable source commit; it is not the package's
cryptographic signature. Trust in the package bytes comes from verifying the
canonical-index Sigstore bundle against the exact workflow OIDC identity.

The release contains:

- the newline-free RFC 8785 canonical `package-index.json`, which is the exact
  Cosign signed subject and semantic package identity;
- a deterministic `.tar.gz` transport whose root index has those same bytes
  and whose payload bytes match the index closure;
- an SPDX 2.3 data-artifact SBOM;
- an in-toto Statement v1 with SLSA Provenance v1;
- a Sigstore v0.3 bundle containing the ephemeral certificate, signature, and
  transparency-log inclusion evidence;
- a release manifest and `SHA256SUMS` for the exact final asset inventory; and
- GitHub build-provenance and SBOM attestations for the final inventory.

`.github/workflows/form-package-release.yml` runs only for
`forms/*/v*` excluding `forms/revocations/v*`. It uses the protected
`form-package-release` Environment, commit-pinned Actions, and Cosign v3. It
refuses an existing release, signs and immediately verifies the canonical
index against the exact workflow identity, creates a draft, compares the
remote and local inventories, attests the assets, then publishes the draft.
When repository release immutability is enabled, publication locks the tag and
assets.

## Verification

For a tag such as `forms/object-bucket/v1.0.0`, verify the retained canonical
index and bundle with:

```console
cosign verify-blob takoform-form-object-bucket_1.0.0_package-index.json \
  --bundle takoform-form-object-bucket_1.0.0_package-index.sigstore.json \
  --certificate-identity \
  'https://github.com/tako0614/terraform-provider-takoform/.github/workflows/form-package-release.yml@refs/tags/forms/object-bucket/v1.0.0' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com'
sha256sum --check --strict SHA256SUMS
```

The bundle carries the signature, certificate, and transparency inclusion
proof. Air-gapped verification additionally requires a retained,
operator-managed Sigstore trusted root from the Public Good Instance; the
distribution endpoint is never a trust root.

## Append-only security revocation

One source file at `forms/revocations/<statementVersion>.json` records a
security decision for one exact package digest and FormRef. It fixes the
effects to block new create/update and activation while retaining referenced
bytes for observe/delete. Deprecation is not a revocation.

`.github/workflows/form-package-revocation.yml` binds the statement to
`forms/revocations/v<statementVersion>`, canonicalizes and keyless-signs it,
adds SLSA and GitHub provenance, verifies an exact draft inventory, and
publishes it through the same protected Environment. CI permits adding a new
statement path but rejects edits, renames, and deletion of existing statement
paths.

## Repository configuration

The source tree and repository settings are both part of the trust boundary:

- active tag rulesets target `refs/tags/forms/*/v*`, restrict creation, and
  prevent deletion and non-fast-forward updates;
- `form-package-release` has required reviewers and only permits
  `forms/*/v*` deployment tags;
- release immutability is enabled before the first publication; and
- a real tag/release is created only after maintainer authorization.

This repository currently has no live Form Package or revocation release.
Host fetch/install, publisher-policy verification, activation, and revocation
enforcement require separate consumer/operator evidence.
