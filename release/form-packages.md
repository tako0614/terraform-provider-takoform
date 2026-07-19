# Form Package release boundary

The repository has two keyless, data-only release lanes. They do not share the
Terraform provider's GPG key, `v*` tag namespace, or `provider-release`
Environment.

## Package source and tag

A release source is one already-valid closed package directory:

```text
forms/releases/<release-id>/<packageVersion>/
  package-index.json
  <exact payload closure listed by the index>
```

The release ID is `k-` plus the lowercase, unpadded base32 encoding of the
exact ASCII FormRef Kind bytes. It is reversible, filesystem-safe, and does not
collapse case-distinct Kinds. The tag is
`forms/<release-id>/v<packageVersion>`. The builder requires exact SemVer
equality, decodes the release ID back to the exact FormRef Kind, verifies the
complete package, and requires the tag to point at a clean `HEAD`. Local tests may use
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
- an RFC 8785 canonical SPDX 2.3 data-artifact SBOM that binds the exact
  FormRef, package digest, package version, index/payload SHA-256 closure, and
  SPDX package verification code; the document `DESCRIBES` the package and
  that package has one deterministic `CONTAINS` relationship for the index
  and every payload file;
- an RFC 8785 canonical in-toto Statement v1 with SLSA Provenance v1 that
  binds the exact index/archive digests, source repository and tag commit,
  the distinct protected-main release-tooling commit, protected workflow, and
  canonicalization mode; its builder ID is versioned by that tooling commit;
- a Sigstore v0.3 bundle containing the ephemeral certificate, signature, and
  transparency-log inclusion evidence;
- a release manifest and `SHA256SUMS` for the exact final asset inventory; and
- GitHub build-provenance and SBOM attestations for the final inventory.

`.github/workflows/form-package-release.yml` is dispatched only from current
protected `main` with exact `tag` and `expected_commit` inputs. It verifies the
tag, equality to the approved commit, and that commit's ancestry from main,
then checks the tagged data into a separate untrusted-source directory. Only
the protected-main release tooling executes. The workflow uses the protected
`form-package-release` Environment, commit-pinned Actions, and Cosign v3. It
passes its exact `GITHUB_SHA` as the tooling commit, distinct from the tagged
package source commit, and both commits are retained in the release manifest,
publisher policy evidence, and SLSA resolved dependencies. It
refuses an existing release, signs and immediately verifies the canonical
index against the exact protected-main workflow identity, creates a draft,
compares remote and local inventories by the exact release ID created by that
run, attests the assets, then publishes that same draft.
When repository release immutability is enabled, publication locks the tag and
assets.

## Verification

For the `ObjectBucket` tag
`forms/k-j5rguzldorbhky3lmv2a/v1.0.0`, verify the retained canonical
index and bundle with:

```console
cosign verify-blob takoform-form-k-j5rguzldorbhky3lmv2a_1.0.0_package-index.json \
  --bundle takoform-form-k-j5rguzldorbhky3lmv2a_1.0.0_package-index.sigstore.json \
  --certificate-identity \
  'https://github.com/tako0614/terraform-provider-takoform/.github/workflows/form-package-release.yml@refs/heads/main' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com'
sha256sum --check --strict SHA256SUMS
```

The bundle carries the signature, certificate, and transparency inclusion
proof. Air-gapped verification additionally requires a retained,
operator-managed Sigstore trusted root from the Public Good Instance; the
distribution endpoint is never a trust root.

Standard-admission readback parses the SBOM and provenance as strict I-JSON,
rejects non-canonical or duplicate-key bytes and unknown/omitted fields, and
recomputes their bindings from the signed package index and retained release
manifest. Asset filenames, media types, and checksums alone are not semantic
release evidence. Form Package release tests additionally validate generated
SBOMs offline against the repository-pinned official SPDX 2.3 JSON Schema from
the SPDX `v2.3` tag.

## Append-only security revocation

One source file at `forms/revocations/<statementVersion>.json` records a
consecutively sequenced security decision for one exact package digest and
FormRef. A matching cumulative
`forms/revocations/checkpoints/<statementVersion>.json` commits every statement
from sequence 1 through the current sequence and the previous canonical
checkpoint digest. It fixes the
effects to block new create/update and activation while retaining referenced
bytes for observe/delete. Deprecation is not a revocation.

`.github/workflows/form-package-revocation.yml` dispatches from protected main,
binds the exact statement/checkpoint source to
`forms/revocations/v<statementVersion>`, verifies the complete cumulative
source chain, and keyless-signs the checkpoint. It adds SLSA and GitHub
provenance, verifies an exact draft inventory, and publishes it through the
same protected Environment. CI permits adding a new statement/checkpoint pair
but rejects edits, renames, and deletion of existing source paths.

After verifying the Sigstore bundle against the exact `@refs/heads/main`
workflow identity, a host starts only at sequence 1 and durably pins the
checkpoint sequence, canonical SHA-256 digest, and cumulative-entry digest. It
accepts only the next sequence with a matching `previousCheckpointDigest` and
unchanged cumulative prefix; rollback, gaps, omissions, prefix rewrites, and
forks fail closed.

## Repository configuration

The source tree and repository settings are both part of the trust boundary:

- active tag rulesets target `refs/tags/forms/*/v*`, restrict creation, and
  prevent deletion and non-fast-forward updates;
- `form-package-release` has required reviewers and only permits deployment
  from the `main` branch;
- release immutability is enabled before the first publication; and
- a real tag/release is created only after maintainer authorization.

All ten `1.0.0` Form Packages have live immutable releases. Their exact
seven-asset inventories and production package-publisher trust inputs are
retained under `admission/v1` and verified offline by
`standard-form-conformance published-package-check`. No admission activation or
revocation release exists. Host fetch/install, the remaining role-specific
publisher authorities, direct Registry readback, portable lifecycle evidence,
activation, and live revocation proof remain separate consumer/operator work.
