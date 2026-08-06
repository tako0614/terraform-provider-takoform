# Form Package publication, Legacy retention, and revocation

Form Package publication is independent of the Terraform provider's GPG key,
`v*` tag namespace, and `provider-release` Environment. The current lane is
driven only by Experimental or Stable records in `forms/lifecycle.json` and
uses content-addressed package identities. The pre-reset 34-entry plan is a
closed immutable **Legacy** inventory and cannot authorize a new release.

The responsibilities are:

- publish a lifecycle-authorized current package without inventing a second
  SemVer clock;
- verify and retain the 34 exact historical publication identities;
- publish an append-only security revocation when an exact Legacy package
  requires it;
- preserve read, observe, delete, recovery, and migration for Legacy users;
- introduce every new Form through Proposal and Experimental lifecycle
  evidence without reusing the closed `portable-v1` release plan.

## Current content-addressed publication lane

`bun run deploy -- takoform-form-package-release plan` derives only publishable
Experimental and Stable identities from `forms/lifecycle.json`. With the
current empty lifecycle it prints no commands. For an authorized current Form,
the prepare, publish, and verify phases use the exact derived tag:

```console
bun run deploy -- takoform-form-package-release prepare \
  --tag forms/<release-id>/sha256-<64-lowercase-hex> \
  --expected-commit <40-character-reviewed-commit>

bun run deploy -- takoform-form-package-release publish \
  --tag forms/<release-id>/sha256-<same-64-lowercase-hex> \
  --expected-commit <same-40-character-reviewed-commit> \
  --run-id <candidate-workflow-run-id> \
  --run-attempt <candidate-workflow-run-attempt>

bun run deploy -- takoform-form-package-release verify \
  --tag forms/<release-id>/sha256-<same-64-lowercase-hex> \
  --expected-commit <same-40-character-reviewed-commit>
```

The artifact ID is the verified `packageDigest` with `:` replaced by `-`.
Source path, tag, asset base, release manifest, and deep verification all use
that same value. `FormRef.definitionVersion` remains the Form contract SemVer;
provider versions remain independent.

## Closed Legacy publication lane

The 34 Legacy Forms carried independent versions and were published on separate
tags from separate reviewed sources. This remains an important identity rule,
but it is a historical fact rather than an open release queue.

[`forms/release-plan.json`](../forms/release-plan.json) is the immutable source
plan bound by the historical publication ledger. It must not be regenerated,
passed to the current `plan`/`prepare`/`publish` phases, or used to create tags.
Only the explicit `verify-all` retention path consumes it.

The following prepare/publish/recovery commands describe how the historical
lane worked. They are retained for forensic and recovery review only. Every
canonical tag is already published, so the create-only entrypoint must refuse
them; operators must not treat these examples as current instructions.

Historical prepare phase:

```console
bun run deploy -- takoform-form-package-release prepare \
  --tag forms/<release-id>/v<semver> \
  --expected-commit <40-character-reviewed-commit>
```

The prepare phase stopped after recording one exact workflow run and required a
reviewer. The historical publish phase then consumed only that explicit run and
attempt:

```console
bun run deploy -- takoform-form-package-release publish \
  --tag forms/<release-id>/v<semver> \
  --expected-commit <same-40-character-reviewed-commit> \
  --run-id <candidate-workflow-run-id> \
  --run-attempt <candidate-workflow-run-attempt>
```

The publish phase verified the candidate before creating its tag or Release,
then performed exact immutable readback. The historical verifier invocation was:

```console
bun run deploy -- takoform-form-package-release verify \
  --tag forms/<release-id>/v<semver> \
  --expected-commit <40-character-reviewed-commit>
```

The historical batch mode serialized several independently reviewed candidates
from one protected `main` commit:

```console
bun run deploy -- takoform-form-package-release publish-batch \
  --input /absolute/path/outside/the/repository/form-publish-batch.json
```

The input is a non-empty, canonical recursively key-sorted JSON array. It must
be a regular non-symbolic-link file no larger than 1 MiB, and every object must
contain exactly four string fields:

```json
[
  {
    "expectedCommit": "0123456789abcdef0123456789abcdef01234567",
    "runAttempt": "1",
    "runId": "123456789",
    "tag": "forms/k-ivsgozkxn5zgwzls/v3.0.0"
  }
]
```

Every tag must be one canonical release-plan identity, and tags and exact
workflow run/attempt pairs may not repeat. `publish-batch` validates the whole
operator input before running the complete owner check exactly once. That gate
produces a process-local proof bound to the clean protected-main commit and
tree. The ordinary `publish` flow is then reused synchronously for each entry:
before tag push, Release draft creation, and final draft publication it freshly
fetches protected `origin/main`, requires the same clean HEAD and tree, and
re-runs the selected Form's release-authority path fence. GitHub writers are
never parallel.

A batch was not atomic across independent public identities. It stopped at the
first failure and reported completed tags plus the failed tag. Historical
recovery required authoritative inspection and a new input containing only
identities proven absent.

`publish` was create-only and was never a retry mechanism after a tag-side
partial failure. The narrow tag-only recovery invocation was:

```console
bun run deploy -- takoform-form-package-release recover-tag-only \
  --tag forms/<release-id>/v<semver> \
  --expected-commit <candidate-source-commit> \
  --expected-tag-object <candidate-annotated-tag-object-id> \
  --expected-recovery-commit <current-reviewed-protected-main-commit> \
  --run-id <original-candidate-workflow-run-id> \
  --run-attempt <original-candidate-workflow-run-attempt>
```

This phase re-downloaded and re-verified the original run/attempt, required
source/tooling/recovery ancestry, and proved the release inputs had not changed.
It never pushed, created, moved, or deleted a tag and failed closed on any
identity ambiguity.

The corresponding exact-draft recovery invocation was:

```console
bun run deploy -- takoform-form-package-release recover-draft \
  --tag forms/<release-id>/v<semver> \
  --expected-commit <candidate-source-commit> \
  --expected-tag-object <candidate-annotated-tag-object-id> \
  --expected-recovery-commit <current-reviewed-protected-main-commit> \
  --release-id <retained-github-release-id> \
  --run-id <original-candidate-workflow-run-id> \
  --run-attempt <original-candidate-workflow-run-attempt>
```

`recover-draft` repeated the original candidate, tag, protected-main, stable
recovery-path, and owner-gate proofs. It accepted only one exact draft identity
with the original tag, name, body, upload endpoint, and candidate asset subset.
Every existing asset had to carry a unique positive GitHub ID and the exact
candidate name, state, size, and digest. Any extra, duplicate, unknown, drifted,
public, or competing identity stopped recovery.

After all 34 independent releases existed, the historical lane produced one
deterministic publication record outside the repository:

```console
bun run deploy -- takoform-form-package-release verify-all \
  --output-root /absolute/new/directory/outside/takoform
```

The retained `form-package-publication-set.json` binds every public Release
identity and digest in plan order. It proves exact historical bytes and their
publisher identity only. The later `ga-core-v2` ten-Form classification is an
immutable Legacy assertion, not a current admission decision. Today, Form
lifecycle comes only from `forms/lifecycle.json`; Host Support and activation
are independent host decisions.

## Package source and tag

A current release source is one already-valid closed v1alpha3 package carrying
an exact v1alpha2 FormRef:

```text
forms/releases/<release-id>/sha256-<64-lowercase-hex>/
  package-index.json
  <exact payload closure listed by the index>
```

The release ID is `k-` plus the lowercase, unpadded base32 encoding of the
exact ASCII FormRef Kind bytes. It is reversible, filesystem-safe, and does not
collapse case-distinct Kinds. The tag is
`forms/<release-id>/sha256-<64-lowercase-hex>`. The builder derives that
artifact ID from the verified canonical index digest, decodes the release ID
back to the exact FormRef Kind, verifies the complete package, and requires the
tag to point at a clean `HEAD`. Published Legacy v1alpha1 packages retain their
original `<packageVersion>` directory and `v<packageVersion>` tag; neither may
be reused or renamed. Local tests may use
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
  FormRef, package digest, artifact identity, index/payload SHA-256 closure, and
  SPDX package verification code; the document `DESCRIBES` the package and
  that package has one deterministic `CONTAINS` relationship for the index
  and every payload file;
- an RFC 8785 canonical in-toto Statement v1 with SLSA Provenance v1 that
  binds the exact index/archive digests, source repository and tag commit,
  the distinct protected-main release-tooling commit, protected workflow, and
  canonicalization mode; its builder ID is versioned by that tooling commit;
- a Sigstore v0.3 bundle containing the ephemeral certificate, signature, and
  transparency-log inclusion evidence;
- a release manifest and `SHA256SUMS` for the exact final asset inventory.

`.github/workflows/form-package-release.yml` is dispatched only from current
protected `main` with exact `tag` and `expected_commit` inputs. It verifies the
planned identity, approved commit, and that commit's ancestry from main, then
checks the exact data source into a separate untrusted-source directory. The
workflow creates only an ephemeral local annotated tag object. Only the
protected-main release tooling executes. The protected `form-package-release`
Environment, commit-pinned Actions, and Cosign v3 sign and immediately verify
the canonical index against the exact protected-main workflow identity. The
workflow emits a same-run checksum-closed candidate containing the tag object,
seven public assets, source/tooling commits, run/attempt, and every asset
digest. It has no repository-write, tag-push, Release, public-attestation, or
publish authority.

The local owner deploy entrypoint downloads that exact run/attempt, verifies its
outer checksum closure, reconstructs and checks the tag object, verifies the
Sigstore bundle and seven-asset semantic closure, repeats remote absence
proofs, then uses only the operator machine's Git/GitHub authority to create the
tag, draft, upload, and immutable release. It downloads the public assets again
and requires the same release ID, tag, names, sizes, and digests. When
repository release immutability is enabled, that final local publication locks
the tag and assets. Asset POSTs use the exact absolute
`https://uploads.github.com/repos/...` URL returned by the release API; passing
that host through `gh api --hostname` is invalid because the CLI rewrites it to
the nonexistent `api.uploads.github.com`. The upload subprocess receives the
operator token only as its host-scoped `GH_ENTERPRISE_TOKEN` environment; the
token is never placed in argv or retained in the repository.

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

Historical publication readback parses the SBOM and provenance as strict
I-JSON, rejects non-canonical or duplicate-key bytes and unknown/omitted
fields, and recomputes their bindings from the signed package index and
retained release manifest. Asset filenames, media types, and checksums alone
are not semantic release evidence. Form Package release tests additionally
validate generated SBOMs offline against the repository-pinned official SPDX
2.3 JSON Schema from the SPDX `v2.3` tag.

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
source chain, and keyless-signs the checkpoint through the same protected
Environment. It emits only a checksum-closed same-run candidate; it cannot
create a tag, Release, public attestation, or published asset. The Form Package
owner deploy surface verifies that candidate and performs the create-only tag,
exact local Release publication, and authoritative readback with operator
credentials. CI permits preparing a new statement/checkpoint pair but rejects
edits, renames, and deletion of existing source paths.

Prepare, publish, and independently read back one checkpoint with explicit
phase boundaries:

```console
bun run deploy -- takoform-form-package-release prepare-revocation \
  --tag forms/revocations/v<statement-version> \
  --expected-commit <40-character-reviewed-commit>

bun run deploy -- takoform-form-package-release publish-revocation \
  --tag forms/revocations/v<statement-version> \
  --expected-commit <same-40-character-reviewed-commit> \
  --run-id <candidate-workflow-run-id> \
  --run-attempt <candidate-workflow-run-attempt>

bun run deploy -- takoform-form-package-release verify-revocation \
  --tag forms/revocations/v<statement-version> \
  --expected-commit <same-40-character-reviewed-commit>
```

After verifying the Sigstore bundle against the exact `@refs/heads/main`
workflow identity, a host starts only at sequence 1 and durably pins the
checkpoint sequence, canonical SHA-256 digest, and cumulative-entry digest. It
accepts only the next sequence with a matching `previousCheckpointDigest` and
unchanged cumulative prefix; rollback, gaps, omissions, prefix rewrites, and
forks fail closed.

## Repository configuration

The source tree and repository settings are both part of the trust boundary:

- the current tag ruleset targets `refs/tags/forms/*/sha256-*`; the retained
  Legacy rule targets `refs/tags/forms/*/v*`. Both restrict creation and
  prevent deletion and non-fast-forward updates;
- `form-package-release` has required reviewers and only permits signing a
  candidate from the `main` branch;
- release immutability is enabled before the first publication; and
- a real tag/release is created only by the local owner deploy entrypoint after
  maintainer authorization.

The retired `1.0.0` and `1.0.1` Form Packages have immutable tags and retained
release evidence. The current `forms/retired-package-set.json` and
`admission/v1/published-package-set.json` select the exact `1.0.1` generation;
`standard-form-conformance published-package-check` authenticates that selected
set and its production package-publisher trust inputs offline. The retained
`1.0.0` history is not rewritten and is not the selected input to that current
check.

The historical `portable-v1` ledger retains 34 signed immutable releases. The
last admission identity actually published was `forms/admissions/v1.0.7`; its
old experiment asserted a ten-Form `portable-standard` subset.
These fields and tags are Legacy evidence only. Current
Form maturity comes from `forms/lifecycle.json`, while every host independently
owns package installation, executable support, activation, principal audience,
and any Offering.
